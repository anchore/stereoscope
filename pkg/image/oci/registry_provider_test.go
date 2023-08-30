package oci

import (
	"context"
	"crypto/x509"
	"os"
	"reflect"
	"testing"

	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func Test_NewProviderFromRegistry(t *testing.T) {
	//GIVEN
	imageStr := "image"
	generator := file.TempDirGenerator{}
	options := image.RegistryOptions{}
	platform := &image.Platform{}

	//WHEN
	provider := NewProviderFromRegistry(imageStr, &generator, options, platform)

	//THEN
	assert.NotNil(t, provider.imageStr)
	assert.NotNil(t, provider.tmpDirGen)
	assert.NotNil(t, provider.registryOptions)
	assert.NotNil(t, provider.platform)
}

func Test_Registry_Provide_FailsUnauthorized(t *testing.T) {
	//GIVEN
	imageStr := "image"
	generator := file.TempDirGenerator{}
	options := image.RegistryOptions{
		InsecureSkipTLSVerify: true,
		Credentials: []image.RegistryCredentials{
			{
				Authority: "index.docker.io",
				Token:     "token",
			},
		},
	}
	platform := &image.Platform{}
	provider := NewProviderFromRegistry(imageStr, &generator, options, platform)
	ctx := context.Background()

	//WHEN
	result, err := provider.Provide(ctx)

	//THEN
	assert.Nil(t, result)
	assert.Error(t, err)
}

func Test_Registry_Provide_FailsImageMissingPlatform(t *testing.T) {
	//GIVEN
	imageStr := "docker.io/golang:1.18"
	generator := file.TempDirGenerator{}
	options := image.RegistryOptions{
		InsecureSkipTLSVerify: true,
	}
	platform := &image.Platform{}
	provider := NewProviderFromRegistry(imageStr, &generator, options, platform)
	ctx := context.Background()

	//WHEN
	result, err := provider.Provide(ctx)

	//THEN
	assert.Nil(t, result)
	assert.Error(t, err)
}

func Test_Registry_Provide(t *testing.T) {
	//GIVEN
	imageStr := "golang:1.18"
	generator := file.TempDirGenerator{}
	options := image.RegistryOptions{
		InsecureSkipTLSVerify: true,
	}
	platform := &image.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
	provider := NewProviderFromRegistry(imageStr, &generator, options, platform)
	ctx := context.Background()

	//WHEN
	result, err := provider.Provide(ctx)

	//THEN
	assert.NotNil(t, result)
	assert.NoError(t, err)
}

func Test_prepareReferenceOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    image.RegistryOptions
		expected []name.Option
	}{
		{
			name:     "not InsecureUseHTTP",
			input:    image.RegistryOptions{},
			expected: nil,
		},
		{
			name: "use InsecureUseHTTP",
			input: image.RegistryOptions{
				InsecureUseHTTP: true,
			},
			expected: []name.Option{name.Insecure},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := prepareReferenceOptions(test.input)
			assert.Equal(t, len(test.expected), len(out))
			if test.expected == nil {
				assert.Equal(t, test.expected, out)
			} else {
				// cannot compare functions directly
				e1 := reflect.ValueOf(test.expected[0])
				e2 := reflect.ValueOf(out[0])
				assert.Equal(t, e1, e2)
			}
		})
	}
}

func Test_getTLSConfig_rootCAs(t *testing.T) {
	certFile := "test-fixtures/certs/server.crt"
	systemCerts, err := tlsconfig.SystemCertPool()
	require.NoError(t, err)

	certPool := systemCerts.Clone()

	pem, err := os.ReadFile(certFile)
	require.NoError(t, err)
	certPool.AppendCertsFromPEM(pem)

	tests := []struct {
		name            string
		registryOptions image.RegistryOptions
		tlsOptions      tlsconfig.Options
		want            *x509.CertPool
		wantErr         assert.ErrorAssertionFunc
	}{
		{
			name: "add single root cert",
			registryOptions: image.RegistryOptions{
				CAFileOrDir: certFile,
			},
			want: certPool,
		},
		{
			name: "add root certs from dir",
			registryOptions: image.RegistryOptions{
				CAFileOrDir: "test-fixtures/certs",
			},
			want: certPool,
		},
		{
			name: "skip TLS verify does not load certs", // just like the stdlib
			registryOptions: image.RegistryOptions{
				InsecureSkipTLSVerify: true,
				CAFileOrDir:           certFile,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getTLSConfig(tt.registryOptions, tt.tlsOptions)
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got.RootCAs))
		})
	}
}
