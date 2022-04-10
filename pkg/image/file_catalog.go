package image

import (
	"fmt"
	"io"
	"sync"

	"github.com/anchore/stereoscope/pkg/file"
)

var ErrFileNotFound = fmt.Errorf("could not find file")

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	sync.RWMutex
	catalog    map[file.ID]FileCatalogEntry
	byMIMEType map[string][]file.ID
}

// FileCatalogEntry represents all stored metadata for a single file reference.
type FileCatalogEntry struct {
	File     file.Reference
	Metadata file.Metadata
	Layer    *Layer
	Contents file.Opener
}

// NewFileCatalog returns an empty FileCatalog.
func NewFileCatalog() FileCatalog {
	return FileCatalog{
		catalog:    make(map[file.ID]FileCatalogEntry),
		byMIMEType: make(map[string][]file.ID),
	}
}

// Add creates a new FileCatalogEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, l *Layer, opener file.Opener) {
	c.Lock()
	defer c.Unlock()
	if m.MIMEType != "" {
		// an empty MIME type means that we didn't have the contents of the file to determine the MIME type. If we have
		// the contents and the MIME type could not be determined then the default value is application/octet-stream.
		c.byMIMEType[m.MIMEType] = append(c.byMIMEType[m.MIMEType], f.ID())
	}
	c.catalog[f.ID()] = FileCatalogEntry{
		File:     f,
		Metadata: m,
		Layer:    l,
		Contents: opener,
	}
}

// Exists indicates if the given file reference exists in the catalog.
func (c *FileCatalog) Exists(f file.Reference) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.catalog[f.ID()]
	return ok
}

// Get fetches a FileCatalogEntry for the given file reference, or returns an error if the file reference has not
// been added to the catalog.
func (c *FileCatalog) Get(f file.Reference) (FileCatalogEntry, error) {
	c.RLock()
	defer c.RUnlock()
	value, ok := c.catalog[f.ID()]
	if !ok {
		return FileCatalogEntry{}, ErrFileNotFound
	}
	return value, nil
}

func (c *FileCatalog) GetByMIMEType(mType string) ([]FileCatalogEntry, error) {
	c.RLock()
	defer c.RUnlock()
	fileIDs, ok := c.byMIMEType[mType]
	if !ok {
		return nil, nil
	}
	var entries []FileCatalogEntry
	for _, id := range fileIDs {
		entry, ok := c.catalog[id]
		if !ok {
			return nil, fmt.Errorf("could not find file: %+v", id)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// FetchContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *FileCatalog) FileContents(f file.Reference) (io.ReadCloser, error) {
	c.RLock()
	defer c.RUnlock()
	catalogEntry, ok := c.catalog[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.RealPath)
	}

	if catalogEntry.Contents == nil {
		return nil, fmt.Errorf("no contents available for file: %+v", f.RealPath)
	}

	return catalogEntry.Contents(), nil
}
