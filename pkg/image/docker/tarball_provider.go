package docker

import (
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type TarballImageProvider struct {
	path string
}

func NewProviderFromTarball(path string) *TarballImageProvider {
	return &TarballImageProvider{
		path: path,
	}
}

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
