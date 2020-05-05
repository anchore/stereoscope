package tarball

import (
	"github.com/anchore/stereoscope/stereoscope/image"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type Provider struct {
	path string
}

func NewTarballProvider(path string) *Provider {
	return &Provider{
		path: path,
	}
}

func (p *Provider) Provide() (*image.Image, error) {
	img, err := tarball.ImageFromPath(p.path, nil)
	if err != nil {
		return nil, err
	}
	return image.NewImage(img), nil
}
