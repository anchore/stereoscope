package oci

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// DirectoryImageProvider is an image.Provider for an OCI image (V1) for an existing tar on disk (from a buildah push <img> oci:<img> command).
type DirectoryImageProvider struct {
	path      string
	tmpDirGen *file.TempDirGenerator
	platform  *image.Platform
}

// NewProviderFromPath creates a new provider instance for the specific image already at the given path.
func NewProviderFromPath(path string, tmpDirGen *file.TempDirGenerator, platform *image.Platform) *DirectoryImageProvider {
	return &DirectoryImageProvider{
		path:      path,
		tmpDirGen: tmpDirGen,
		platform:  platform,
	}
}

// Provide an image object that represents the OCI image as a directory.
func (p *DirectoryImageProvider) Provide(_ context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	pathObj, err := layout.FromPath(p.path)
	if err != nil {
		return nil, fmt.Errorf("unable to read image from OCI directory path %q: %w", p.path, err)
	}

	index, err := layout.ImageIndexFromPath(p.path)
	if err != nil {
		return nil, fmt.Errorf("unable to parse OCI directory index: %w", err)
	}

	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("unable to parse OCI directory indexManifest: %w", err)
	}

	manifestLen := len(indexManifest.Manifests)
	if manifestLen < 1 {
		return nil, errors.New("expected at least one OCI manifest (found 0)")
	}

	var manifest v1.Descriptor
	if manifestLen > 1 {
		if p.platform == nil {
			// if all the manifests have the same digest, then we can treat this as a single image
			if !checkManifestDigestsEqual(indexManifest.Manifests) {
				// TODO: default to the current OS?
				return nil, errors.New("when a OCI manifest contains multiple references, a platform selector is required")
			}
			manifest = indexManifest.Manifests[0]
		} else {
			var found bool
			for _, m := range indexManifest.Manifests {
				if m.Platform == nil {
					continue
				}

				// Check if the manifest's platform matches our selector.
				if m.Platform.OS != p.platform.OS {
					continue
				}

				// Check if the manifest's architecture matches our selector.
				if m.Platform.Architecture != p.platform.Architecture {
					continue
				}

				// Check if the manifest's variant matches our selector.
				if m.Platform.Variant != p.platform.Variant {
					continue
				}

				// TODO: there is the possibility that multiple manifests may match.
				// Do we continue iterating all of them and check if multiple matches
				// exist then throw an error?
				manifest = m
				found = true
				break
			}

			if !found {
				return nil, fmt.Errorf("unable to find a OCI manifest matching the given platform (platform: %s)", p.platform)
			}
		}
	} else {
		// Only one manifest exists, so use it.
		manifest = indexManifest.Manifests[0]
	}

	img, err := pathObj.Image(manifest.Digest)
	if err != nil {
		return nil, fmt.Errorf("unable to parse OCI directory as an image: %w", err)
	}

	metadata := []image.AdditionalMetadata{
		image.WithManifestDigest(manifest.Digest.String()),
	}

	// make a best-effort attempt at getting the raw indexManifest
	rawManifest, err := img.RawManifest()
	if err == nil {
		metadata = append(metadata, image.WithManifest(rawManifest))
	}

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)

	contentTempDir, err := p.tmpDirGen.NewDirectory("oci-dir-image")
	if err != nil {
		return nil, err
	}

	return image.New(img, p.tmpDirGen, contentTempDir, metadata...), nil
}

// ProvideIndex provides an image index that represents the OCI image as a directory.
func (p *DirectoryImageProvider) ProvideIndex(_ context.Context, userMetadata ...image.AdditionalMetadata) (*image.Index, error) {
	pathObj, err := layout.FromPath(p.path)
	if err != nil {
		return nil, fmt.Errorf("unable to read image from OCI directory path %q: %w", p.path, err)
	}

	index, err := layout.ImageIndexFromPath(p.path)
	if err != nil {
		return nil, fmt.Errorf("unable to parse OCI directory index: %w", err)
	}

	indexManifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("unable to parse OCI directory indexManifest: %w", err)
	}

	if len(indexManifest.Manifests) < 1 {
		return nil, fmt.Errorf("expected at least one OCI directory manifests (found %d)", len(indexManifest.Manifests))
	}

	images := make([]*image.Image, len(indexManifest.Manifests))
	for i, manifest := range indexManifest.Manifests {
		img, err := pathObj.Image(manifest.Digest)
		if err != nil {
			return nil, fmt.Errorf("unable to parse OCI directory as an image: %w", err)
		}

		metadata := []image.AdditionalMetadata{
			image.WithManifestDigest(manifest.Digest.String()),
		}
		if manifest.Platform != nil {
			if manifest.Platform.Architecture != "" {
				metadata = append(metadata, image.WithArchitecture(manifest.Platform.Architecture, manifest.Platform.Variant))
			}
			if manifest.Platform.OS != "" {
				metadata = append(metadata, image.WithOS(manifest.Platform.OS))
			}
		}

		// make a best-effort attempt at getting the raw indexManifest
		rawManifest, err := img.RawManifest()
		if err == nil {
			metadata = append(metadata, image.WithManifest(rawManifest))
		}

		// apply user-supplied metadata last to override any default behavior
		metadata = append(metadata, userMetadata...)

		contentTempDir, err := p.tmpDirGen.NewDirectory("oci-dir-image-" + strconv.Itoa(i))
		if err != nil {
			return nil, err
		}

		images[i] = image.New(img, p.tmpDirGen, contentTempDir, metadata...)
	}

	return image.NewIndex(index, images), nil
}

func checkManifestDigestsEqual(manifests []v1.Descriptor) bool {
	if len(manifests) < 1 {
		return false
	}
	for _, m := range manifests {
		if m.Digest != manifests[0].Digest {
			return false
		}
	}
	return true
}
