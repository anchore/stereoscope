package stereoscope

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/image/tarball"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/go-multierror"
)

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
		// TODO: replace
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
func GetImage(userStr string) (*image.Image, error) {
	var provider image.Provider
	source, imgStr := image.ParseImageSpec(userStr)

	switch source {
	case image.TarballSource:
		// note: the imgStr is the path on disk to the tar file
		provider = tarball.NewProvider(imgStr)
	case image.DockerSource:
		imgRef, err := name.ParseReference(imgStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse image identifier: %w", err)
		}
		cacheDir := trackerInstance.newTempDir()
		provider = docker.NewProvider(imgRef, cacheDir)
	default:
		return nil, fmt.Errorf("unable determine image source")
	}

	return provider.Provide()
}

func Cleanup() {
	err := trackerInstance.cleanup()
	if err != nil {
		// TODO: replace
		panic(err)
	}
}
