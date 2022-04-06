package sif

import (
	"context"

	"github.com/anchore/stereoscope/pkg/image"
)

// Provider is an abstraction for any object that provides sif image objects.
type Provider interface {
	Provide(context.Context, ...image.AdditionalMetadata) (*Image, error)
}
