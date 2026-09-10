package image

import (
	"archive/tar"
	"context"
	"fmt"
	async "github.com/anchore/go-sync"
	"github.com/anchore/stereoscope/pkg/file"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"io"
	"runtime"
	"strings"
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
		name     string
		layers   int
		override int
		want     int
	}{
		{name: "override wins", layers: 10, override: 3, want: 3},
		{name: "override capped at layer count", layers: 2, override: 8, want: 2},
		{name: "registry-style single fetch", layers: 10, override: 1, want: 1},
		{name: "default is CPUs capped at 8 and the layer count", layers: 1000, override: 0, want: cpuDefault},
		{name: "default capped by layer count", layers: 1, override: 0, want: 1},
		{name: "never below one worker", layers: 0, override: 3, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, layerReadWorkers(tt.layers, tt.override))
		})
	}
}

func TestWithLayerReadConcurrency(t *testing.T) {
	i := &Image{}
	require.NoError(t, WithLayerReadConcurrency(2)(i))
	assert.Equal(t, 2, i.layerReadConcurrency)
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

	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 3}
	require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	// completion order is up to the pools; the layer set must still be manifest-ordered and
	// fully indexed before readLayers returns (the squash that follows depends on both)
	for idx, layer := range layers {
		assert.Equalf(t, uint(idx), layer.Metadata.Index, "layer %d out of order", idx)
		assert.NotNilf(t, layer.Tree, "layer %d was not indexed", idx)
		assert.NotEmptyf(t, layer.Metadata.Digest, "layer %d has no metadata", idx)
	}
}

func TestImage_readLayers_sequentialMatchesConcurrent(t *testing.T) {
	// a registry-style single-fetch run must produce the same layer set shape as a parallel one
	for _, concurrency := range []int{1, 4} {
		layers := randomLayers(t, 4)
		i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: concurrency}
		require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))
		for idx, layer := range layers {
			require.Equal(t, uint(idx), layer.Metadata.Index)
			require.NotNil(t, layer.Tree)
		}
	}
}

func TestImage_readLayers_failedLayerFailsTheReadWithoutHanging(t *testing.T) {
	layers := randomLayers(t, 3)
	// the middle layer fails up front (unknown media type): the read must report that layer and
	// return - with workers still draining - rather than deadlock or panic
	layers[1] = NewLayer(fakeLayer("garbage/media-type", nil))

	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 2}
	err := i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "layer 1"), "error should name the failed layer: %v", err)
}

func TestImage_readLayers_firstErrorWinsUnderManyFailures(t *testing.T) {
	layers := randomLayers(t, 5)
	for idx := range layers {
		layers[idx] = NewLayer(fakeLayer("garbage/media-type", nil))
	}
	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 4}
	err := i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch layer")
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

func TestImage_readLayers_perImageDefaultAppliesWhenCallerSuppliesNothing(t *testing.T) {
	var inFlight, maxSeen atomic.Int64
	layers := countingLayers(t, 8, &inFlight, &maxSeen)

	// WithLayerReadConcurrency is the fallback the registry provider relies on
	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 1}
	require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}, newLayerGates(len(layers))))

	assert.Equal(t, int64(1), maxSeen.Load(), "per-image fetch bound was not applied")
}

func TestImage_readLayers_stageBoundsAreIndependent(t *testing.T) {
	// the point of two executors rather than one: pinning fetch must not serialise indexing,
	// which is the registry configuration and what a fused work unit cannot express
	const layerCount = 6
	i := &Image{layerReadConcurrency: 1}

	assert.Equal(t, 1, layerReadWorkers(layerCount, i.layerReadConcurrency), "fetch bound follows the per-image option")
	assert.Greater(t, layerReadWorkers(layerCount, 0), 1, "index bound stays at the default")

	// and both executors are installed, so neither stage silently falls back to inline execution
	ctx := i.withLayerExecutors(context.Background(), layerCount)
	assert.True(t, async.HasContextExecutor(ctx, LayerFetchExecutor))
	assert.True(t, async.HasContextExecutor(ctx, LayerIndexExecutor))
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
	require.NoError(t, layers[0].Fetch(0, i.contentCacheDir))
	require.NoError(t, layers[0].Index(NewFileCatalog()))

	err := i.squashLayers(layers, gates, &progress.Manual{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "layer 1")
	assert.Contains(t, err.Error(), "not indexed")
}
