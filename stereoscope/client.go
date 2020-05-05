package stereoscope

import (
	"fmt"
	"github.com/anchore/stereoscope/stereoscope/image"
	"github.com/anchore/stereoscope/stereoscope/image/tarball"
)

type Client struct {
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) GetImage(imgStr string) (*image.Image, error) {
	source, location := image.ParseImageSpec(imgStr)

	var provider image.Provider

	switch source {
	case image.TarballSource:
		provider = tarball.NewTarballProvider(location)
	default:
		return nil, fmt.Errorf("unable determine image source")
	}

	return provider.Provide()
}
