package stereoscope

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/go-multierror"
)

const (
	NoActionOption Option = iota
	ReadImageOption
	SquashImageOption
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

// GetImage parses the user provided image string and provides a image object
func GetImage(userStr string, options ...Option) (*image.Image, error) {
	var provider image.Provider
	source, imgStr := image.ParseImageSpec(userStr)

	var processingOption = NoActionOption
	if len(options) == 0 {
		processingOption = SquashImageOption
	} else {
		for _, o := range options {
			if o > processingOption {
				processingOption = o
			}
		}
	}

	switch source {
	case image.DockerTarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = docker.NewProviderFromTarball(imgStr)
	case image.DockerDaemonSource:
		imgRef, err := name.ParseReference(imgStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse image identifier: %w", err)
		}
		cacheDir := trackerInstance.newTempDir()
		provider = docker.NewProviderFromDaemon(imgRef, cacheDir)
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

	if processingOption >= SquashImageOption {
		err = img.Squash()
		if err != nil {
			return nil, fmt.Errorf("could not squash image: %+v", err)
		}
	}

	return img, nil
}

func Cleanup() {
	err := trackerInstance.cleanup()
	if err != nil {
		// TODO: replace
		log.Errorf("failed to cleanup: %w", err)
	}
}
