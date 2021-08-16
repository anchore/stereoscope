package oci

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// RegistryImageProvider is a image.Provider capable of fetching and representing a container image fetched from a remote registry (described by the OCI distribution spec).
type RegistryImageProvider struct {
	imageStr        string
	tmpDirGen       *file.TempDirGenerator
	registryOptions *image.RegistryOptions
}

// NewProviderFromRegistry creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromRegistry(imgStr string, tmpDirGen *file.TempDirGenerator, registryOptions *image.RegistryOptions) *RegistryImageProvider {
	return &RegistryImageProvider{
		imageStr:        imgStr,
		tmpDirGen:       tmpDirGen,
		registryOptions: registryOptions,
	}
}

// Provide an image object that represents the cached docker image tar fetched a registry.
func (p *RegistryImageProvider) Provide() (*image.Image, error) {
	log.Debugf("pulling image info directly from registry image=%q", p.imageStr)

	imageTempDir, err := p.tmpDirGen.NewTempDir()
	if err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(p.imageStr, prepareReferenceOptions(p.registryOptions)...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse registry reference=%q: %+v", p.imageStr, err)
	}

	descriptor, err := remote.Get(ref, prepareRemoteOptions(ref, p.registryOptions)...)
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
		image.WithRepoDigests([]string{repoDigest}),
	}

	// make a best effort to get the manifest, should not block getting an image though if it fails
	if manifestBytes, err := img.RawManifest(); err == nil {
		metadata = append(metadata, image.WithManifest(manifestBytes))
	}

	return image.NewImage(img, imageTempDir, metadata...), nil
}

func prepareReferenceOptions(registryOptions *image.RegistryOptions) []name.Option {
	var options []name.Option
	if registryOptions.InsecureUseHTTP {
		options = append(options, name.Insecure)
	}
	return options
}

func prepareRemoteOptions(ref name.Reference, registryOptions *image.RegistryOptions) []remote.Option {
	var opts []remote.Option
	if registryOptions.InsecureSkipTLSVerify {
		t := &http.Transport{
			// nolint: gosec
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		opts = append(opts, remote.WithTransport(t))
	}

	// note: the authn.Authenticator and authn.Keychain options are mutually exclusive, only one may be provided.
	// If no explicit authenticator can be found, then fallback to the keychain.
	authenticator := registryOptions.Authenticator(ref.Context().RegistryStr())
	if authenticator != nil {
		opts = append(opts, remote.WithAuth(authenticator))
	} else {
		// use the Keychain specified from a docker config file.
		log.Debugf("no registry credentials configured, using the default keychain")
		opts = append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}

	return opts
}
