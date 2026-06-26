package stereoscope
package stereoscope

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

func providerOrder(t *testing.T) []string {
	t.Helper()
	providers := ImageProviders(ImageProviderConfig{UserInput: "localhost/myimage:latest"})
	var names []string
	for _, p := range providers {
		names = append(names, p.Value.Name())
	}
	return names
}

func TestImageProviders_includesContainersStorage(t *testing.T) {
	names := providerOrder(t)
	assert.Contains(t, names, image.ContainersStorageSource, "containers-storage provider should be registered")
}

func TestImageProviders_containersStorageBeforeOciRegistry(t *testing.T) {
	names := providerOrder(t)

	csIdx := slices.Index(names, image.ContainersStorageSource)
	registryIdx := slices.Index(names, image.OciRegistrySource)

	require.NotEqual(t, -1, csIdx, "containers-storage provider not found")
	require.NotEqual(t, -1, registryIdx, "oci-registry provider not found")

	assert.Less(t, csIdx, registryIdx, "containers-storage must be resolved before the OCI registry so locally built images win")
}

func TestExtractSchemeSource_containersStorage(t *testing.T) {
	tests := []struct {
		name           string
		userInput      string
		expectedSource string
		expectedInput  string
	}{
		{
			name:           "explicit containers-storage scheme with registry-qualified ref",
			userInput:      "containers-storage:localhost/myimage:latest",
			expectedSource: image.ContainersStorageSource,
			expectedInput:  "localhost/myimage:latest",
		},
		{
			name:           "explicit containers-storage scheme with bare ref",
			userInput:      "containers-storage:myimage:latest",
			expectedSource: image.ContainersStorageSource,
			expectedInput:  "myimage:latest",
		},
		{
			name:           "plain reference is not interpreted as containers-storage scheme",
			userInput:      "localhost/myimage:latest",
			expectedSource: "",
			expectedInput:  "localhost/myimage:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, input := ExtractSchemeSource(tt.userInput, allProviderTags()...)
			assert.Equal(t, tt.expectedSource, source)
			assert.Equal(t, tt.expectedInput, input)
		})
	}
}
