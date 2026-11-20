package oci

import (
	"bytes"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/anchore/stereoscope/internal/log"
)

// layoutImageIndex returns the image index that the selected image belongs to within an OCI layout, if there
// is exactly one answer.
//
// The index.json at the root of a layout is not that index. It is the layout's table of contents: it is not
// content addressed, a registry never serves it, and it may list unrelated images side by side, so its digest
// identifies nothing. What a registry would have served for a multi-platform image is an index stored as a
// blob and referenced from index.json (as written by `skopeo copy --all` or `docker buildx build -o type=oci`).
// That nested index is reported, taken from the entry in index.json so that it matches what the same reference
// would resolve to in a registry, rather than from an index nested further down.
//
// Nothing is reported when the image is listed in index.json directly, or is reachable through more than one
// nested index, since the layout does not say which of them the image was meant to be selected from.
func layoutImageIndex(layoutIndex v1.ImageIndex, selected v1.Hash) ([]byte, v1Types.MediaType, bool) {
	indexManifest, err := layoutIndex.IndexManifest()
	if err != nil {
		return nil, "", false
	}

	var found *v1.Descriptor
	for i := range indexManifest.Manifests {
		desc := indexManifest.Manifests[i]
		switch {
		case desc.MediaType.IsImage():
			if desc.Digest == selected {
				return nil, "", false
			}
		case desc.MediaType.IsIndex():
			if found != nil && found.Digest == desc.Digest {
				// the same index under another name (e.g. a second tag)
				continue
			}
			nested, err := layoutIndex.ImageIndex(desc.Digest)
			if err != nil {
				return nil, "", false
			}
			contains, err := indexContainsImage(nested, selected)
			if err != nil {
				return nil, "", false
			}
			if !contains {
				continue
			}
			if found != nil {
				return nil, "", false
			}
			found = &desc
		}
	}
	if found == nil {
		return nil, "", false
	}

	nested, err := layoutIndex.ImageIndex(found.Digest)
	if err != nil {
		return nil, "", false
	}
	raw, err := nested.RawManifest()
	if err != nil {
		return nil, "", false
	}

	// a layout blob is not verified when it is read, and the digest reported for the index is taken from these
	// bytes. Do not report an index that is not the one index.json names.
	digest, _, err := v1.SHA256(bytes.NewReader(raw))
	if err != nil || digest != found.Digest {
		log.WithFields("digest", found.Digest.String()).Debug("nested index in OCI directory does not match its digest")
		return nil, "", false
	}

	return raw, found.MediaType, true
}

// indexContainsImage reports whether the image is a child of the index, at any depth.
func indexContainsImage(index v1.ImageIndex, image v1.Hash) (bool, error) {
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return false, err
	}
	for _, desc := range indexManifest.Manifests {
		switch {
		case desc.MediaType.IsImage():
			if desc.Digest == image {
				return true, nil
			}
		case desc.MediaType.IsIndex():
			nested, err := index.ImageIndex(desc.Digest)
			if err != nil {
				return false, err
			}
			contains, err := indexContainsImage(nested, image)
			if err != nil || contains {
				return contains, err
			}
		}
	}
	return false, nil
}
