package docker

import (
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

	tags, err := extractTags(p.path)
	if err != nil {
		return nil, err
	}

	return image.NewImageWithTags(img, tags), nil
}
