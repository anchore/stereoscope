package oci

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	containerregistryV1Types "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

const Registry image.Source = image.OciRegistrySource

// NewRegistryProvider creates a new provider instance for a specific image that will later be cached to the given directory.
func NewRegistryProvider(tmpDirGen *file.TempDirGenerator, registryOptions image.RegistryOptions, imageStr string, platform *image.Platform) image.Provider {
	return &registryImageProvider{
		tmpDirGen:       tmpDirGen,
		imageStr:        imageStr,
		platform:        platform,
		registryOptions: registryOptions,
	}
}

// registryImageProvider is an image.Provider capable of fetching and representing a container image fetched from a remote registry (described by the OCI distribution spec).
type registryImageProvider struct {
	tmpDirGen       *file.TempDirGenerator
	imageStr        string
	platform        *image.Platform
	registryOptions image.RegistryOptions
}

func (p *registryImageProvider) Name() string {
	return Registry
}

// Provide an image object that represents the cached docker image tar fetched a registry.
func (p *registryImageProvider) Provide(ctx context.Context) (*image.Image, error) {
	log.Debugf("pulling image info directly from registry image=%q", p.imageStr)

	startTime := time.Now()
	imageTempDir, err := p.tmpDirGen.NewDirectory("oci-registry-image")
	if err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(p.imageStr, prepareReferenceOptions(p.registryOptions)...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry reference=%q: %+v", p.imageStr, err)
	}

	platform := defaultPlatformIfNil(p.platform)

	options := prepareRemoteOptions(ctx, ref, p.registryOptions, platform)

	descriptor, err := remote.Get(ref, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to get image descriptor from registry: %+v", err)
	}

	p.finalizePlatform(descriptor, &platform)

	img, err := descriptor.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to get image from registry: %+v", err)
	}

	c, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get image config from registry: %+v", err)
	}

	if err := validatePlatform(platform, c.OS, c.Architecture, c.Variant); err != nil {
		return nil, err
	}

	log.WithFields("image", p.imageStr, "time", time.Since(startTime)).Info("completed downloading manifest")

	// craft a repo digest from the registry reference and the known digest
	// note: the descriptor is fetched from the registry, and the descriptor digest is the same as the repo digest
	repoDigest := fmt.Sprintf("%s/%s@%s", ref.Context().RegistryStr(), ref.Context().RepositoryStr(), descriptor.Digest.String())

	metadata := []image.AdditionalMetadata{
		image.WithRepoDigests(repoDigest),
	}

	// make a best effort to get the manifest, should not block getting an image though if it fails
	if manifestBytes, err := img.RawManifest(); err == nil {
		metadata = append(metadata, image.WithManifest(manifestBytes))
	}

	if platform != nil {
		metadata = append(metadata,
			image.WithArchitecture(platform.Architecture, platform.Variant),
			image.WithOS(platform.OS),
		)
	}

	out := image.New(img, p.tmpDirGen, imageTempDir, metadata...)
	err = out.Read()
	if err != nil {
		cleanErr := out.Cleanup()
		return nil, errors.Join(err, cleanErr)
	}
	return out, err
}

func (p *registryImageProvider) finalizePlatform(descriptor *remote.Descriptor, platform **image.Platform) {
	if p.platform != nil {
		return
	}

	// no platform was specified by the user. There are two cases we want to cover:
	// 1. there is a manifest list, in which case we want to default the architecture to the host's architecture
	// 2. there is a single platform image, in which case we want to use that architecture (specify no default)
	switch descriptor.MediaType {
	case containerregistryV1Types.OCIManifestSchema1, containerregistryV1Types.DockerManifestSchema1, containerregistryV1Types.DockerManifestSchema2:
		// this is not for a multi-platform image, do not force the architecture if a platform was not specified explicitly by the user
		*platform = nil
		descriptor.Platform = nil
	}
}

func prepareReferenceOptions(registryOptions image.RegistryOptions) []name.Option {
	var options []name.Option
	if registryOptions.InsecureUseHTTP {
		log.Debug("HTTP transport is enabled for registry communication")
		options = append(options, name.Insecure)
	}
	return options
}

func prepareRemoteOptions(ctx context.Context, ref name.Reference, registryOptions image.RegistryOptions, p *image.Platform) (options []remote.Option) {
	options = append(options, remote.WithContext(ctx))

	// Set the user agent to indicate what binary is making the request
	// (e.g. syft, grype)
	options = append(options, remote.WithUserAgent(os.Args[0]))

	if p != nil {
		options = append(options, remote.WithPlatform(*toContainerRegistryPlatform(p)))
	}

	registryName := ref.Context().RegistryStr()

	// note: the authn.Authenticator and authn.Keychain options are mutually exclusive, only one may be provided.
	// If no explicit authenticator can be found, check if explicit Keychain has been provided, and if not, then
	// fallback to the default keychain. With the authenticator also comes the option to configure TLS transport.
	authenticator := registryOptions.Authenticator(registryName)

	switch {
	case authenticator != nil:
		options = append(options, remote.WithAuth(authenticator))
	case registryOptions.Keychain != nil:
		options = append(options, remote.WithAuthFromKeychain(registryOptions.Keychain))
	default:
		// use the Keychain specified from a docker config file.
		log.Debugf("no registry credentials configured for %q, using the default keychain", registryName)
		options = append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	tlsConfig, err := registryOptions.TLSConfig(registryName)
	if err != nil {
		log.Warn("unable to configure TLS transport: %w", err)
	} else if tlsConfig != nil {
		options = append(options, remote.WithTransport(getTransport(tlsConfig)))
	}

	return options
}

func getTransport(tlsConfig *tls.Config) *http.Transport {
	// use the default transport to inherit existing default options (including proxy options)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return transport
}
