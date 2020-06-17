package image

import (
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
)

// Metadata represents container image metadata.
type Metadata struct {
	// Digest is the sha256 of this image manifest json
	Digest string
	// Size in bytes of all the image layer content sizes
	Size      int64
	Config    v1.ConfigFile
	MediaType v1Types.MediaType
	Tags      []name.Tag
}

// readImageMetadata extracts the most pertinent information from the underlying image tar.
func readImageMetadata(img v1.Image) (Metadata, error) {
	digest, err := img.ConfigName()
	if err != nil {
		return Metadata{}, err
	}

	config, err := img.ConfigFile()
	if err != nil {
		return Metadata{}, err
	}

	mediaType, err := img.MediaType()
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		Digest:    digest.String(),
		Config:    *config,
		MediaType: mediaType,
	}, nil
}
