package oci

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// makeBasicAuthRegistry puts basic auth (user / pass) in front of a registry made by makeRegistry. It
// returns the guarded host and the open one behind it, which a test pushes to.
func makeBasicAuthRegistry(t *testing.T) (registryHost, upstreamHost string) {
	t.Helper()

	upstreamHost = makeRegistry(t)
	upstream, err := http.NewRequest(http.MethodGet, "http://"+upstreamHost, nil)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "user" || pass != "pass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`))
			return
		}

		out := r.Clone(r.Context())
		out.RequestURI = ""
		out.URL.Scheme = upstream.URL.Scheme
		out.URL.Host = upstream.URL.Host

		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)

	return strings.TrimPrefix(srv.URL, "http://"), upstreamHost
}

func Test_RegistryProvider_preservesTransportErrors(t *testing.T) {
	// a caller has to tell a rejected credential from a missing image without reading the message,
	// which only works while go-containerregistry's *transport.Error stays reachable in the chain
	guarded, upstream := makeBasicAuthRegistry(t)
	pushRandomRegistryImage(t, upstream, "my-image", "the-tag")

	tests := []struct {
		name       string
		imageStr   string
		options    image.RegistryOptions
		wantStatus int
		wantCode   transport.ErrorCode
	}{
		{
			name:     "rejected credentials",
			imageStr: fmt.Sprintf("%s/my-image:the-tag", guarded),
			options: image.RegistryOptions{
				Credentials: []image.RegistryCredentials{{Authority: guarded, Username: "user", Password: "wrong"}},
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   transport.UnauthorizedErrorCode,
		},
		{
			name:       "unknown manifest",
			imageStr:   fmt.Sprintf("%s/my-image:no-such-tag", upstream),
			wantStatus: http.StatusNotFound,
			wantCode:   transport.ManifestUnknownErrorCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := file.TempDirGenerator{}
			t.Cleanup(func() { _ = generator.Cleanup() })

			img, err := NewRegistryProvider(&generator, tt.options, tt.imageStr, nil).Provide(context.Background())
			require.Error(t, err)
			assert.Nil(t, img)

			var transportErr *transport.Error
			require.ErrorAs(t, err, &transportErr)
			assert.Equal(t, tt.wantStatus, transportErr.StatusCode)
			require.Len(t, transportErr.Errors, 1)
			assert.Equal(t, tt.wantCode, transportErr.Errors[0].Code)
		})
	}
}
