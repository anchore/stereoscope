package image

import "context"

// Provider is an abstraction for any object that provides image objects (e.g. the docker daemon API, a tar file of
// an OCI image, podman varlink API, etc.).
type Provider interface {
	Provide(context.Context, ...AdditionalMetadata) (*Image, error)
}

// IndexProvider is an abstraction for any object that provides image indexes.
type IndexProvider interface {
	ProvideIndex(context.Context, ...AdditionalMetadata) (*Index, error)
}
