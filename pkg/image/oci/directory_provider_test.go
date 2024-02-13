package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anchore/stereoscope/pkg/file"
)

func Test_NewProviderFromPath(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}
	defer generator.Cleanup()

	//WHEN
	provider := NewDirectoryProvider(&generator, path).(*directoryImageProvider)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_Directory_Provider(t *testing.T) {
	//GIVEN
	tests := []struct {
		name        string
		path        string
		expectedErr bool
	}{
		{"fails to read from path", "", true},
		{"fails to read invalid oci manifest", "test-fixtures/invalid_file", true},
		{"fails to read valid oci manifest with no images", "test-fixtures/no_manifests", true},
		{"fails to read an invalid oci directory", "test-fixtures/valid_manifest", true},
		{"reads a valid oci directory", "test-fixtures/valid_oci_dir", false},
	}

	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	for _, tc := range tests {
		provider := NewDirectoryProvider(tmpDirGen, tc.path)
		t.Run(tc.name, func(t *testing.T) {
			//WHEN
			image, err := provider.Provide(context.Background())

			//THEN
			if tc.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, image)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, image)
			}

		})
	}
}
