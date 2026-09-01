package image

import (
	"errors"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diffIDPanicLayer proves a known diff ID is trusted as-is: computing one from the layer contents
// (which for an OCI layout means decompressing the whole layer) must never happen.
type diffIDPanicLayer struct{ mockLayer }

func (diffIDPanicLayer) DiffID() (v1.Hash, error) {
	panic("DiffID must not be computed when the image config already supplies it")
}

func Test_newLayerMetadata_knownDiffIDSkipsComputingIt(t *testing.T) {
	md, err := newLayerMetadata(diffIDPanicLayer{mockLayer{mediaType: v1Types.DockerLayer}}, 7, "sha256:1111111111111111111111111111111111111111111111111111111111111111")
	require.NoError(t, err)
	assert.Equal(t, "sha256:1111111111111111111111111111111111111111111111111111111111111111", md.Digest)
	assert.Equal(t, uint(7), md.Index)
}

// configFileImage overrides only what configDiffIDs reads; anything else is out of bounds by design.
type configFileImage struct {
	v1.Image
	cfg *v1.ConfigFile
	err error
}

func (c configFileImage) ConfigFile() (*v1.ConfigFile, error) { return c.cfg, c.err }

func Test_configDiffIDs(t *testing.T) {
	h := func(hex byte) v1.Hash {
		hx := make([]byte, 64)
		for i := range hx {
			hx[i] = hex
		}
		return v1.Hash{Algorithm: "sha256", Hex: string(hx)}
	}

	tests := []struct {
		name       string
		image      v1.Image
		layerCount int
		want       []string
	}{
		{
			name:       "config supplies one diff ID per layer",
			image:      configFileImage{cfg: &v1.ConfigFile{RootFS: v1.RootFS{DiffIDs: []v1.Hash{h('a'), h('b')}}}, err: nil},
			layerCount: 2,
			want:       []string{h('a').String(), h('b').String()},
		},
		{
			name:       "count mismatch falls back to computing",
			image:      configFileImage{cfg: &v1.ConfigFile{RootFS: v1.RootFS{DiffIDs: []v1.Hash{h('a')}}}, err: nil},
			layerCount: 2,
			want:       nil,
		},
		{
			name:       "config read failure falls back to computing",
			image:      configFileImage{err: errors.New("no config for you")},
			layerCount: 1,
			want:       nil,
		},
		{
			name:       "nil config falls back to computing",
			image:      configFileImage{},
			layerCount: 1,
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &Image{image: tt.image}
			assert.Equal(t, tt.want, i.configDiffIDs(tt.layerCount))
		})
	}
}

func Test_configDiffIDs_matchesARealImageConfig(t *testing.T) {
	// a well-formed image's config lists exactly the diff IDs its layers would compute
	img, err := random.Image(64, 3)
	require.NoError(t, err)
	cfg, err := img.ConfigFile()
	require.NoError(t, err)

	got := (&Image{image: img}).configDiffIDs(3)
	require.Len(t, got, 3)
	for idx, want := range cfg.RootFS.DiffIDs {
		assert.Equal(t, want.String(), got[idx])
	}
}
