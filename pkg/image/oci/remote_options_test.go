package oci

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
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

func Test_RegistryProvider_DisableDefaultKeychain(t *testing.T) {
	// the docker config the default keychain would find holds working credentials for the registry,
	// so the pull succeeding or not shows whether that file was consulted
	guarded, upstream := makeBasicAuthRegistry(t)
	pushRandomRegistryImage(t, upstream, "my-image", "the-tag")

	dockerConfig := t.TempDir()
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	require.NoError(t, os.WriteFile(
		filepath.Join(dockerConfig, "config.json"),
		[]byte(fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, guarded, auth)),
		0o600,
	))
	t.Setenv("DOCKER_CONFIG", dockerConfig)

	imageStr := fmt.Sprintf("%s/my-image:the-tag", guarded)
	provide := func(t *testing.T, registryOptions image.RegistryOptions) (*image.Image, error) {
		t.Helper()

		generator := file.TempDirGenerator{}
		t.Cleanup(func() { _ = generator.Cleanup() })

		return NewRegistryProvider(&generator, registryOptions, imageStr, nil).Provide(context.Background())
	}

	t.Run("the default keychain is used by default", func(t *testing.T) {
		img, err := provide(t, image.RegistryOptions{})
		require.NoError(t, err)
		require.NotNil(t, img)
		require.NoError(t, img.Cleanup())
	})

	t.Run("the default keychain is not used when disabled", func(t *testing.T) {
		img, err := provide(t, image.RegistryOptions{DisableDefaultKeychain: true})
		assert.Nil(t, img)

		var transportErr *transport.Error
		require.ErrorAs(t, err, &transportErr)
		assert.Equal(t, http.StatusUnauthorized, transportErr.StatusCode)
	})

	t.Run("credentials that do not match the registry do not bring the default keychain back", func(t *testing.T) {
		img, err := provide(t, image.RegistryOptions{
			DisableDefaultKeychain: true,
			Credentials: []image.RegistryCredentials{
				{Authority: "registry.example.com", Username: "someone", Password: "else"},
			},
		})
		assert.Nil(t, img)

		var transportErr *transport.Error
		require.ErrorAs(t, err, &transportErr)
		assert.Equal(t, http.StatusUnauthorized, transportErr.StatusCode)
	})

	t.Run("supplied credentials still work when disabled", func(t *testing.T) {
		img, err := provide(t, image.RegistryOptions{
			DisableDefaultKeychain: true,
			Credentials:            []image.RegistryCredentials{{Authority: guarded, Username: "user", Password: "pass"}},
		})
		require.NoError(t, err)
		require.NotNil(t, img)
		require.NoError(t, img.Cleanup())
	})
}

func Test_RegistryProvider_failsOnTLSConfigurationErrors(t *testing.T) {
	// the registry is plain HTTP and needs no credentials, so the pull would succeed if the broken TLS
	// settings were only logged
	registryHost := makeRegistry(t)
	pushRandomRegistryImage(t, registryHost, "my-image", "the-tag")
	imageStr := fmt.Sprintf("%s/my-image:the-tag", registryHost)

	t.Setenv("DOCKER_CONFIG", t.TempDir())

	dir := t.TempDir()
	notPEM := filepath.Join(dir, "not-a-cert.pem")
	require.NoError(t, os.WriteFile(notPEM, []byte("this is not a certificate"), 0o600))

	tests := []struct {
		name    string
		options image.RegistryOptions
		wantErr string
	}{
		{
			name:    "CA path does not exist",
			options: image.RegistryOptions{CAFileOrDir: filepath.Join(dir, "missing.pem")},
			wantErr: "unable to stat",
		},
		{
			name:    "CA file is not PEM",
			options: image.RegistryOptions{CAFileOrDir: notPEM},
			wantErr: "failed to append certificates from PEM file",
		},
		{
			name:    "CA directory holds a file that is not PEM",
			options: image.RegistryOptions{CAFileOrDir: dir},
			wantErr: "failed to append certificates from PEM file",
		},
		{
			name: "client certificate does not exist",
			options: image.RegistryOptions{
				Credentials: []image.RegistryCredentials{{
					Authority:  registryHost,
					ClientCert: filepath.Join(dir, "missing.crt"),
					ClientKey:  filepath.Join(dir, "missing.key"),
				}},
			},
			wantErr: "unable to configure TLS client config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := file.TempDirGenerator{}
			t.Cleanup(func() { _ = generator.Cleanup() })

			img, err := NewRegistryProvider(&generator, tt.options, imageStr, nil).Provide(context.Background())
			assert.Nil(t, img)
			require.ErrorContains(t, err, "unable to configure TLS transport")
			require.ErrorContains(t, err, tt.wantErr)

			ref, err := ParseRegistryReference(imageStr, tt.options)
			require.NoError(t, err)

			options, err := RemoteOptions(context.Background(), ref, tt.options, nil)
			assert.Nil(t, options)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}

	t.Run("a missing CA path is a wrapped not-exist error", func(t *testing.T) {
		ref, err := ParseRegistryReference(imageStr, image.RegistryOptions{})
		require.NoError(t, err)

		_, err = RemoteOptions(context.Background(), ref, image.RegistryOptions{CAFileOrDir: filepath.Join(dir, "missing.pem")}, nil)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
