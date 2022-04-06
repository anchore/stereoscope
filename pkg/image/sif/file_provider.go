package sif

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/sylabs/sif/v2/pkg/sif"
)

// FileImageProvider is an image.Provider for a sif image using existing sif file on disk.
type FileImageProvider struct {
	path      string
	tmpDirGen *file.TempDirGenerator
}

// NewProviderFromPath creates a new provider instance for the specific image already at the given path.
func NewProviderFromFile(path string, tmpDirGen *file.TempDirGenerator) *FileImageProvider {
	return &FileImageProvider{
		path:      path,
		tmpDirGen: tmpDirGen,
	}
}

// Provide an image object that represents the sif image as a file.
func (p *FileImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*Image, error) {
	contentTempDir, err := p.tmpDirGen.NewDirectory("sif-dir-image")
	if err != nil {
		return nil, err
	}

	f, err := sif.LoadContainerFromPath(p.path, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return nil, fmt.Errorf("failed to load image: %w", err)
	}
	defer func() { _ = f.UnloadContainer() }()

	d, err := f.GetDescriptor(sif.WithPartitionType(sif.PartPrimSys))
	if err != nil {
		return nil, fmt.Errorf("failed to get partition descriptor: %w", err)
	}

	fs, _, arch, err := d.PartitionMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get partition metadata: %w", err)
	}

	if fs != sif.FsSquash {
		return nil, errUnsupportedFSType
	}

	if err := mountSquashFS(ctx, d.Offset(), p.path, contentTempDir); err != nil {
		return nil, err
	}

	fp, err := os.OpenFile(p.path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	h := sha256.New()
	if _, err := io.Copy(h, fp); err != nil {
		return nil, fmt.Errorf("fail get sif file checksum: %v", err)
	}

	metadata := []image.AdditionalMetadata{
		image.WithOS("linux"),
		image.WithArchitecture(arch, ""),
		image.WithManifestDigest(fmt.Sprintf("sha256:%x", h.Sum(nil))),
	}
	img, err := NewImage(contentTempDir, metadata...)
	if err != nil {
		return nil, err
	}
	img.Metadata.Size = f.DataSize()

	return img, nil
}

var errUnsupportedFSType = errors.New("unrecognized filesystem type")

// mountSquashFS mounts the SquashFS filesystem from path at offset into mountPath.
func mountSquashFS(ctx context.Context, offset int64, path, mountPath string) error {
	args := []string{
		"-o", fmt.Sprintf("ro,offset=%d", offset),
		filepath.Clean(path),
		filepath.Clean(mountPath),
	}

	cmd := exec.CommandContext(ctx, "squashfuse", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to mount: %w", err)
	}

	return nil
}
