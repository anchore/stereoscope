package oci

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

func Test_retryOptions(t *testing.T) {
	// the zero value must add nothing at all, which is what leaves go-containerregistry's own retry
	// in place. negative values are not a way to ask for something new
	assert.Nil(t, retryOptions(image.RegistryOptions{}))
	assert.Nil(t, retryOptions(image.RegistryOptions{RetryAttempts: -1, RetryBackoff: -time.Second}))

	assert.Len(t, retryOptions(image.RegistryOptions{RetryAttempts: 1}), 1)
	assert.Len(t, retryOptions(image.RegistryOptions{RetryBackoff: time.Millisecond}), 1)
}

func Test_RemoteOptions_retry(t *testing.T) {
	tests := []struct {
		name         string
		options      image.RegistryOptions
		wantRequests int64
	}{
		{
			name:         "a single attempt disables retry",
			options:      image.RegistryOptions{RetryAttempts: 1},
			wantRequests: 1,
		},
		{
			name:         "attempts are counted in total",
			options:      image.RegistryOptions{RetryAttempts: 4, RetryBackoff: time.Millisecond},
			wantRequests: 4,
		},
		{
			// one second then three would pass here too, only slowly, so the wait is bounded below
			name:         "a backoff alone keeps the default number of attempts",
			options:      image.RegistryOptions{RetryBackoff: time.Millisecond},
			wantRequests: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// the registry answers the ping and is then unavailable for the manifest, a status
			// go-containerregistry retries
			var manifestRequests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v2/" {
					w.WriteHeader(http.StatusOK)
					return
				}
				manifestRequests.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(srv.Close)

			t.Setenv("DOCKER_CONFIG", t.TempDir())

			ref, err := ParseRegistryReference(strings.TrimPrefix(srv.URL, "http://")+"/my-image:the-tag", tt.options)
			require.NoError(t, err)

			options, err := RemoteOptions(context.Background(), ref, tt.options, nil)
			require.NoError(t, err)

			start := time.Now()
			_, err = remote.Head(ref, options...)

			var transportErr *transport.Error
			require.ErrorAs(t, err, &transportErr)
			assert.Equal(t, http.StatusServiceUnavailable, transportErr.StatusCode)
			assert.Equal(t, tt.wantRequests, manifestRequests.Load())
			assert.Less(t, time.Since(start), 900*time.Millisecond, "the configured backoff was not the one waited on")
		})
	}
}

func Test_applyTransportTimeouts(t *testing.T) {
	t.Run("the zero value changes nothing", func(t *testing.T) {
		want := getTransport(nil)
		got := getTransport(nil)
		applyTransportTimeouts(got, image.RegistryOptions{})

		if d := cmp.Diff(want, got,
			cmpopts.IgnoreFields(http.Transport{}, "Proxy", "DialContext", "TLSNextProto"),
			cmpopts.IgnoreUnexported(http.Transport{})); d != "" {
			t.Errorf("unexpected transport change (-want +got):\n%s", d)
		}

		// functions cannot be compared for equality, only for identity
		assert.Equal(t, funcPointer(want.DialContext), funcPointer(got.DialContext), "the dialer was replaced")
	})

	t.Run("timeouts are set on the transport", func(t *testing.T) {
		rt := getTransport(nil)
		applyTransportTimeouts(rt, image.RegistryOptions{
			TLSHandshakeTimeout:   3 * time.Second,
			ResponseHeaderTimeout: 7 * time.Second,
		})

		assert.Equal(t, 3*time.Second, rt.TLSHandshakeTimeout)
		assert.Equal(t, 7*time.Second, rt.ResponseHeaderTimeout)
	})

	t.Run("the dial timeout bounds the inherited dialer", func(t *testing.T) {
		rt := getTransport(nil)
		var dialed string
		rt.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
			dialed = addr
			<-ctx.Done()
			return nil, ctx.Err()
		}
		applyTransportTimeouts(rt, image.RegistryOptions{DialTimeout: 20 * time.Millisecond})

		start := time.Now()
		_, err := rt.DialContext(context.Background(), "tcp", "registry.example.com:443")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, "registry.example.com:443", dialed, "the inherited dialer was not the one called")
		assert.Less(t, time.Since(start), 5*time.Second)
	})
}

func Test_RemoteOptions_responseHeaderTimeout(t *testing.T) {
	// a registry that accepts the request and never answers is otherwise only ended by the context
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	t.Setenv("DOCKER_CONFIG", t.TempDir())

	registryOptions := image.RegistryOptions{
		ResponseHeaderTimeout: 50 * time.Millisecond,
		RetryAttempts:         1,
	}

	ref, err := ParseRegistryReference(strings.TrimPrefix(srv.URL, "http://")+"/my-image:the-tag", registryOptions)
	require.NoError(t, err)

	options, err := RemoteOptions(context.Background(), ref, registryOptions, nil)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := remote.Head(ref, options...)
		done <- err
	}()

	select {
	case err := <-done:
		// matched on text because it is the ping that fails here, which go-containerregistry reports
		// as the error from each scheme it tried, and the first net.Error among them is the https
		// attempt being refused rather than the timeout
		require.ErrorContains(t, err, "timeout awaiting response headers")
	case <-time.After(10 * time.Second):
		t.Fatal("the request was not bounded by ResponseHeaderTimeout")
	}
}

func funcPointer(f any) uintptr {
	return reflect.ValueOf(f).Pointer()
}
