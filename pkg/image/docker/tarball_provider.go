package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

var ErrMultipleManifests = fmt.Errorf("cannot process multiple docker manifests")

// TarballImageProvider is a image.Provider for a docker image (V2) for an existing tar on disk (the output from a "docker image save ..." command).
type TarballImageProvider struct {
	path        string
	extraTags   []string
	repoDigests []string
	tmpDirGen   *file.TempDirGenerator
}

// NewProviderFromTarball creates a new provider instance for the specific image already at the given path.
func NewProviderFromTarball(path string, tmpDirGen *file.TempDirGenerator, tags []string, repoDigests []string) *TarballImageProvider {
	return &TarballImageProvider{
		path:        path,
		extraTags:   tags,
		repoDigests: repoDigests,
		tmpDirGen:   tmpDirGen,
	}
}

// Provide an image object that represents the docker image tar at the configured location on disk.
func (p *TarballImageProvider) Provide(context.Context) (*image.Image, error) {
	img, err := tarball.ImageFromPath(p.path, nil)
	if err != nil {
		// raise a more controlled error for when there are multiple images within the given tar (from https://github.com/anchore/grype/issues/215)
		if err.Error() == "tarball must contain only a single image to be used with tarball.Image" {
			return nil, ErrMultipleManifests
		}
		return nil, fmt.Errorf("unable to provide image from tarball: %w", err)
	}

	// make a best-effort to generate an OCI manifest and gets tags, but ultimately this should be considered optional
	var rawOCIManifest []byte
	var rawConfig []byte
	var ociManifest *v1.Manifest
	var metadata []image.AdditionalMetadata

	theManifest, err := extractManifest(p.path)
	if err != nil {
		log.Warnf("could not extract manifest: %+v", err)
	}

	var tags = internal.NewStringSet()
	for _, t := range p.extraTags {
		tags.Add(t)
	}

	if theManifest != nil {
		// given that we have a manifest, continue processing to get the tags and OCI manifest

		for _, t := range theManifest.allTags() {
			tags.Add(t)
		}

		ociManifest, rawConfig, err = generateOCIManifest(p.path, theManifest)
		if err != nil {
			log.Warnf("failed to generate OCI manifest from docker archive: %+v", err)
		}

		// we may have the config available, use it
		if rawConfig != nil {
			metadata = append(metadata, image.WithConfig(rawConfig))
		}
	}

	if ociManifest != nil {
		rawOCIManifest, err = json.Marshal(&ociManifest)
		if err != nil {
			log.Warnf("failed to serialize OCI manifest: %+v", err)
		} else {
			metadata = append(metadata, image.WithManifest(rawOCIManifest))
		}
	}

	if len(tags) > 0 {
		metadata = append(metadata, image.WithTags(tags.ToSlice()...))
	}

	metadata = append(metadata, image.WithRepoDigests(p.repoDigests))

	contentTempDir, err := p.tmpDirGen.NewTempDir()
	if err != nil {
		return nil, err
	}

	return image.NewImage(img, contentTempDir, metadata...), nil
}
