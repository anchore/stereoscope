package oci

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/stretchr/testify/assert"
)

func Test_NewProviderFromTarball(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}

	//WHEN
	provider := NewProviderFromTarball(path, &generator)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_TarballProvide(t *testing.T) {
	//GIVEN
	provider := NewProviderFromTarball("test-fixtures/file.tar", file.NewTempDirGenerator("tempDir"))

	//WHEN
	image, err := provider.Provide(nil)

	//THEN
	assert.NoError(t, err)
	assert.NotNil(t, image)
}

func Test_TarballProvide_Fails(t *testing.T) {
	//GIVEN
	provider := NewProviderFromTarball("", file.NewTempDirGenerator("tempDir"))

	//WHEN
	image, err := provider.Provide(nil)

	//THEN
	assert.Error(t, err)
	assert.Nil(t, image)
}
