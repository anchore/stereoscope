package image

import (
	"context"
	async "github.com/anchore/go-sync"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

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
	require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}))

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
		require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}))
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
	err := i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "layer 1"), "error should name the failed layer: %v", err)
}

func TestImage_readLayers_firstErrorWinsUnderManyFailures(t *testing.T) {
	layers := randomLayers(t, 5)
	for idx := range layers {
		layers[idx] = NewLayer(fakeLayer("garbage/media-type", nil))
	}
	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 4}
	err := i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{})
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
	require.NoError(t, i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{}))

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
	require.NoError(t, i.readLayers(context.Background(), layers, NewFileCatalog(), &progress.Manual{}))

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
	err := i.readLayers(ctx, layers, NewFileCatalog(), &progress.Manual{})
	require.NoError(t, err, "a cancelled read reports no per-layer failure")

	indexed := 0
	for _, layer := range layers {
		if layer.Tree != nil {
			indexed++
		}
	}
	assert.Lessf(t, indexed, len(layers), "cancellation should have stopped some work, indexed %d/%d", indexed, len(layers))
}
