package tarball

import (
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type Provider struct {
	path string
}

func NewProvider(path string) *Provider {
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
