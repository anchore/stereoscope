package containerd

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"github.com/containerd/containerd"
)

// DaemonImageProvider is a image.Provider capable of fetching and representing a docker image from the containerd daemon API.
type DaemonImageProvider struct {
	imageStr  string
	tmpDirGen *file.TempDirGenerator
	client    containerd.Client
	platform  *image.Platform
}

// NewProviderFromDaemon creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromDaemon(imgStr string, tmpDirGen *file.TempDirGenerator, c containerd.Client, platform *image.Platform) *DaemonImageProvider {
	return &DaemonImageProvider{
		imageStr:  imgStr,
		tmpDirGen: tmpDirGen,
		client:    c,
		platform:  platform,
	}
}

func (p *DaemonImageProvider) pullImageIfMissing(ctx context.Context) (*containerd.Image, error) {
	// check if the image exists locally
	img, err := p.client.GetImage(ctx, p.imageStr)
	if err != nil {
		//TODO: include platform in pulling the image
		pulledImaged, err := p.client.Pull(ctx, p.imageStr)
		if err != nil {
			return nil, err
		}
		return &pulledImaged, nil
	} else {
		// looks like the image exists, but the platform doesn't match what the user specified, so we need to
		// pull the image again with the correct platform specifier, which will override the local tag.
		if err := p.validPlatform(img); err != nil {
			//TODO: include platform in pulling the image
			pulledImaged, err := p.client.Pull(ctx, p.imageStr)
			if err != nil {
				return nil, err
			}
			return &pulledImaged, nil
		}
	}
	return &img, nil
}

func (p *DaemonImageProvider) validPlatform(img containerd.Image) error {
	if p.platform == nil {
		// the user did not specify a platform
		return nil
	}

	platform := img.Target().Platform
	switch {
	case platform.OS != p.platform.OS:
		return fmt.Errorf("image has unexpected OS %q, which differs from the user specified PS %q", i.Os, p.platform.OS)
	case platform.Architecture != p.platform.Architecture:
		return fmt.Errorf("image has unexpected architecture %q, which differs from the user specified architecture %q", i.Architecture, p.platform.Architecture)
	case platform.Variant != p.platform.Variant:
		return fmt.Errorf("image has unexpected architecture %q, which differs from the user specified architecture %q", i.Architecture, p.platform.Architecture)
	}

	return nil
}

// save the image from the containerd daemon to a tar file
func (p *DaemonImageProvider) saveImage(ctx context.Context, img containerd.Image) (string, error) {
	imageTempDir, err := p.tmpDirGen.NewDirectory("containerd-daemon-image")
	if err != nil {
		return "", err
	}

	// create a file within the temp dir
	tempTarFile, err := os.Create(path.Join(imageTempDir, "image.tar"))
	if err != nil {
		return "", fmt.Errorf("unable to create temp file for image: %w", err)
	}
	defer func() {
		err := tempTarFile.Close()
		if err != nil {
			log.Errorf("unable to close temp file (%s): %w", tempTarFile.Name(), err)
		}
	}()

	err = p.client.Export(ctx, tempTarFile)
	if err != nil {
		return "", fmt.Errorf("unable to save image tar: %w", err)
	}

	return tempTarFile.Name(), nil
}

func withMetadata(img containerd.Image, userMetadata []image.AdditionalMetadata) (metadata []image.AdditionalMetadata) {
	tags := []string{}
	for k, v := range img.Labels() {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}
	metadata = append(metadata,
		image.WithTags(tags...),
		image.WithArchitecture(img.Target().Platform.Architecture, img.Target().Platform.Variant),
		image.WithOS(img.Target().Platform.OS),
	)

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)
	return metadata
}

// Provide an image object that represents the cached docker image tar fetched from a containerd daemon.
func (p *DaemonImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	image, err := p.pullImageIfMissing(ctx)
	if err != nil {
		return nil, err
	}

	if err := p.validPlatform(*image); err != nil {
		return nil, err
	}

	tarFileName, err := p.saveImage(ctx, *image)
	if err != nil {
		return nil, err
	}

	// use the existing tarball provider to process what was pulled from the containerd daemon
	return oci.NewProviderFromTarball(tarFileName, p.tmpDirGen).Provide(ctx, withMetadata(*image, userMetadata)...)
}
