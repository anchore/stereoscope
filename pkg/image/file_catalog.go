package image

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/anchore/stereoscope/pkg/file"
)

var ErrFileNotFound = fmt.Errorf("could not find file")

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	catalog map[file.ID]*FileCatalogEntry
}

// FileCatalogEntry represents all stored metadata for a single file reference.
type FileCatalogEntry struct {
	File     file.Reference
	Metadata file.Metadata
	Layer    *Layer
}

// NewFileCatalog returns an empty FileCatalog.
func NewFileCatalog() FileCatalog {
	return FileCatalog{
		catalog: make(map[file.ID]*FileCatalogEntry),
	}
}

// Add creates a new FileCatalogEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, l *Layer) {
	c.catalog[f.ID()] = &FileCatalogEntry{
		File:     f,
		Metadata: m,
		Layer:    l,
	}
}

// Exists indicates if the given file reference exists in the catalog.
func (c *FileCatalog) Exists(f file.Reference) bool {
	_, ok := c.catalog[f.ID()]
	return ok
}

// Get fetches a FileCatalogEntry for the given file reference, or returns an error if the file reference has not
// been added to the catalog.
func (c *FileCatalog) Get(f file.Reference) (FileCatalogEntry, error) {
	value, ok := c.catalog[f.ID()]
	if !ok {
		return FileCatalogEntry{}, ErrFileNotFound
	}
	return *value, nil
}

// FetchContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *FileCatalog) FileContents(f file.Reference) (io.ReadCloser, error) {
	catalogEntry, ok := c.catalog[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.RealPath)
	}

	// get header + content reader from the underlying tar
	tarEntries, err := catalogEntry.Layer.indexedContent.EntriesByName(catalogEntry.Metadata.TarHeaderName)
	if err != nil {
		return nil, err
	}

	for _, tarEntry := range tarEntries {
		if tarEntry.Sequence == catalogEntry.Metadata.TarSequence {
			return ioutil.NopCloser(tarEntry.Reader), nil
		}
	}

	return nil, nil
}
