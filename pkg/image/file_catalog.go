package image

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/anchore/stereoscope/pkg/file"
)

var ErrFileNotFound = fmt.Errorf("could not find file")

var cacheFileSizeThreshold int64 = 5 * file.MB

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	catalog          map[file.ID]*FileCatalogEntry
	contentsCacheDir string
	// contentsCachePath is a mapping of the paths for each file ID already previously requested by a caller. This is
	// to prevent duplicated or unnecessary tar content requests (which can be expensive)
	contentsCachePath map[file.ID]string
}

// FileCatalogEntry represents all stored metadata for a single file reference.
type FileCatalogEntry struct {
	File     file.Reference
	Metadata file.Metadata
	Layer    *Layer
}

// NewFileCatalog returns an empty FileCatalog.
func NewFileCatalog(contentsCacheDir string) FileCatalog {
	return FileCatalog{
		catalog:           make(map[file.ID]*FileCatalogEntry),
		contentsCachePath: make(map[file.ID]string),
		contentsCacheDir:  contentsCacheDir,
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

// handleContentResponse returns a io.ReadCloser for the given file reference that does not take up precious file
// descriptors until the first Read() call on the io.ReadCloser. This function is additionally responsible for handling
// caching of previous results into a cache directory in case future calls are interested in the results as well as
// provide a non-memory-intensive Reader for the file reference by storing to disk.
func (c *FileCatalog) handleContentResponse(ref file.Reference, tarReader io.Reader) (io.ReadCloser, error) {
	entry, err := c.Get(ref)
	if err != nil {
		return nil, err
	}

	if entry.Metadata.Size <= cacheFileSizeThreshold {
		// this is a small file, read the contents into memory and return a reader
		theBytes, err := ioutil.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("unable to handle in-memory content response: %w", err)
		}
		return ioutil.NopCloser(bytes.NewReader(theBytes)), nil
	}

	// check to see if this is already in the cache, if so, return a reader to the cache reference instead
	if p, ok := c.contentsCachePath[ref.ID()]; ok {
		return file.NewDeferredReadCloser(p), nil
	}

	// cache the result to a directory and return a DeferredReadCloser to not allocate file handles unless they are
	// actively being used.
	tempFile, err := ioutil.TempFile(c.contentsCacheDir, ref.RealPath.Basename()+"-")
	if err != nil {
		return nil, fmt.Errorf("unable to create content response cache: %w", err)
	}
	defer tempFile.Close()

	// stream the contents from the reader directly into the temp file
	if _, err := io.Copy(tempFile, tarReader); err != nil {
		return nil, fmt.Errorf("unable to copy content response to cache: %w", err)
	}

	// keep track of the reference in the file catalog cache
	if _, ok := c.contentsCachePath[ref.ID()]; ok {
		// the ref should not have already existed! this implies a potential race condition
		return nil, fmt.Errorf("found duplicate file catalog cache entry: %+v", ref)
	}
	c.contentsCachePath[ref.ID()] = tempFile.Name()

	// provide a io.ReadCloser that allocates a file handle on upon the first Read() call.
	return file.NewDeferredReadCloser(tempFile.Name()), nil
}

// FetchContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *FileCatalog) FileContents(f file.Reference) (io.ReadCloser, error) {
	entry, ok := c.catalog[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.RealPath)
	}

	// check and see if there is a cache hit for the current file, if so, use that
	if cacheValue, exists := c.contentsCachePath[f.ID()]; exists {
		return file.NewDeferredReadCloser(cacheValue), nil
	}

	// get header + content reader from the underlying tar
	tarEntry, err := entry.Layer.indexedContent.Entry(entry.Metadata.TarHeaderName)
	if err != nil {
		return nil, err
	}
	if tarEntry != nil {
		return c.handleContentResponse(f, tarEntry.Reader)
	}

	return nil, nil
}
