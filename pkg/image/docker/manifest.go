package docker

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type manifest struct {
	raw    []byte
	parsed tarball.Manifest
}

// newManifest creates a new manifest struct from the given Docker archive manifest bytes
func newManifest(raw []byte) (manifest, error) {
	var parsed tarball.Manifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return manifest{}, fmt.Errorf("unable to parse manifest.json: %w", err)
	}

	if len(parsed) == 0 {
		return manifest{}, fmt.Errorf("no valid manifest.json found")
	}

	return manifest{
		raw:    raw,
		parsed: parsed,
	}, nil
}

// tags returns the image tags referenced within the images manifest file (within the given docker image tar).
func (m manifest) tags() (tags []string) {
	for _, entry := range m.parsed {
		tags = append(tags, entry.RepoTags...)
	}
	return tags
}
