package oci

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anchore/stereoscope/pkg/file"
)

func Test_NewProviderFromTarball(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}

	//WHEN
	provider := NewProviderFromTarball(path, &generator, nil)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_TarballProvide(t *testing.T) {
	//GIVEN
	provider := NewProviderFromTarball("test-fixtures/file.tar", file.NewTempDirGenerator("tempDir"), nil)

	//WHEN
	image, err := provider.Provide(nil)

	//THEN
	assert.NoError(t, err)
	assert.NotNil(t, image)
}

func Test_TarballProvide_Fails(t *testing.T) {
	//GIVEN
	provider := NewProviderFromTarball("", file.NewTempDirGenerator("tempDir"), nil)

	//WHEN
	image, err := provider.Provide(nil)

	//THEN
	assert.Error(t, err)
	assert.Nil(t, image)
}
