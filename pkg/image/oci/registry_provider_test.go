package oci

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func TestValidatePlatform(t *testing.T) {
	isFetchError := func(t require.TestingT, err error, args ...interface{}) {
		var pErr *image.ErrPlatformMismatch
		require.ErrorAs(t, err, &pErr)
	}
	tests := []struct {
		name           string
		platform       *image.Platform
		givenOs        string
		givenArch      string
		givenVariant   string
		expectedErrMsg string
		expectedErr    require.ErrorAssertionFunc
	}{
		{
			name:        "nil platform",
			platform:    nil,
			givenOs:     "linux",
			givenArch:   "amd64",
			expectedErr: require.NoError,
		},
		{
			name:           "missing OS",
			platform:       &image.Platform{OS: "linux", Architecture: "amd64"},
			givenOs:        "",
			givenArch:      "amd64",
			expectedErr:    isFetchError,
			expectedErrMsg: "missing architecture or OS",
		},
		{
			name:           "missing architecture",
			platform:       &image.Platform{OS: "linux", Architecture: "amd64"},
			givenOs:        "linux",
			givenArch:      "",
			expectedErr:    isFetchError,
			expectedErrMsg: "missing architecture or OS",
		},
		{
			name:           "invalid platform string",
			platform:       &image.Platform{OS: "linux", Architecture: "amd64"},
			givenOs:        "invalid/thing/place",
			givenArch:      "platform",
			expectedErr:    isFetchError,
			expectedErrMsg: "failed to parse platform from image config",
		},
		{
			name:           "mismatched platform",
			platform:       &image.Platform{OS: "linux", Architecture: "amd64"},
			givenOs:        "windows",
			givenArch:      "arm64",
			expectedErr:    isFetchError,
			expectedErrMsg: `image platform="windows/arm64" does not match user specified platform="linux/amd64"`,
		},
		{
			name:        "matching platform",
			platform:    &image.Platform{OS: "linux", Architecture: "amd64"},
			givenOs:     "linux",
			givenArch:   "amd64",
			expectedErr: require.NoError,
		},
		{
			name:         "matching platform with variant v7",
			platform:     &image.Platform{OS: "linux", Architecture: "arm", Variant: "v7"},
			givenOs:      "linux",
			givenArch:    "arm",
			givenVariant: "v7",
			expectedErr:  require.NoError,
		},
		{
			name:         "matching platform with variant arm64/v8",
			platform:     &image.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			givenOs:      "linux",
			givenArch:    "arm64",
			givenVariant: "v8",
			expectedErr:  require.NoError,
		},
		{
			name:           "mismatched variant",
			platform:       &image.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			givenOs:        "linux",
			givenArch:      "arm64",
			givenVariant:   "v7",
			expectedErr:    isFetchError,
			expectedErrMsg: `image platform="linux/arm64/v7" does not match user specified platform="linux/arm64/v8"`,
		},
		{
			name:           "user specifies variant, image does not have one",
			platform:       &image.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			givenOs:        "linux",
			givenArch:      "arm64",
			givenVariant:   "",
			expectedErr:    isFetchError,
			expectedErrMsg: `image platform="linux/arm64" does not match user specified platform="linux/arm64/v8"`,
		},
		{
			name:         "image has variant, user does not specify one",
			platform:     &image.Platform{OS: "linux", Architecture: "arm64"},
			givenOs:      "linux",
			givenArch:    "arm64",
			givenVariant: "v8",
			expectedErr:  require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlatform(tt.platform, tt.givenOs, tt.givenArch, tt.givenVariant)
			tt.expectedErr(t, err)
			if err != nil {
				assert.ErrorContains(t, err, tt.expectedErrMsg)
			}
		})
	}
}

func Test_RegistryProvider(t *testing.T) {
	imageName := "my-image"
	imageTag := "the-tag"

	registryHost := makeRegistry(t)
	pushRandomRegistryImage(t, registryHost, imageName, imageTag)

	generator := file.TempDirGenerator{}
	defer generator.Cleanup()

	options := image.RegistryOptions{}
	provider := NewRegistryProvider(&generator, options, fmt.Sprintf("%s/%s:%s", registryHost, imageName, imageTag), nil)
	img, err := provider.Provide(context.TODO())
	assert.NoError(t, err)
	assert.NotNil(t, img)
}

func Test_NewProviderFromRegistry(t *testing.T) {
	//GIVEN
	imageStr := "image"
	generator := file.TempDirGenerator{}
	defer generator.Cleanup()
	options := image.RegistryOptions{}
	platform := &image.Platform{}

	//WHEN
	provider := NewRegistryProvider(&generator, options, imageStr, platform).(*registryImageProvider)

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
	defer generator.Cleanup()
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
	provider := NewRegistryProvider(&generator, options, imageStr, platform)
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
	defer generator.Cleanup()
	options := image.RegistryOptions{
		InsecureSkipTLSVerify: true,
	}
	platform := &image.Platform{}
	provider := NewRegistryProvider(&generator, options, imageStr, platform)
	ctx := context.Background()

	//WHEN
	result, err := provider.Provide(ctx)

	//THEN
	assert.Nil(t, result)
	assert.Error(t, err)
}

func Test_DockerMainRegistry_Provide(t *testing.T) {
	//GIVEN
	imageStr := "alpine:3.17"
	generator := file.TempDirGenerator{}
	defer generator.Cleanup()
	options := image.RegistryOptions{
		InsecureSkipTLSVerify: true,
	}
	platform := &image.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
	provider := NewRegistryProvider(&generator, options, imageStr, platform)
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

func Test_getTransport_haxProxyCfg(t *testing.T) {
	defTransport := http.DefaultTransport.(*http.Transport)
	transport := getTransport(nil)

	assert.NotNil(t, transport.Proxy)
	assert.NotNil(t, transport.DialContext)

	if d := cmp.Diff(defTransport, transport,
		cmpopts.IgnoreFields(http.Transport{}, "TLSClientConfig", "Proxy", "DialContext", "TLSNextProto"),
		cmpopts.IgnoreUnexported(http.Transport{})); d != "" {
		t.Errorf("unexpected proxy config (-want +got):\n%s", d)
	}
}

func pushRandomRegistryImage(t *testing.T, registryHost, repo, tag string) {
	t.Helper()

	repoTag := repo + ":" + tag

	baseImg, err := random.Image(1024, 2)
	require.NoError(t, err)

	cfg, err := baseImg.ConfigFile()
	require.NoError(t, err)

	// match the default values that stereoscope uses
	cfg.OS = "linux"
	cfg.Architecture = runtime.GOARCH

	// update the image with the modified config with os/arch info
	img, err := mutate.ConfigFile(baseImg, cfg)
	require.NoError(t, err)

	opts := []name.Option{name.Insecure, name.WithDefaultRegistry(registryHost)}
	ref, err := name.ParseReference(repoTag, opts...)
	require.NoError(t, err)

	remoteopts := []remote.Option{remote.WithUserAgent("syft-test-util")}
	err = remote.Write(ref, img, remoteopts...)
	require.NoError(t, err)

	latestTag, err := name.NewTag(tag, opts...)
	require.NoError(t, err)
	err = remote.Tag(latestTag, img, remoteopts...)
	require.NoError(t, err)
}

func makeRegistry(t *testing.T) (registryHost string) {
	memoryBlobHandler := registry.NewInMemoryBlobHandler()
	registryInstance := registry.New(registry.WithBlobHandler(memoryBlobHandler))
	ts := httptest.NewServer(http.HandlerFunc(registryInstance.ServeHTTP))
	t.Cleanup(ts.Close)
	return strings.TrimPrefix(ts.URL, "http://")
}
