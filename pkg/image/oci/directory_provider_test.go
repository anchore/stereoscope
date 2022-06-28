package oci

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/stretchr/testify/assert"
)

func Test_NewProviderFromPath(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}

	//WHEN
	provider := NewProviderFromPath(path, &generator)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_Directory_Provide(t *testing.T) {
	//GIVEN
	tests := []struct {
		name        string
		path        string
		expectedErr bool
	}{
		{"fails to read from path", "", true},
		{"reads invalid oci manifest", "test-fixtures/invalid_file", true},
		{"reads valid oci manifest with no images", "test-fixtures/no_manifests", true},
		{"reads a fully correct manifest", "test-fixtures/valid_manifest", false},
	}

	for _, tc := range tests {
		provider := NewProviderFromPath(tc.path, file.NewTempDirGenerator("tempDir"))
		t.Run(tc.name, func(t *testing.T) {
			//WHEN
			image, err := provider.Provide(nil)

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
