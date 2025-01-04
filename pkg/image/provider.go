package image

import (
	"context"
	"fmt"
)

// ErrFetchingImage is meant to be used when a provider has positively resolved the image but, while fetching the
// image, an error occurred. The goal is to differentiate between a provider that cannot resolve an image (thus
// if the caller has a set of providers, it can try another provider) and a provider that can resolve an image but
// there is an unresolvable problem (e.g. network error, mismatched architecture, etc... thus the caller should
// not try any further providers).
type ErrFetchingImage struct {
	Reason string
}

func (e *ErrFetchingImage) Error() string {
	return fmt.Sprintf("error fetching image: %s", e.Reason)
}

// Provider is an abstraction for any object that provides image objects (e.g. the docker daemon API, a tar file of
// an OCI image, podman varlink API, etc.).
type Provider interface {
	Name() string
	Provide(context.Context) (*Image, error)
}
