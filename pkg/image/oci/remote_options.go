package oci

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/anchore/stereoscope/pkg/image"
)

// ParseRegistryReference parses an image reference the way the registry provider does, so that a
// registry reached with image.RegistryOptions.InsecureUseHTTP is addressed over plain HTTP.
func ParseRegistryReference(imageStr string, registryOptions image.RegistryOptions) (name.Reference, error) {
	return name.ParseReference(imageStr, prepareReferenceOptions(registryOptions)...)
}

// RemoteOptions returns the go-containerregistry options the registry provider uses to reach the
// registry of the given reference: the context, the user agent, the credentials and TLS settings
// selected from registryOptions for that registry, and a transport that resumes blob downloads
// dropped mid-stream (unless image.RegistryOptions.DisableBlobResume is set), along with any retry
// and timeout settings. It is meant for a caller that makes registry calls the provider does not
// (listing tags or a HEAD on a manifest, say) and needs them to authenticate and connect exactly as
// a pull would.
//
// The reference should come from ParseRegistryReference. A nil platform leaves the platform to
// go-containerregistry, whereas the provider picks a default one when resolving an image index.
//
// An error is returned when the TLS settings cannot be loaded, such as a CA file or client
// certificate that is missing or does not parse.
//
// The options are specific to the registry of ref and should not be reused for another. They
// already carry a transport and either an authenticator or a keychain, so credentials belong in
// registryOptions: appending remote.WithTransport discards the transport configured here, and
// go-containerregistry rejects an authenticator and a keychain given together.
func RemoteOptions(ctx context.Context, ref name.Reference, registryOptions image.RegistryOptions, platform *image.Platform) ([]remote.Option, error) {
	return prepareRemoteOptions(ctx, ref, registryOptions, platform, newEffectiveURLTransport(nil))
}

// retryOptions overrides go-containerregistry's retry of a failed request only where the caller asked
// for something else, so that its defaults, and any change to them upstream, apply otherwise.
func retryOptions(registryOptions image.RegistryOptions) []remote.Option {
	if registryOptions.RetryAttempts <= 0 && registryOptions.RetryBackoff <= 0 {
		return nil
	}

	// go-containerregistry does not export its default, and replacing the backoff replaces all of it
	backoff := remote.Backoff{
		Duration: time.Second,
		Factor:   3.0,
		Jitter:   0.1,
		Steps:    3,
	}
	if registryOptions.RetryAttempts > 0 {
		backoff.Steps = registryOptions.RetryAttempts
	}
	if registryOptions.RetryBackoff > 0 {
		backoff.Duration = registryOptions.RetryBackoff
	}

	return []remote.Option{remote.WithRetryBackoff(backoff)}
}

// applyTransportTimeouts sets the timeouts the caller asked for on the base transport, which the pull
// and any resumed blob request share, so both are held to them.
func applyTransportTimeouts(rt *http.Transport, registryOptions image.RegistryOptions) {
	if registryOptions.TLSHandshakeTimeout > 0 {
		rt.TLSHandshakeTimeout = registryOptions.TLSHandshakeTimeout
	}

	if registryOptions.ResponseHeaderTimeout > 0 {
		rt.ResponseHeaderTimeout = registryOptions.ResponseHeaderTimeout
	}

	if timeout := registryOptions.DialTimeout; timeout > 0 {
		// the inherited dialer is wrapped rather than replaced, so that whatever the process set up on
		// http.DefaultTransport (a custom resolver, say) still applies
		dial := rt.DialContext
		if dial == nil {
			dial = (&net.Dialer{}).DialContext
		}
		rt.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			// the deadline only governs the dial, so cancelling on return leaves the connection alone
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return dial(ctx, network, addr)
		}
	}
}
