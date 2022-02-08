package oci

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/containerd/containerd/platforms"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistryV1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// RegistryImageProvider is an image.Provider capable of fetching and representing a container image fetched from a remote registry (described by the OCI distribution spec).
type RegistryImageProvider struct {
	imageStr        string
	tmpDirGen       *file.TempDirGenerator
	registryOptions image.RegistryOptions
}

// NewProviderFromRegistry creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromRegistry(imgStr string, tmpDirGen *file.TempDirGenerator, registryOptions image.RegistryOptions) *RegistryImageProvider {
	return &RegistryImageProvider{
		imageStr:        imgStr,
		tmpDirGen:       tmpDirGen,
		registryOptions: registryOptions,
	}
}

// Provide an image object that represents the cached docker image tar fetched a registry.
func (p *RegistryImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	log.Debugf("pulling image info directly from registry image=%q", p.imageStr)

	imageTempDir, err := p.tmpDirGen.NewDirectory("oci-registry-image")
	if err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(p.imageStr, prepareReferenceOptions(p.registryOptions)...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry reference=%q: %+v", p.imageStr, err)
	}

	remoteOptions, err := prepareRemoteOptions(ctx, ref, p.registryOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get registry options: %+v", err)
	}

	descriptor, err := remote.Get(ref, remoteOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to get image descriptor from registry: %+v", err)
	}

	img, err := descriptor.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to get image from registry: %+v", err)
	}

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

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)

	return image.NewImage(img, imageTempDir, metadata...), nil
}

func prepareReferenceOptions(registryOptions image.RegistryOptions) []name.Option {
	var options []name.Option
	if registryOptions.InsecureUseHTTP {
		options = append(options, name.Insecure)
	}
	return options
}

func prepareRemoteOptions(ctx context.Context, ref name.Reference, registryOptions image.RegistryOptions) ([]remote.Option, error) {
	var options []remote.Option

	if registryOptions.InsecureSkipTLSVerify {
		t := &http.Transport{
			// nolint: gosec
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		options = append(options, remote.WithTransport(t))
	}

	if registryOptions.Platform != "" {
		// registry library doesn't support parsing this, so containerd implemetation is used
		// wich in turns requires struct remapping
		p, err := platforms.Parse(registryOptions.Platform)
		if err != nil {
			err = fmt.Errorf("failed to parse platform %q: %w", registryOptions.Platform, err)
			return nil, err
		}
		options = append(options, remote.WithPlatform(containerregistryV1.Platform{
			Architecture: p.Architecture,
			OS:           p.OS,
			OSVersion:    p.OSVersion,
			OSFeatures:   p.OSFeatures,
			Variant:      p.Variant,
		}))
	}
	options = append(options, remote.WithContext(ctx))

	// note: the authn.Authenticator and authn.Keychain options are mutually exclusive, only one may be provided.
	// If no explicit authenticator can be found, then fallback to the keychain.
	authenticator := registryOptions.Authenticator(ref.Context().RegistryStr())
	if authenticator != nil {
		options = append(options, remote.WithAuth(authenticator))
	} else {
		// use the Keychain specified from a docker config file.
		log.Debugf("no registry credentials configured, using the default keychain")
		options = append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	return options, nil
}
