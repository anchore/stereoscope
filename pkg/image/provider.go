package image

import "context"

// Provider is an abstraction for any object that provides image objects (e.g. the docker daemon API, a tar file of
// an OCI image, podman varlink API, etc.).
// Temp files are bound to the lifecycle of the context passed in for Provide.
type Provider interface {
	Provide(ctx context.Context) (*Image, error)
}
