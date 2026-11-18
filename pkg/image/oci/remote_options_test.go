package oci

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

func Test_ParseRegistryReference(t *testing.T) {
	tests := []struct {
		name       string
		options    image.RegistryOptions
		wantScheme string
	}{
		{name: "https by default", wantScheme: "https"},
		{name: "http when InsecureUseHTTP is set", options: image.RegistryOptions{InsecureUseHTTP: true}, wantScheme: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseRegistryReference("registry.example.com/repo/img:1.0", tt.options)
			require.NoError(t, err)
			assert.Equal(t, "registry.example.com/repo/img:1.0", ref.Name())
			assert.Equal(t, tt.wantScheme, ref.Context().Scheme())
		})
	}

	_, err := ParseRegistryReference("not a reference", image.RegistryOptions{})
	require.Error(t, err)
}

func Test_RemoteOptions(t *testing.T) {
	// the options have to work for calls the provider itself never makes, with the credentials the
	// provider would have picked for that registry
	guarded, upstream := makeBasicAuthRegistry(t)
	pushRandomRegistryImage(t, upstream, "my-image", "the-tag")

	// nothing here may be answered by credentials on the machine running the test
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	imageStr := fmt.Sprintf("%s/my-image:the-tag", guarded)

	t.Run("with matching credentials", func(t *testing.T) {
		registryOptions := image.RegistryOptions{
			Credentials: []image.RegistryCredentials{
				{Authority: "registry.example.com", Username: "someone", Password: "else"},
				{Authority: guarded, Username: "user", Password: "pass"},
			},
		}

		ref, err := ParseRegistryReference(imageStr, registryOptions)
		require.NoError(t, err)

		options, err := RemoteOptions(context.Background(), ref, registryOptions, nil)
		require.NoError(t, err)

		descriptor, err := remote.Head(ref, options...)
		require.NoError(t, err)
		assert.NotEmpty(t, descriptor.Digest.String())

		tags, err := remote.List(ref.Context(), options...)
		require.NoError(t, err)
		assert.Equal(t, []string{"the-tag"}, tags)

		img, err := remote.Image(ref, options...)
		require.NoError(t, err)
		layers, err := img.Layers()
		require.NoError(t, err)
		require.NotEmpty(t, layers)
		blob, err := layers[0].Compressed()
		require.NoError(t, err, "a blob must be reachable through the transport the options carry")
		_, err = io.Copy(io.Discard, blob)
		require.NoError(t, err)
		require.NoError(t, blob.Close())
	})

	t.Run("without credentials", func(t *testing.T) {
		registryOptions := image.RegistryOptions{}

		ref, err := ParseRegistryReference(imageStr, registryOptions)
		require.NoError(t, err)

		options, err := RemoteOptions(context.Background(), ref, registryOptions, nil)
		require.NoError(t, err)

		_, err = remote.List(ref.Context(), options...)
		var transportErr *transport.Error
		require.ErrorAs(t, err, &transportErr)
		assert.Equal(t, http.StatusUnauthorized, transportErr.StatusCode)
	})
}
