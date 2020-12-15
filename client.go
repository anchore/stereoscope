package stereoscope

import (
	"fmt"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"github.com/anchore/stereoscope/pkg/logger"
	"github.com/wagoodman/go-partybus"
)

var tempDirGenerator = file.NewTempDirGenerator()

// GetImage parses the user provided image string and provides an image object
func GetImage(userStr string) (*image.Image, error) {
	var provider image.Provider
	source, imgStr, err := image.DetectSource(userStr)
	if err != nil {
		return nil, err
	}

	log.Debugf("image: source=%+v location=%+v", source, imgStr)

	switch source {
	case image.DockerTarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = docker.NewProviderFromTarball(imgStr, &tempDirGenerator)
	case image.DockerDaemonSource:
		provider = docker.NewProviderFromDaemon(imgStr, &tempDirGenerator)
	case image.OciDirectorySource:
		provider = oci.NewProviderFromPath(imgStr, &tempDirGenerator)
	case image.OciTarballSource:
		provider = oci.NewProviderFromTarball(imgStr, &tempDirGenerator)
	default:
		return nil, fmt.Errorf("unable determine image source")
	}

	img, err := provider.Provide()
	if err != nil {
		return nil, err
	}

	err = img.Read()
	if err != nil {
		return nil, fmt.Errorf("could not read image: %+v", err)
	}

	return img, nil
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
