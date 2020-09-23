package stereoscope

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anchore/stereoscope/pkg/image/oci"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/logger"
	"github.com/hashicorp/go-multierror"
	"github.com/wagoodman/go-partybus"
)

const (
	NoActionOption Option = iota
	ReadImageOption
)

type Option uint

var trackerInstance *tracker

func init() {
	trackerInstance = &tracker{
		tempDir: make([]string, 0),
	}
}

type tracker struct {
	tempDir []string
}

// newTempDir creates an empty dir in the platform temp dir
func (t *tracker) newTempDir() string {
	dir, err := ioutil.TempDir("", "stereoscope-cache")
	if err != nil {
		log.Errorf("could not create temp dir: %w", err)
		panic(err)
	}

	t.tempDir = append(t.tempDir, dir)
	return dir
}

func (t *tracker) cleanup() error {
	var allErrors error
	for _, dir := range t.tempDir {
		err := os.RemoveAll(dir)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}

// GetImage parses the user provided image string and provides an image object
func GetImage(userStr string, options ...Option) (*image.Image, error) {
	var provider image.Provider
	source, imgStr, err := image.DetectSource(userStr)
	if err != nil {
		return nil, err
	}

	var processingOption = NoActionOption
	if len(options) == 0 {
		processingOption = ReadImageOption
	} else {
		for _, o := range options {
			if o > processingOption {
				processingOption = o
			}
		}
	}

	log.Debugf("image: source=%+v location=%+v processingOption=%+v", source, imgStr, processingOption)

	switch source {
	case image.DockerTarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = docker.NewProviderFromTarball(imgStr)
	case image.DockerDaemonSource:
		cacheDir := trackerInstance.newTempDir()
		provider = docker.NewProviderFromDaemon(imgStr, cacheDir)
	case image.OciDirectorySource:
		provider = oci.NewProviderFromPath(imgStr)
	case image.OciTarballSource:
		cacheDir := trackerInstance.newTempDir()
		provider = oci.NewProviderFromTarball(imgStr, cacheDir)
	default:
		return nil, fmt.Errorf("unable determine image source")
	}

	img, err := provider.Provide()
	if err != nil {
		return nil, err
	}

	if processingOption >= ReadImageOption {
		err = img.Read()
		if err != nil {
			return nil, fmt.Errorf("could not read image: %+v", err)
		}
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
	err := trackerInstance.cleanup()
	if err != nil {
		log.Errorf("failed to cleanup: %w", err)
	}
}
