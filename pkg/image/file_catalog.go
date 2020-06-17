package image

import (
	"fmt"
	"io/ioutil"

	"github.com/anchore/stereoscope/pkg/file"
)

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	catalog map[file.ID]*FileCatalogEntry
}

// FileCatalogEntry represents all stored metadata for a single file reference.
type FileCatalogEntry struct {
	File     file.Reference
	Metadata file.Metadata
	Source   *Layer
}

// NewFileCatalog returns an empty FileCatalog.
func NewFileCatalog() FileCatalog {
	return FileCatalog{
		catalog: make(map[file.ID]*FileCatalogEntry),
	}
}

// Add creates a new FileCatalogEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, s *Layer) {
	c.catalog[f.ID()] = &FileCatalogEntry{
		File:     f,
		Metadata: m,
		Source:   s,
	}
}

// Get fetches a FileCatalogEntry for the given file reference, or returns an error if the file reference has not
// been added to the catalog.
func (c *FileCatalog) Get(f file.Reference) (FileCatalogEntry, error) {
	value, ok := c.catalog[f.ID()]
	if !ok {
		return FileCatalogEntry{}, fmt.Errorf("could not find file")
	}
	return *value, nil
}

// FetchContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *FileCatalog) FileContents(f file.Reference) (string, error) {
	entry, ok := c.catalog[f.ID()]
	if !ok {
		return "", fmt.Errorf("could not find file: %+v", f.Path)
	}
	sourceTarReader, err := entry.Source.content.Uncompressed()
	if err != nil {
		return "", err
	}
	fileReader, err := file.ReaderFromTar(sourceTarReader, entry.Metadata.TarHeaderName)
	if err != nil {
		return "", err
	}
	bytes, err := ioutil.ReadAll(fileReader)
	return string(bytes), err
}

// buildTarContentsRequests orders the set of file references for each layer to optimize the image tar reading process
// to be consisted of only sequential reads, so read requests are only a single pass through the image tar.
func (c *FileCatalog) buildTarContentsRequests(files ...file.Reference) (map[*Layer]file.TarContentsRequest, error) {
	allRequests := make(map[*Layer]file.TarContentsRequest)
	for _, f := range files {
		record, err := c.Get(f)
		if err != nil {
			return nil, err
		}
		layer := record.Source
		if _, ok := allRequests[layer]; !ok {
			allRequests[layer] = make(file.TarContentsRequest)
		}
		allRequests[layer][record.Metadata.TarHeaderName] = f
	}
	return allRequests, nil
}

// MultipleFileContents returns the contents of all provided file references. Returns an error if any of the file
// references does not exist in the underlying layer tars.
func (c *FileCatalog) MultipleFileContents(files ...file.Reference) (map[file.Reference]string, error) {
	allRequests, err := c.buildTarContentsRequests(files...)
	if err != nil {
		return nil, err
	}

	allResults := make(map[file.Reference]string)
	for layer, request := range allRequests {
		sourceTarReader, err := layer.content.Uncompressed()
		if err != nil {
			return nil, err
		}
		layerResults, err := file.ContentsFromTar(sourceTarReader, request)
		if err != nil {
			return nil, err
		}
		for fileRef, content := range layerResults {
			if _, ok := allResults[fileRef]; ok {
				return nil, fmt.Errorf("duplicate entries: %+v", fileRef)
			}
			allResults[fileRef] = content
		}
	}

	return allResults, nil
}
