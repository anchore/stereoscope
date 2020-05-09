package stereoscope

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/tarball"
)

func GetImage(imgStr string) (*image.Image, error) {
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
