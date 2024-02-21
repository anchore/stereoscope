package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anchore/stereoscope/pkg/file"
)

func Test_NewProviderFromTarball(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}
	defer generator.Cleanup()

	//WHEN
	provider := NewArchiveProvider(&generator, path).(*tarballImageProvider)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_TarballProvide(t *testing.T) {
	//GIVEN
	generator := file.NewTempDirGenerator("tempDir")
	defer generator.Cleanup()

	provider := NewArchiveProvider(generator, "test-fixtures/valid-oci.tar")

	//WHEN
	image, err := provider.Provide(context.TODO())

	//THEN
	assert.NoError(t, err)
	assert.NotNil(t, image)
}

func Test_TarballProvide_Fails(t *testing.T) {
	//GIVEN
	generator := file.NewTempDirGenerator("tempDir")
	defer generator.Cleanup()

	provider := NewArchiveProvider(generator, "")

	//WHEN
	image, err := provider.Provide(context.TODO())

	//THEN
	assert.Error(t, err)
	assert.Nil(t, image)
}
