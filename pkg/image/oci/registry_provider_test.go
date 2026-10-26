package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-containerregistry/pkg/authn"
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
	isFetchError := func(t require.TestingT, err error, args ...any) {
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

// truncatingProxy forwards to upstream, tearing the connection down partway through the first
// `drops` responses to each blob, so that the read has to be resumed repeatedly. more than one
// drop matters: a single splice exercises neither the budgets nor a second pass through
// verify.ReadCloser's running hash. it returns the proxy's host and a count of the truncations it
// has made.
func truncatingProxy(t *testing.T, upstream string, drops int) (string, func() int) {
	t.Helper()

	var mu sync.Mutex
	truncated := make(map[string]int)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://"+upstream+r.URL.RequestURI(), nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()

		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)

		mu.Lock()
		truncate := r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") &&
			len(body) > 1 && truncated[r.URL.Path] < drops
		if truncate {
			truncated[r.URL.Path]++
		}
		mu.Unlock()

		if truncate {
			_, _ = w.Write(body[:len(body)/2])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}

		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	count := func() int {
		mu.Lock()
		defer mu.Unlock()

		total := 0
		for _, n := range truncated {
			total += n
		}
		return total
	}

	return strings.TrimPrefix(srv.URL, "http://"), count
}

func Test_RegistryProvider_resumesAgainstAGoContainerRegistryServer(t *testing.T) {
	// the resumable transport under the real go-containerregistry stack (remote.Get, the retry
	// transport, verify.ReadCloser's digest check, and the reopen client built by reopenClient)
	// against go-containerregistry's own registry, which answers 416 to an open-ended range. that
	// registry is anonymous, so no bearer token is exercised here; the unit tests cover a reopen
	// re-authorizing
	upstream := makeRegistry(t)
	pushRandomRegistryImage(t, upstream, "resume-probe", "latest")

	generator := file.TempDirGenerator{}
	defer func() { _ = generator.Cleanup() }()

	// two drops per blob, so the running digest is exercised across more than one splice. each is
	// a stall on blobs this small, so more would only add backoff
	proxy, truncations := truncatingProxy(t, upstream, 2)
	imageStr := fmt.Sprintf("%s/resume-probe:latest", proxy)
	provider := NewRegistryProvider(&generator, image.RegistryOptions{InsecureUseHTTP: true}, imageStr, nil)

	img, err := provider.Provide(context.TODO())
	require.NoError(t, err, "the layer must survive a mid-stream drop and pass the digest check")
	require.NotNil(t, img)
	t.Cleanup(func() { _ = img.Cleanup() })

	// two layers and a config, and every drop asked for must actually have happened, or this test
	// passes without exercising a resume at all
	assert.Equal(t, 3*2, truncations())
}

func Test_RegistryProvider_blobResumeCanBeDisabled(t *testing.T) {
	// a caller meeting a registry that mishandles range requests needs a way to turn resuming off
	ref, err := name.ParseReference("registry.example.com/img:latest")
	require.NoError(t, err)

	effective := newEffectiveURLTransport(nil)

	prepareRemoteOptions(context.Background(), ref, image.RegistryOptions{}, nil, effective)
	assert.IsType(t, &resumableTransport{}, effective.base, "resuming is on by default")

	prepareRemoteOptions(context.Background(), ref, image.RegistryOptions{DisableBlobResume: true}, nil, effective)
	assert.IsType(t, &http.Transport{}, effective.base, "DisableBlobResume removes it from the chain")
}

func Test_reopenClient_aWaitingResumeCanBeCancelled(t *testing.T) {
	// a resume waiting on another's client build must still honour its own Close or cancellation,
	// which a mutex held across the build would not
	ref, err := name.ParseReference("registry.example.com/img:latest")
	require.NoError(t, err)

	building := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	clientFor := reopenClient(ref, http.DefaultTransport, "stereoscope-test", func(context.Context) (authn.Authenticator, error) {
		close(building)
		<-release
		return nil, errors.New("never built")
	})

	go func() { _, _ = clientFor(context.Background()) }()
	<-building

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := clientFor(ctx)
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled resume kept waiting on another's client build")
	}
}

func makeRegistry(t *testing.T) (registryHost string) {
	memoryBlobHandler := registry.NewInMemoryBlobHandler()
	registryInstance := registry.New(registry.WithBlobHandler(memoryBlobHandler))
	ts := httptest.NewServer(http.HandlerFunc(registryInstance.ServeHTTP))
	t.Cleanup(ts.Close)
	return strings.TrimPrefix(ts.URL, "http://")
}

func TestEffectiveURLTransport(t *testing.T) {
	tests := []struct {
		name           string
		originalHost   string
		redirectHost   string
		expectRedirect bool
	}{
		{
			name:           "no redirect",
			originalHost:   "example.com",
			redirectHost:   "example.com",
			expectRedirect: false,
		},
		{
			name:           "with redirect",
			originalHost:   "old.example.com",
			redirectHost:   "new.example.com",
			expectRedirect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newEffectiveURLTransport(nil)

			// Create a mock request
			req := &http.Request{
				URL: &url.URL{Host: tt.originalHost},
			}

			// Create a mock response with effective URL
			resp := &http.Response{
				Request: &http.Request{
					URL: &url.URL{Host: tt.redirectHost},
				},
			}

			// Mock the base transport
			transport.base = &mockRoundTripper{resp: resp}

			// Make the request
			_, err := transport.RoundTrip(req)
			require.NoError(t, err)

			// Check effective host
			effectiveHost := transport.getEffectiveHost(tt.originalHost)
			if tt.expectRedirect {
				assert.Equal(t, tt.redirectHost, effectiveHost)
			} else {
				assert.Equal(t, tt.originalHost, effectiveHost)
			}
		})
	}
}

func TestRegistryProviderWithRedirect(t *testing.T) {
	// Create a test server that redirects
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the actual registry
		registryHost := makeRegistry(t)
		redirectURL := fmt.Sprintf("http://%s%s", registryHost, r.URL.Path)
		http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	// Create a registry provider with redirect server
	tmpDirGen := file.NewTempDirGenerator("oci-test")
	defer tmpDirGen.Cleanup()

	redirectHost := strings.TrimPrefix(redirectServer.URL, "http://")
	imageStr := fmt.Sprintf("%s/test:latest", redirectHost)

	provider := NewRegistryProvider(tmpDirGen, image.RegistryOptions{InsecureUseHTTP: true}, imageStr, nil)

	// Test that the provider handles redirects gracefully
	// Note: This test may not fully verify redirect behavior with a real image
	// due to complexity of setting up a complete registry with redirects,
	// but it ensures our code doesn't break with the redirect transport
	assert.NotNil(t, provider)
	assert.Equal(t, Registry, provider.Name())
}

// mockRoundTripper is a helper for testing
type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}
