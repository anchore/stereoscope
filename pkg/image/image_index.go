package image

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/hashicorp/go-multierror"
)

// Index represents a container image index.
type Index struct {
	// index is the raw index manifest and content provider from the GCR lib
	index v1.ImageIndex
	// images is a list of images associated with an index.
	images []*Image
}

// NewIndex provides a new image index object.
func NewIndex(index v1.ImageIndex, images []*Image) *Index {
	return &Index{
		index:  index,
		images: images,
	}
}

// Images returns a list of images associated with an index.
func (i *Index) Images() []*Image {
	return i.images
}

// Cleanup removes all temporary files created from parsing the index and associated images.
// Future calls to image will not function correctly after this call.
func (i *Index) Cleanup() error {
	if i == nil {
		return nil
	}
	var errs error
	for _, img := range i.images {
		if err := img.Cleanup(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}
