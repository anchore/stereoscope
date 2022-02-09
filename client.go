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

var tempDirGenerator = file.NewTempDirGenerator()

// GetImageFromSource returns an image from the explicitly provided source.
func GetImageFromSource(ctx context.Context, imgStr string, source image.Source, registryOptions *image.RegistryOptions) (*image.Image, error) {
	var provider image.Provider
	log.Debugf("image: source=%+v location=%+v", source, imgStr)

	switch source {
	case image.DockerTarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = docker.NewProviderFromTarball(imgStr, &tempDirGenerator, nil, nil)
	case image.DockerDaemonSource:
		c, err := dockerClient.GetClient()
		if err != nil {
			return nil, err
		}
		provider = docker.NewProviderFromDaemon(imgStr, &tempDirGenerator, c)
	case image.PodmanDaemonSource:
		c, err := podman.GetClient()
		if err != nil {
			return nil, err
		}
		provider = docker.NewProviderFromDaemon(imgStr, &tempDirGenerator, c)
	case image.OciDirectorySource:
		provider = oci.NewProviderFromPath(imgStr, &tempDirGenerator)
	case image.OciTarballSource:
		provider = oci.NewProviderFromTarball(imgStr, &tempDirGenerator)
	case image.OciRegistrySource:
		provider = oci.NewProviderFromRegistry(imgStr, &tempDirGenerator, registryOptions)
	default:
		return nil, fmt.Errorf("unable determine image source")
	}

	img, err := provider.Provide(ctx)
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
func GetImage(ctx context.Context, userStr string, registryOptions *image.RegistryOptions) (*image.Image, error) {
	source, imgStr, err := image.DetectSource(userStr)
	if err != nil {
		return nil, err
	}
	return GetImageFromSource(ctx, imgStr, source, registryOptions)
}

func SetLogger(logger logger.Logger) {
	log.Log = logger
}

func SetBus(b *partybus.Bus) {
	bus.SetPublisher(b)
}

func Cleanup() {
	if err := tempDirGenerator.Cleanup(); err != nil {
		log.Errorf("failed to cleanup: %w", err)
	}
}
