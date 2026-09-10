package image

import (
	"archive/tar"
	"context"
	"fmt"
	async "github.com/anchore/go-sync"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"io"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/random"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wagoodman/go-progress"
)

func Test_layerReadWorkers(t *testing.T) {
	cpuDefault := runtime.NumCPU()
	if cpuDefault > 8 {
		cpuDefault = 8
	}

	tests := []struct {
		name   string
		layers int
		want   int
	}{
		{name: "CPUs capped at 8", layers: 1000, want: cpuDefault},
		{name: "capped by layer count", layers: 1, want: 1},
		{name: "never below one worker", layers: 0, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, layerReadWorkers(tt.layers))
		})
	}
}

// layerConcurrency installs stage executors the way a caller would. A bound of 0 leaves that
// stage out, so Read fills in the default.
func layerConcurrency(fetch, index int) context.Context {
	ctx := context.Background()
	if fetch > 0 {
		ctx = async.SetContextExecutor(ctx, LayerFetchExecutor, async.NewExecutor(fetch))
	}
	if index > 0 {
		ctx = async.SetContextExecutor(ctx, LayerIndexExecutor, async.NewExecutor(index))
	}
	return ctx
}

// randomLayers builds n small, well-formed docker layers.
func randomLayers(t *testing.T, n int) []*Layer {
	t.Helper()
	layers := make([]*Layer, n)
	for idx := range layers {
		v1Layer, err := random.Layer(64, v1Types.DockerLayer)
		require.NoError(t, err)
		layers[idx] = NewLayer(v1Layer)
	}
	return layers
}

func TestImage_readLayers_concurrentReadKeepsManifestOrder(t *testing.T) {
	const layerCount = 6
	layers := randomLayers(t, layerCount)

	i := &Image{contentCacheDir: t.TempDir()}
	require.NoError(t, i.readLayers(layerConcurrency(3, 3), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	// completion order is up to the pools; the layer set must still be manifest-ordered and
	// fully indexed before readLayers returns (the squash that follows depends on both)
	for idx, layer := range layers {
		assert.Equalf(t, uint(idx), layer.Metadata.Index, "layer %d out of order", idx)
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
		assert.NotEmptyf(t, layer.Metadata.Digest, "layer %d has no metadata", idx)
	}
}

// fileTuple is the part of a file's identity that must survive indexing unchanged, regardless of
// how many workers raced to produce it.
type fileTuple struct {
	RealPath        string
	Type            file.Type
	Size            int64
	LinkDestination string
}

// fileTuples walks every file in tree (including dirs and links, not just regulars) and resolves
// each against catalog, so the comparison covers exactly what the tree/catalog pair actually
// recorded rather than just path names.
func fileTuples(t *testing.T, tree filetree.Reader, catalog FileCatalogReader) []fileTuple {
	t.Helper()
	var tuples []fileTuple
	for _, ref := range tree.AllFiles(file.AllTypes()...) {
		entry, err := catalog.Get(ref)
		require.NoErrorf(t, err, "no catalog entry for %s", ref.RealPath)
		tuples = append(tuples, fileTuple{
			RealPath:        string(entry.RealPath),
			Type:            entry.Type,
			Size:            entry.Size(),
			LinkDestination: entry.LinkDestination,
		})
	}
	sort.Slice(tuples, func(i, j int) bool { return tuples[i].RealPath < tuples[j].RealPath })
	return tuples
}

func TestImage_Read_concurrentMatchesSequential(t *testing.T) {
	// output parity between the serial and concurrent read paths is the core correctness claim of
	// the fetch/index pipeline. Build one layer set, read it twice - once with both stages bounded
	// to 1, once bounded well above the layer count - and require the two images to be identical,
	// not just individually well-formed.
	const layerCount = 6
	reg, lnk, sym := byte(tar.TypeReg), byte(tar.TypeLink), byte(tar.TypeSymlink)

	var v1Layers []v1.Layer
	for idx := 0; idx < layerCount; idx++ {
		entries := []tarEntry{
			{path: fmt.Sprintf("only-in-%d.txt", idx), typeFlag: reg, contents: fmt.Sprintf("unique-%d", idx)},
			// every layer overwrites this, so squash order matters
			{path: "shared.txt", typeFlag: reg, contents: fmt.Sprintf("v%d", idx)},
			{path: fmt.Sprintf("link-to-%d", idx), typeFlag: sym, linkPath: fmt.Sprintf("only-in-%d.txt", idx)},
		}
		if idx == 0 {
			// a same-layer hardlink pair: the data-carrying name must precede the hardlink header
			entries = append(entries,
				tarEntry{path: "hard-target.txt", typeFlag: reg, contents: "hardlink-payload"},
				tarEntry{path: "hard-link.txt", typeFlag: lnk, linkPath: "hard-target.txt"},
			)
		}
		v1Layers = append(v1Layers, layerFromTarEntries(t, entries...))
	}

	read := func(ctx context.Context) *Image {
		t.Helper()
		v1Img, err := mutate.AppendLayers(empty.Image, v1Layers...)
		require.NoError(t, err)
		img := New(v1Img, file.NewTempDirGenerator("concurrency-parity-test"), t.TempDir())
		t.Cleanup(func() { require.NoError(t, img.Cleanup()) })
		require.NoError(t, img.Read(ctx))
		return img
	}

	serial := read(layerConcurrency(1, 1))
	concurrent := read(layerConcurrency(4, 4))

	require.Equal(t, serial.Metadata.Size, concurrent.Metadata.Size, "image size")
	require.Len(t, concurrent.Layers, len(serial.Layers))

	for idx := range serial.Layers {
		s, c := serial.Layers[idx], concurrent.Layers[idx]
		require.Equalf(t, s.Metadata.Index, c.Metadata.Index, "layer %d index", idx)
		require.Equalf(t, s.Metadata.Digest, c.Metadata.Digest, "layer %d digest", idx)
		require.Equalf(t, s.Metadata.Size, c.Metadata.Size, "layer %d size", idx)

		require.Equalf(t, fileTuples(t, s.Tree, serial.FileCatalog), fileTuples(t, c.Tree, concurrent.FileCatalog),
			"layer %d tree", idx)
		require.Equalf(t, fileTuples(t, s.SquashedTree, serial.FileCatalog), fileTuples(t, c.SquashedTree, concurrent.FileCatalog),
			"layer %d squashed tree", idx)
	}

	// and both must resolve the overwritten path to the top layer's value
	for name, img := range map[string]*Image{"serial": serial, "concurrent": concurrent} {
		rc, err := img.OpenPathFromSquash("/shared.txt")
		require.NoErrorf(t, err, "%s image", name)
		contents, err := io.ReadAll(rc)
		require.NoErrorf(t, err, "%s image", name)
		require.NoErrorf(t, rc.Close(), "%s image", name)
		require.Equalf(t, fmt.Sprintf("v%d", layerCount-1), string(contents), "%s image resolved the wrong layer", name)
	}
}

func TestImage_readLayers_failedLayerFailsTheReadWithoutHanging(t *testing.T) {
	layers := randomLayers(t, 3)
	// the middle layer fails up front (unknown media type): the read must report that layer and
	// return - with workers still draining - rather than deadlock or panic
	layers[1] = NewLayer(fakeLayer("garbage/media-type", nil))

	i := &Image{contentCacheDir: t.TempDir()}
	err := i.readLayers(layerConcurrency(2, 2), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "layer 1"), "error should name the failed layer: %v", err)
}

// barrierLayer's MediaType call blocks until every simultaneously-bad layer sharing its barrier
// has also reached this point, then releases them all together. readLayers now cancels its
// peers' still-queued work on the first recorded failure, so a synthetic failure that's fast
// enough to win the race could otherwise stop a peer's fetch from ever being attempted - and an
// error nobody ever discovered can hardly be reported.
type barrierLayer struct {
	v1.Layer
	barrier *sync.WaitGroup
}

func (b barrierLayer) MediaType() (v1Types.MediaType, error) {
	b.barrier.Done()
	b.barrier.Wait()
	return b.Layer.MediaType()
}

func badLayersWithBarrier(layers []*Layer, bad ...int) {
	var barrier sync.WaitGroup
	barrier.Add(len(bad))
	for _, idx := range bad {
		layers[idx] = NewLayer(barrierLayer{Layer: fakeLayer("garbage/media-type", nil), barrier: &barrier})
	}
}

func TestImage_readLayers_reportsFailuresLowestLayerFirst(t *testing.T) {
	// several layers can fail, and go-sync joins in completion order, so without ordering the
	// reported error names a different layer from one run to the next - enough to flake a
	// downstream test over a corrupt-image fixture
	layers := randomLayers(t, 6)
	badLayersWithBarrier(layers, 4, 1, 3)

	i := &Image{contentCacheDir: t.TempDir()}
	err := i.readLayers(layerConcurrency(4, 4), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "failed to fetch layer 1")
	// every failure is reported, and layer 1 before layer 3 before layer 4
	one, three, four := strings.Index(msg, "layer 1"), strings.Index(msg, "layer 3"), strings.Index(msg, "layer 4")
	require.NotEqual(t, -1, three)
	require.NotEqual(t, -1, four)
	assert.Less(t, one, three, "layer 1 should be reported before layer 3")
	assert.Less(t, three, four, "layer 3 should be reported before layer 4")
}

func TestImage_readLayers_errorOrderIsStableAcrossRuns(t *testing.T) {
	// the ordering above must not just happen to hold on one scheduling
	var first string
	for run := 0; run < 25; run++ {
		layers := randomLayers(t, 6)
		badLayersWithBarrier(layers, 5, 2, 4)
		i := &Image{contentCacheDir: t.TempDir()}
		err := i.readLayers(layerConcurrency(4, 4), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
		require.Error(t, err)
		if run == 0 {
			first = err.Error()
			continue
		}
		require.Equal(t, first, err.Error(), "error text changed between runs")
	}
}

// countingLayer records the high-water mark of concurrent Uncompressed calls, which is what a
// fetch bound is supposed to cap.
type countingLayer struct {
	v1.Layer
	inFlight *atomic.Int64
	maxSeen  *atomic.Int64
}

func (c *countingLayer) Uncompressed() (io.ReadCloser, error) {
	n := c.inFlight.Add(1)
	for {
		m := c.maxSeen.Load()
		if n <= m || c.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	rc, err := c.Layer.Uncompressed()
	if err != nil {
		c.inFlight.Add(-1)
		return nil, err
	}
	return &decrementOnClose{ReadCloser: rc, n: c.inFlight}, nil
}

type decrementOnClose struct {
	io.ReadCloser
	n *atomic.Int64
}

func (d *decrementOnClose) Close() error {
	d.n.Add(-1)
	return d.ReadCloser.Close()
}

func countingLayers(t *testing.T, n int, inFlight, maxSeen *atomic.Int64) []*Layer {
	t.Helper()
	layers := make([]*Layer, n)
	for idx := range layers {
		v1Layer, err := random.Layer(4096, v1Types.DockerLayer)
		require.NoError(t, err)
		layers[idx] = NewLayer(&countingLayer{Layer: v1Layer, inFlight: inFlight, maxSeen: maxSeen})
	}
	return layers
}

func TestImage_readLayers_callerSuppliedExecutorBoundsTheFetchStage(t *testing.T) {
	// the default for 8 layers on any CI box is >1, so a max of 1 can only come from the
	// executor the caller installed
	var inFlight, maxSeen atomic.Int64
	layers := countingLayers(t, 8, &inFlight, &maxSeen)

	ctx := async.SetContextExecutor(context.Background(), LayerFetchExecutor, async.NewExecutor(1))
	i := &Image{contentCacheDir: t.TempDir()}
	require.NoError(t, i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	assert.Equal(t, int64(1), maxSeen.Load(), "caller's fetch executor was not honoured")
	for idx, layer := range layers {
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
	}
}

func TestImage_readLayers_defaultAppliesWhenCallerSuppliesNothing(t *testing.T) {
	var inFlight, maxSeen atomic.Int64
	layers := countingLayers(t, 8, &inFlight, &maxSeen)

	// no executors installed at all: both stages must still get a working bound rather than
	// falling back to go-sync's inline serial executor, which would collapse the two stages
	i := &Image{contentCacheDir: t.TempDir()}
	require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	assert.LessOrEqual(t, maxSeen.Load(), int64(layerReadWorkers(len(layers))), "default fetch bound was exceeded")
	for idx, layer := range layers {
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
	}
}

func TestWithLayerExecutors_defaultsFillInOnlyWhenNothingIsInstalled(t *testing.T) {
	const layerCount = 6

	t.Run("nothing installed", func(t *testing.T) {
		// both stages need a bound, because go-sync's last fallback is an inline serial executor
		// and that would collapse the pipeline into one stage
		ctx := withLayerExecutors(context.Background(), layerCount)
		assert.True(t, async.HasContextExecutor(ctx, LayerFetchExecutor))
		assert.True(t, async.HasContextExecutor(ctx, LayerIndexExecutor))
	})

	t.Run("ExecutorDefault installed", func(t *testing.T) {
		// a host that installed a single process-wide budget already answered for both stages;
		// go-sync resolves the missing names to it, so we must not talk over it
		base := async.SetContextExecutor(context.Background(), async.ExecutorDefault, async.NewExecutor(3))
		ctx := withLayerExecutors(base, layerCount)
		assert.False(t, async.HasContextExecutor(ctx, LayerFetchExecutor), "overrode the host's default")
		assert.False(t, async.HasContextExecutor(ctx, LayerIndexExecutor), "overrode the host's default")
	})

	t.Run("one stage named, plus ExecutorDefault", func(t *testing.T) {
		// the registry provider's shape: it pins fetch and leaves indexing to whatever the host set
		base := async.SetContextExecutor(context.Background(), async.ExecutorDefault, async.NewExecutor(3))
		base = async.SetContextExecutor(base, LayerFetchExecutor, async.NewExecutor(1))
		ctx := withLayerExecutors(base, layerCount)
		assert.True(t, async.HasContextExecutor(ctx, LayerFetchExecutor), "the pinned stage must survive")
		assert.False(t, async.HasContextExecutor(ctx, LayerIndexExecutor), "the other stage falls back to the host's default")
	})
}

func TestImage_readLayers_honoursExecutorDefault(t *testing.T) {
	// end to end: a host budget of one must actually bound fetching, not just be recorded
	var inFlight, maxSeen atomic.Int64
	layers := countingLayers(t, 8, &inFlight, &maxSeen)

	ctx := async.SetContextExecutor(context.Background(), async.ExecutorDefault, async.NewExecutor(1))
	i := &Image{contentCacheDir: t.TempDir()}
	require.NoError(t, i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	assert.Equal(t, int64(1), maxSeen.Load(), "the host's ExecutorDefault did not bound the fetch stage")
	for idx, layer := range layers {
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
	}
}

func TestImage_readLayers_stageBoundsAreIndependent(t *testing.T) {
	// the point of two executors rather than one over a fused per-layer unit: pinning fetch must
	// not serialise indexing. This is the registry configuration, and a single bound cannot
	// express it.
	const layerCount = 8
	var inFlight, maxSeen atomic.Int64
	layers := countingLayers(t, layerCount, &inFlight, &maxSeen)

	// only a fetch executor, so Read must fill in the index default rather than reusing this one
	ctx := layerConcurrency(1, 0)
	i := &Image{contentCacheDir: t.TempDir()}
	require.NoError(t, i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	assert.Equal(t, int64(1), maxSeen.Load(), "the caller's fetch bound was not honoured")
	for idx, layer := range layers {
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
	}

	// and the default filled in for the stage the caller left out
	filled := withLayerExecutors(layerConcurrency(1, 0), layerCount)
	assert.True(t, async.HasContextExecutor(filled, LayerFetchExecutor))
	assert.True(t, async.HasContextExecutor(filled, LayerIndexExecutor))
	assert.Greater(t, layerReadWorkers(layerCount), 1, "the index default should not be serial")
}

func TestImage_readLayers_cancelledContextStopsTheRead(t *testing.T) {
	layers := randomLayers(t, 8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	i := &Image{contentCacheDir: t.TempDir()}
	err := i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.NoError(t, err, "a cancelled read reports no per-layer failure")

	indexed := 0
	for _, layer := range layers {
		if layer.Tree != nil {
			indexed++
		}
	}
	assert.Lessf(t, indexed, len(layers), "cancellation should have stopped some work, indexed %d/%d", indexed, len(layers))
}

func TestImage_readLayers_internalAbortReportsTheLayerErrorNotCancellation(t *testing.T) {
	// layer 2 fails, which cancels readLayers' internal context so its peers stop starting more
	// work. That internal cancellation is our own bookkeeping, not the caller's, and must not
	// leak out as context.Canceled.
	layers := randomLayers(t, 6)
	layers[2] = NewLayer(fakeLayer("garbage/media-type", nil))

	i := &Image{contentCacheDir: t.TempDir()}
	err := i.readLayers(layerConcurrency(4, 4), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch layer 2")
	assert.NotErrorIs(t, err, context.Canceled, "an internal abort must not surface as cancellation")
}

// failingFetchLayer has a valid media type, so Image.Read's up-front validation passes and the
// failure lands in the fetch stage where the pipeline has to cope with it.
type failingFetchLayer struct {
	v1.Layer
}

func (failingFetchLayer) MediaType() (v1Types.MediaType, error) { return v1Types.DockerLayer, nil }
func (failingFetchLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, fmt.Errorf("simulated fetch failure")
}

func TestImage_Read_squashOverlapMatchesOrderedSquash(t *testing.T) {
	// every layer overwrites shared.txt, so a squash that ran out of order would resolve it wrong
	var layers []v1.Layer
	for idx := 0; idx < 6; idx++ {
		layers = append(layers, layerFromTarEntries(t,
			tarEntry{path: fmt.Sprintf("only-in-%d.txt", idx), typeFlag: tar.TypeReg, contents: fmt.Sprintf("c%d", idx)},
			tarEntry{path: "shared.txt", typeFlag: tar.TypeReg, contents: fmt.Sprintf("v%d", idx)},
		))
	}
	img := readImageFromLayers(t, layers...)

	require.Len(t, img.Layers, 6)
	for idx, layer := range img.Layers {
		assert.Equalf(t, uint(idx), layer.Metadata.Index, "layer %d out of order", idx)
		assert.NotNilf(t, layer.SquashedTree, "layer %d was never squashed", idx)
		assert.NotNilf(t, layer.SquashedSearchContext, "layer %d has no squashed search context", idx)
	}

	// the squash must still be bottom-up: the top layer's value wins
	rc, err := img.OpenPathFromSquash("/shared.txt")
	require.NoError(t, err)
	defer rc.Close()
	contents, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "v5", string(contents), "squash did not resolve to the top layer")

	// and each layer's own squash sees only layers at or below it
	for idx, layer := range img.Layers {
		_, ref, err := layer.SquashedTree.File(file.Path(fmt.Sprintf("/only-in-%d.txt", idx)))
		require.NoErrorf(t, err, "layer %d squash", idx)
		assert.Truef(t, ref.HasReference(), "layer %d squash is missing its own file", idx)

		if idx+1 < len(img.Layers) {
			_, above, err := layer.SquashedTree.File(file.Path(fmt.Sprintf("/only-in-%d.txt", idx+1)))
			require.NoErrorf(t, err, "layer %d squash", idx)
			assert.Falsef(t, above.HasReference(), "layer %d squash leaked a file from the layer above", idx)
		}
	}
}

func TestImage_Read_fetchFailureDoesNotHangTheSquash(t *testing.T) {
	good := func(idx int) v1.Layer {
		return layerFromTarEntries(t, tarEntry{
			path: fmt.Sprintf("f%d.txt", idx), typeFlag: tar.TypeReg, contents: "ok",
		})
	}
	// the middle layer fails to fetch, so its gate must open as not-ok and squash must stop
	// rather than block forever or squash a layer with no tree
	v1Img, err := mutate.AppendLayers(empty.Image, good(0), failingFetchLayer{Layer: good(1)}, good(2))
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("gate-test"), t.TempDir())
	t.Cleanup(func() { _ = img.Cleanup() })

	done := make(chan error, 1)
	go func() { done <- img.Read(context.Background()) }()

	select {
	case readErr := <-done:
		require.Error(t, readErr)
		assert.Contains(t, readErr.Error(), "layer 1")
	case <-time.After(30 * time.Second):
		t.Fatal("Read hung: a failed layer left the squash waiting on its gate")
	}
}

func TestImage_Read_cancelledContextDoesNotHangTheSquash(t *testing.T) {
	var layers []v1.Layer
	for idx := 0; idx < 8; idx++ {
		layers = append(layers, layerFromTarEntries(t, tarEntry{
			path: fmt.Sprintf("f%d.txt", idx), typeFlag: tar.TypeReg, contents: "ok",
		}))
	}
	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("cancel-test"), t.TempDir())
	t.Cleanup(func() { _ = img.Cleanup() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- img.Read(ctx) }()

	select {
	case readErr := <-done:
		// a cancelled read must report the cancellation rather than hand back a half-built
		// image, and must not panic building a search context over an unsquashed layer
		require.Error(t, readErr)
		assert.ErrorIs(t, readErr, context.Canceled)
	case <-time.After(30 * time.Second):
		t.Fatal("Read hung on a cancelled context: gates were never released")
	}
}

// blockingLayer's Uncompressed call reports itself started and then waits to be released, so a
// test can cancel the read while this layer's fetch worker is provably still in flight rather
// than racing to catch it with a sleep.
type blockingLayer struct {
	v1.Layer
	started chan<- struct{}
	release <-chan struct{}
}

func (b blockingLayer) Uncompressed() (io.ReadCloser, error) {
	close(b.started)
	<-b.release
	return b.Layer.Uncompressed()
}

func TestImage_Read_midFlightCancellationIsReportedAsCancellation(t *testing.T) {
	// this is also the regression case for the fetch/index handoff: cancelling while a fetch
	// worker is genuinely mid-flight is what could race the handoff channel's close against a
	// worker still trying to hand a layer off to indexing
	started := make(chan struct{})
	release := make(chan struct{})

	var layers []v1.Layer
	for idx := 0; idx < 6; idx++ {
		l := layerFromTarEntries(t, tarEntry{path: fmt.Sprintf("f%d.txt", idx), typeFlag: tar.TypeReg, contents: "ok"})
		if idx == 3 {
			l = blockingLayer{Layer: l, started: started, release: release}
		}
		layers = append(layers, l)
	}
	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("cancel-midflight-test"), t.TempDir())
	t.Cleanup(func() { _ = img.Cleanup() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- img.Read(ctx) }()

	<-started
	cancel()
	close(release) // let the blocked worker proceed now that it is already cancelled

	select {
	case readErr := <-done:
		require.Error(t, readErr)
		assert.ErrorIs(t, readErr, context.Canceled, "a mid-flight caller cancellation must be reported as cancellation")
	case <-time.After(30 * time.Second):
		t.Fatal("Read hung after a mid-flight cancellation")
	}
}

func TestLayerGates_releaseAllIsIdempotentAndUnblocksWaiters(t *testing.T) {
	g := newLayerGates(3)
	g.done(0, true)
	g.done(0, false) // second call must not flip the answer or double-Done

	g.releaseAll()
	g.releaseAll()

	assert.True(t, g.wait(0), "an indexed layer should stay ok")
	assert.False(t, g.wait(1), "an unreached layer should report not-ok")
	assert.False(t, g.wait(2))
}

func TestImage_squashLayers_reportsAnUnindexedLayer(t *testing.T) {
	// squash must not trust the read stages to have raised something. If it returned nil here,
	// Read would carry on and build the image search context over a layer with no tree, which
	// panics. This is the deterministic backstop for that.
	layers := randomLayers(t, 3)
	gates := newLayerGates(len(layers))
	gates.done(0, true)
	gates.done(1, false) // indexed nothing
	gates.done(2, true)

	i := &Image{contentCacheDir: t.TempDir()}
	// layer 0 needs a tree for the idx==0 branch to work at all
	require.NoError(t, layers[0].fetch(0, i.contentCacheDir))
	require.NoError(t, layers[0].index(NewFileCatalog()))

	err := i.squashLayers(layers, gates, &progress.Manual{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "layer 1")
	assert.Contains(t, err.Error(), "not indexed")
}

// panickingLayer panics where a stage will run it, with a media type that passes Read's
// up-front validation so the panic actually reaches the pool.
type panickingLayer struct {
	v1.Layer
}

func (panickingLayer) MediaType() (v1Types.MediaType, error) { return v1Types.DockerLayer, nil }
func (panickingLayer) Uncompressed() (io.ReadCloser, error) {
	panic("boom from a fetch worker")
}

func TestImage_Read_panicInAStageBecomesAnError(t *testing.T) {
	// stereoscope reads untrusted images inside long-running services, so a panic in a worker
	// must not take the process down. Before the stages were driven by go-sync the panic
	// happened on a goroutine the caller could not recover from at all.
	//
	// What this pins is containment: no crash, no hang, and the panic reaches the caller as an
	// error. It is deliberately not a regression test for go-sync losing the error it recovered
	// (fixed in v0.1.2) - two layers do not reliably open that window, and go-sync covers it
	// directly in Test_CollectHandlesPanicsConcurrently.
	good := layerFromTarEntries(t, tarEntry{path: "a.txt", typeFlag: tar.TypeReg, contents: "ok"})
	bad := layerFromTarEntries(t, tarEntry{path: "b.txt", typeFlag: tar.TypeReg, contents: "different digest"})
	v1Img, err := mutate.AppendLayers(empty.Image, good, panickingLayer{Layer: bad})
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("panic-test"), t.TempDir())
	t.Cleanup(func() { _ = img.Cleanup() })

	done := make(chan error, 1)
	go func() { done <- img.Read(context.Background()) }()

	select {
	case readErr := <-done:
		require.Error(t, readErr, "a panicking layer must surface as an error, not a crash")
		assert.Contains(t, readErr.Error(), "boom from a fetch worker")
	case <-time.After(30 * time.Second):
		// a panic skips the stage's gates.done, so this also pins that Read's releaseAll
		// safety net opens the gate the squash is waiting on
		t.Fatal("Read hung after a panic: the squash gate was never released")
	}
}

func TestImage_Read_squashedSearchContextMatchesAFreshlyBuiltOne(t *testing.T) {
	// Read hands back the top layer's squashed search context rather than rebuilding one over the
	// same tree and index. That is only safe if the two are interchangeable, so pin it: symlinks
	// are what NewSearchContext actually indexes, so the fixture carries some.
	var layers []v1.Layer
	for idx := 0; idx < 4; idx++ {
		layers = append(layers, layerFromTarEntries(t,
			tarEntry{path: fmt.Sprintf("f%d.txt", idx), typeFlag: tar.TypeReg, contents: "x"},
			tarEntry{path: fmt.Sprintf("s%d", idx), typeFlag: tar.TypeSymlink, linkPath: fmt.Sprintf("f%d.txt", idx)},
			tarEntry{path: "shared.txt", typeFlag: tar.TypeReg, contents: fmt.Sprintf("v%d", idx)},
		))
	}
	img := readImageFromLayers(t, layers...)

	reused := img.SquashedSearchContext
	fresh := filetree.NewSearchContext(img.SquashedTree(), img.FileCatalog)

	for _, glob := range []string{"**/*.txt", "**/s*", "**"} {
		fromReused, err := reused.SearchByGlob(glob)
		require.NoErrorf(t, err, "glob %q", glob)
		fromFresh, err := fresh.SearchByGlob(glob)
		require.NoErrorf(t, err, "glob %q", glob)

		require.Lenf(t, fromReused, len(fromFresh), "glob %q returned a different count", glob)
		for i := range fromReused {
			assert.Equalf(t, fromFresh[i].RequestPath, fromReused[i].RequestPath, "glob %q result %d", glob, i)
		}
	}
}
