package oci

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistryV1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// RegistryImageProvider is an image.Provider capable of fetching and representing a container image fetched from a remote registry (described by the OCI distribution spec).
type RegistryImageProvider struct {
	imageStr        string
	tmpDirGen       *file.TempDirGenerator
	registryOptions image.RegistryOptions
	platform        *image.Platform
}

// NewProviderFromRegistry creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromRegistry(imgStr string, tmpDirGen *file.TempDirGenerator, registryOptions image.RegistryOptions, platform *image.Platform) *RegistryImageProvider {
	return &RegistryImageProvider{
		imageStr:        imgStr,
		tmpDirGen:       tmpDirGen,
		registryOptions: registryOptions,
		platform:        platform,
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

	options := prepareRemoteOptions(ctx, ref, p.registryOptions, p.platform)

	descriptor, err := remote.Get(ref, options...)
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

	if p.platform != nil {
		metadata = append(metadata,
			image.WithArchitecture(p.platform.Architecture, p.platform.Variant),
			image.WithOS(p.platform.OS),
		)
	}

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)

	return image.New(img, p.tmpDirGen, imageTempDir, metadata...), nil
}

func prepareReferenceOptions(registryOptions image.RegistryOptions) []name.Option {
	var options []name.Option
	if registryOptions.InsecureUseHTTP {
		options = append(options, name.Insecure)
	}
	return options
}

func prepareRemoteOptions(ctx context.Context, ref name.Reference, registryOptions image.RegistryOptions, p *image.Platform) (options []remote.Option) {
	options = append(options, remote.WithContext(ctx))

	if p != nil {
		options = append(options, remote.WithPlatform(containerregistryV1.Platform{
			Architecture: p.Architecture,
			OS:           p.OS,
			Variant:      p.Variant,
		}))
	}

	registryName := ref.Context().RegistryStr()

	// note: the authn.Authenticator and authn.Keychain options are mutually exclusive, only one may be provided.
	// If no explicit authenticator can be found, check if explicit Keychain has been provided, and if not, then
	// fallback to the default keychain. With the authenticator also comes the option to configure TLS transport.
	authenticator := registryOptions.Authenticator(registryName)
	tlsOptions := registryOptions.TLSOptions(registryName, registryOptions.InsecureSkipTLSVerify)

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

	if tlsOptions != nil {
		tlsConfig, err := getTLSConfig(registryOptions, *tlsOptions)
		if err != nil {
			log.Warn("unable to configure TLS transport: %w", err)
		} else {
			options = append(options, remote.WithTransport(&http.Transport{
				TLSClientConfig: tlsConfig,
			}))
		}
	}

	return options
}

func getTLSConfig(registryOptions image.RegistryOptions, tlsOptions tlsconfig.Options) (*tls.Config, error) {
	// note: tlsOptions allows for CAFile, however, this doesn't allow us to provide possibly multiple CA certs
	// to the underlying root pool. In order to support this we need to do the work to load the certs ourselves.
	tlsConfig, err := tlsconfig.Client(tlsOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to configure TLS client config: %w", err)
	}

	if !registryOptions.InsecureSkipTLSVerify && registryOptions.CAFileOrDir != "" {
		fi, err := os.Stat(registryOptions.CAFileOrDir)
		if err != nil {
			return nil, fmt.Errorf("unable to stat %q: %w", registryOptions.CAFileOrDir, err)
		}
		// load all the files in the directory as CA certs
		rootCAs := tlsConfig.RootCAs
		if rootCAs == nil {
			rootCAs, err = tlsconfig.SystemCertPool()
			if err != nil {
				log.Warnf("unable to load system cert pool: %w", err)
				rootCAs = x509.NewCertPool()
			}
		}

		var files []string
		if fi.IsDir() {
			// glob all *.crt, *.pem, and *.cert files in the directory
			var err error

			files, err = doublestar.Glob(os.DirFS("."), filepath.Join(registryOptions.CAFileOrDir, "*.{crt,pem,cert}"))
			if err != nil {
				return nil, fmt.Errorf("unable to find certs in %q: %w", registryOptions.CAFileOrDir, err)
			}
		} else {
			files = []string{registryOptions.CAFileOrDir}
		}

		for _, certFile := range files {
			log.Tracef("loading CA certificate from %q", certFile)
			pem, err := os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("could not read CA certificate %q: %v", certFile, err)
			}
			if !rootCAs.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("failed to append certificates from PEM file: %q", certFile)
			}
		}

		tlsConfig.RootCAs = rootCAs
	}

	return tlsConfig, nil
}
