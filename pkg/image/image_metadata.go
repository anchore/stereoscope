package image

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Metadata struct {
	// sha256 of this image manifest json
	Digest string
	// Size in bytes of all the image layer content sizes
	Size   int64
	Config v1.ConfigFile
}

func readImageMetadata(img v1.Image) (Metadata, error) {
	digest, err := img.ConfigName()
	if err != nil {
		return Metadata{}, err
	}

	config, err := img.ConfigFile()
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		Digest: digest.String(),
		Config: *config,
	}, nil
}
