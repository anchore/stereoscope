package stereoscope

import (
	"context"
	"fmt"

	"github.com/anchore/stereoscope/internal/bus"
	dockerClient "github.com/anchore/stereoscope/internal/docker"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/internal/podman"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"github.com/anchore/stereoscope/pkg/logger"
	"github.com/wagoodman/go-partybus"
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

// GetImageFromSource returns an image from the explicitly provided source.
func GetImageFromSource(ctx context.Context, imgStr string, source image.Source, options ...Option) (*image.Image, error) {
	var provider image.Provider
	log.Debugf("image: source=%+v location=%+v", source, imgStr)

	tempDirGenerator := rootTempDirGenerator.NewGenerator()

	var cfg config
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, fmt.Errorf("unable to parse option: %w", err)
		}
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
		provider = docker.NewProviderFromDaemon(imgStr, tempDirGenerator, c)
	case image.PodmanDaemonSource:
		c, err := podman.GetClient()
		if err != nil {
			return nil, err
		}
		provider = docker.NewProviderFromDaemon(imgStr, tempDirGenerator, c)
	case image.OciDirectorySource:
		provider = oci.NewProviderFromPath(imgStr, tempDirGenerator)
	case image.OciTarballSource:
		provider = oci.NewProviderFromTarball(imgStr, tempDirGenerator)
	case image.OciRegistrySource:
		provider = oci.NewProviderFromRegistry(imgStr, tempDirGenerator, cfg.Registry)
	default:
		return nil, fmt.Errorf("unable determine image source")
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

// Cleanup deletes all directories created by stereoscope calls. Note: please use image.Image.Cleanup() over this
// function when possible.
func Cleanup() {
	if err := rootTempDirGenerator.Cleanup(); err != nil {
		log.Errorf("failed to cleanup tempdir root: %w", err)
	}
}
