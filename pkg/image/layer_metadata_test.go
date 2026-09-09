package image

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newLayerMetadata_diffIDSource(t *testing.T) {
	layer, err := random.Layer(64, "application/vnd.docker.image.rootfs.diff.tar.gzip")
	require.NoError(t, err)
	wantDiffID, err := layer.DiffID()
	require.NoError(t, err)

	// without a known diff ID the layer is asked (and pays for the answer)
	md, err := newLayerMetadata(layer, 3, "")
	require.NoError(t, err)
	assert.Equal(t, wantDiffID.String(), md.Digest)
	assert.Equal(t, uint(3), md.Index)

	// a known diff ID from the image config is trusted as-is
	md, err = newLayerMetadata(layer, 0, "sha256:1111111111111111111111111111111111111111111111111111111111111111")
	require.NoError(t, err)
	assert.Equal(t, "sha256:1111111111111111111111111111111111111111111111111111111111111111", md.Digest)
}

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
