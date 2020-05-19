package docker

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func extractManifest(tarPath string) (tarball.Manifest, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := f.Close()
		if err != nil {
			// TODO: replace
			panic(err)
		}
	}()

	var manifest tarball.Manifest
	manifestReader, err := file.ReaderFromTar(f, "manifest.json")
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(manifestReader).Decode(&manifest); err != nil {
		return nil, err
	}

	if manifest == nil {
		return nil, fmt.Errorf("no valid manifest.json in tarball")
	}

	return manifest, nil
}

func extractTags(tarPath string) ([]name.Tag, error) {
	manifest, err := extractManifest(tarPath)
	if err != nil {
		return nil, err
	}

	if len(manifest) != 1 {
		return nil, fmt.Errorf("unexpected manifest length (%d)", len(manifest))
	}

	tags := make([]name.Tag, 0)
	for _, tag := range manifest[0].RepoTags {
		tagObj, err := name.NewTag(tag)
		if err != nil {
			return nil, fmt.Errorf("unable to parse tag: '%s'", tag)
		}
		tags = append(tags, tagObj)
	}
	return tags, nil
}
