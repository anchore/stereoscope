package oci

import (
	"context"

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
// dropped mid-stream (unless image.RegistryOptions.DisableBlobResume is set). It is meant for a
// caller that makes registry calls the provider does not (listing tags or a HEAD on a manifest, say)
// and needs them to authenticate and connect exactly as a pull would.
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
