package image

import (
	"runtime"
	"strings"
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
	require.NoError(t, i.readLayers(layers, NewFileCatalog(), &progress.Manual{}))

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
		require.NoError(t, i.readLayers(layers, NewFileCatalog(), &progress.Manual{}))
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
	err := i.readLayers(layers, NewFileCatalog(), &progress.Manual{})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "layer 1"), "error should name the failed layer: %v", err)
}

func TestImage_readLayers_firstErrorWinsUnderManyFailures(t *testing.T) {
	layers := randomLayers(t, 5)
	for idx := range layers {
		layers[idx] = NewLayer(fakeLayer("garbage/media-type", nil))
	}
	i := &Image{contentCacheDir: t.TempDir(), layerReadConcurrency: 4}
	err := i.readLayers(layers, NewFileCatalog(), &progress.Manual{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch layer")
}
