package stereoscope

import (
	"context"
	"fmt"
	"runtime"

	"github.com/wagoodman/go-partybus"

	"github.com/anchore/go-logger"
	"github.com/anchore/stereoscope/internal/bus"
	dockerClient "github.com/anchore/stereoscope/internal/docker"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/internal/podman"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"github.com/anchore/stereoscope/pkg/image/sif"
)

var rootTempDirGenerator = file.NewTempDirGenerator("stereoscope")

func WithRegistryOptions(options image.RegistryOptions) Option {
	return func(c *config) error {
		c.Registry = options
		return nil
	}
}

func WithInsecureSkipTLSVerify() Option {
	return func(c *config) error {
		c.Registry.InsecureSkipTLSVerify = true
		return nil
	}
}

func WithInsecureAllowHTTP() Option {
	return func(c *config) error {
		c.Registry.InsecureUseHTTP = true
		return nil
	}
}

func WithCredentials(credentials ...image.RegistryCredentials) Option {
	return func(c *config) error {
		c.Registry.Credentials = append(c.Registry.Credentials, credentials...)
		return nil
	}
}

func WithAdditionalMetadata(metadata ...image.AdditionalMetadata) Option {
	return func(c *config) error {
		c.AdditionalMetadata = append(c.AdditionalMetadata, metadata...)
		return nil
	}
}

func WithPlatform(platform string) Option {
	return func(c *config) error {
		p, err := image.NewPlatform(platform)
		if err != nil {
			return err
		}
		c.Platform = p
		return nil
	}
}

// GetImageFromSource returns an image from the explicitly provided source.
func GetImageFromSource(ctx context.Context, imgStr string, source image.Source, options ...Option) (*image.Image, error) {
	log.Debugf("image: source=%+v location=%+v", source, imgStr)

	var cfg config
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, fmt.Errorf("unable to parse option: %w", err)
		}
	}

	provider, err := selectImageProvider(imgStr, source, cfg)
	if err != nil {
		return nil, err
	}

	img, err := provider.Provide(ctx, cfg.AdditionalMetadata...)
	if err != nil {
		return nil, fmt.Errorf("unable to use %s source: %w", source, err)
	}

	err = img.Read()
	if err != nil {
		return nil, fmt.Errorf("could not read image: %+v", err)
	}

	return img, nil
}

func selectImageProvider(imgStr string, source image.Source, cfg config) (image.Provider, error) {
	var provider image.Provider
	tempDirGenerator := rootTempDirGenerator.NewGenerator()

	if err := setPlatform(source, &cfg, runtime.GOARCH); err != nil {
		return nil, err
	}

	switch source {
	case image.DockerTarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = docker.NewProviderFromTarball(imgStr, tempDirGenerator)
	case image.DockerDaemonSource:
		c, err := dockerClient.GetClient()
		if err != nil {
			return nil, err
		}
		provider, err = docker.NewProviderFromDaemon(imgStr, tempDirGenerator, c, cfg.Platform)
		if err != nil {
			return nil, err
		}
	case image.PodmanDaemonSource:
		c, err := podman.GetClient()
		if err != nil {
			return nil, err
		}
		provider, err = docker.NewProviderFromDaemon(imgStr, tempDirGenerator, c, cfg.Platform)
		if err != nil {
			return nil, err
		}
	case image.OciDirectorySource:
		provider = oci.NewProviderFromPath(imgStr, tempDirGenerator)
	case image.OciTarballSource:
		provider = oci.NewProviderFromTarball(imgStr, tempDirGenerator)
	case image.OciRegistrySource:
		provider = oci.NewProviderFromRegistry(imgStr, tempDirGenerator, cfg.Registry, cfg.Platform)
	case image.SingularitySource:
		provider = sif.NewProviderFromPath(imgStr, tempDirGenerator)
	default:
		return nil, fmt.Errorf("unable to determine image source")
	}
	return provider, nil
}

func setPlatform(source image.Source, cfg *config, defaultArch string) error {
	// we should override the platform based on the host architecture if the user did not specify a platform
	// see https://github.com/anchore/stereoscope/issues/149 for more details
	defaultPlatform, err := image.NewPlatform(defaultArch)
	if err != nil {
		log.WithFields("error", err).Warnf("unable to set default platform to %q", runtime.GOARCH)
		defaultPlatform = nil
	}

	switch source {
	case image.DockerTarballSource, image.OciDirectorySource, image.OciTarballSource, image.SingularitySource:
		if cfg.Platform != nil {
			return fmt.Errorf("specified platform=%q however image source=%q does not support selecting platform", cfg.Platform.String(), source.String())
		}

	case image.DockerDaemonSource, image.PodmanDaemonSource, image.OciRegistrySource:
		if cfg.Platform == nil {
			cfg.Platform = defaultPlatform
		}

	default:
		return fmt.Errorf("unable to determine image source to select platform")
	}
	return nil
}

// GetImage parses the user provided image string and provides an image object;
// note: the source where the image should be referenced from is automatically inferred.
func GetImage(ctx context.Context, userStr string, options ...Option) (*image.Image, error) {
	source, imgStr, err := image.DetectSource(userStr)
	if err != nil {
		return nil, err
	}
	return GetImageFromSource(ctx, imgStr, source, options...)
}

func SetLogger(logger logger.Logger) {
	log.Log = logger
}

func SetBus(b *partybus.Bus) {
	bus.SetPublisher(b)
}

// Cleanup deletes all directories created by stereoscope calls.
// Deprecated: please use image.Image.Cleanup() over this.
func Cleanup() {
	if err := rootTempDirGenerator.Cleanup(); err != nil {
		log.Errorf("failed to cleanup tempdir root: %w", err)
	}
}
