package containers_storage

import (
	"context"
	"fmt"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"path"

	"github.com/anchore/stereoscope/pkg/image"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/transports/alltransports"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)


// ContainersStorageProvider is a image.Provider capable of fetching and representing a OCI image from the
// containers-storage location (used by podman and others).
type ContainersStorageProvider struct {
	ImageStr string
	cacheDir string
}

// NewProvider creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProvider(imgStr, cacheDir string) *ContainersStorageProvider {
	return &ContainersStorageProvider{
		ImageStr: imgStr,
		cacheDir: cacheDir,
	}
}

// Provide an image object that represents the cached OCI directory fetched from container-storage.
func (p *ContainersStorageProvider) Provide() (*image.Image, error) {
	csSrcImageRef, err := alltransports.ParseImageName("containers-storage:"+p.ImageStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the image reference: %w", err)
	}

	imagePath := path.Join(p.cacheDir, "oci-image-export")
	destRef, err := alltransports.ParseImageName("oci:"+imagePath)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the image reference: %w", err)
	}

	_, err = copy.Image(context.Background(), nil, destRef, csSrcImageRef, &copy.Options{
		SourceCtx:             nil,
		DestinationCtx:        nil,
		ForceManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		ImageListSelection:    copy.CopySystemImage,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to provide containers-storage image: %w", err)
	}

	return oci.NewProviderFromPath(imagePath).Provide()
}