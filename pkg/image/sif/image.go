package sif

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anchore/stereoscope/pkg/image"
)

// Image represents sif image containing metadata info and path to mounted directory
type Image struct {
	Metadata    image.Metadata
	MountedPath string
}

// NewImage provides a new image object.
func NewImage(mountedPath string, additionalMetadata ...image.AdditionalMetadata) (*Image, error) {
	img := &image.Image{
		Metadata: image.Metadata{},
	}
	for _, opt := range additionalMetadata {
		if err := opt(img); err != nil {
			return nil, err
		}
	}

	imgObj := &Image{
		Metadata:    img.Metadata,
		MountedPath: mountedPath,
	}
	return imgObj, nil
}

// Cleanup unmounts sif image.
func (i *Image) Cleanup(ctx context.Context) error {
	if i == nil {
		return nil
	}
	if err := unmount(ctx, i.MountedPath); err != nil {
		return err
	}
	return nil
}

func unmount(ctx context.Context, mountPath string) error {
	cmd := exec.CommandContext(ctx, "umount", filepath.Clean(mountPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unmount: %w", err)
	}

	return nil
}
