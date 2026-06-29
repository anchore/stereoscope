//go:build !containers_image_openpgp

package containerstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func TestStubProvider_Name(t *testing.T) {
	p := NewProvider(file.NewTempDirGenerator("test"), "localhost/myimage:latest", nil)
	assert.Equal(t, image.ContainersStorageSource, p.Name())
	assert.Equal(t, "containers-storage", p.Name())
}

func TestStubProvider_ProvideReturnsNotCompiledIn(t *testing.T) {
	p := NewProvider(file.NewTempDirGenerator("test"), "localhost/myimage:latest", nil)

	img, err := p.Provide(context.Background())

	require.Error(t, err)
	assert.Nil(t, img, "stub must not return an image so source auto-resolution can continue")
	assert.ErrorContains(t, err, "containers-storage support is not compiled in")
	assert.ErrorContains(t, err, "containers_image_openpgp")
}
