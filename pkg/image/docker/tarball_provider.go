package docker

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// TarballImageProvider is a image.Provider for a docker image (V2) for an existing tar on disk (the output from a "docker image save ..." command).
type TarballImageProvider struct {
	path string
}

// NewProviderFromTarball creates a new provider instance for the specific image already at the given path.
func NewProviderFromTarball(path string) *TarballImageProvider {
	return &TarballImageProvider{
		path: path,
	}
}

// Provide an image object that represents the docker image tar at the configured location on disk.
func (p *TarballImageProvider) Provide() (*image.Image, error) {
	img, err := tarball.ImageFromPath(p.path, nil)
	if err != nil {
		return nil, err
	}

	theManifest, err := p.extractManifest()
	if err != nil {
		return nil, err
	}

	return image.NewImage(img, image.WithTags(theManifest.tags()...), image.WithManifest(theManifest.raw))
}

// extractManifest is helper function for extracting and parsing a docker image manifest (V2) from a docker image tar.
func (p *TarballImageProvider) extractManifest() (manifest, error) {
	f, err := os.Open(p.path)
	if err != nil {
		return manifest{}, err
	}

	defer func() {
		err := f.Close()
		if err != nil {
			log.Errorf("unable to close tar file (%s): %w", f.Name(), err)
		}
	}()

	manifestReader, err := file.ReaderFromTar(f, "manifest.json")
	if err != nil {
		return manifest{}, err
	}

	contents, err := ioutil.ReadAll(manifestReader)
	if err != nil {
		return manifest{}, fmt.Errorf("unable to read manifest.json: %w", err)
	}
	return newManifest(contents)
}
