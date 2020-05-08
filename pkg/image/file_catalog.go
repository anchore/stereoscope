package image

import (
	"fmt"
	"github.com/anchore/stereoscope/pkg/file"
	"io/ioutil"
)

type FileCatalogEntry struct {
	File     file.Reference
	Metadata file.Metadata
	Source   *Layer
}

type FileCatalog struct {
	catalog map[file.ID]*FileCatalogEntry
}

func NewFileCatalog() FileCatalog {
	return FileCatalog{
		catalog: make(map[file.ID]*FileCatalogEntry),
	}
}

func (c *FileCatalog) Add(f file.Reference, m file.Metadata, s *Layer) {
	c.catalog[f.ID()] = &FileCatalogEntry{
		File:     f,
		Metadata: m,
		Source:   s,
	}
}

func (c *FileCatalog) Get(f file.Reference) (FileCatalogEntry, error) {
	value, ok := c.catalog[f.ID()]
	if !ok {
		return FileCatalogEntry{}, fmt.Errorf("could not find file")
	}
	return *value, nil
}

func (c *FileCatalog) FileContents(f file.Reference) (string, error) {
	entry, ok := c.catalog[f.ID()]
	if !ok {
		return "", fmt.Errorf("could not find file: %+v", f.Path)
	}
	sourceTarReader, err := entry.Source.Content.Uncompressed()
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

func (c *FileCatalog) MultipleFileContents(files ...file.Reference) (map[file.Reference]string, error) {
	allRequests, err := c.buildTarContentsRequests(files...)
	if err != nil {
		return nil, err
	}

	allResults := make(map[file.Reference]string)
	for layer, request := range allRequests {
		sourceTarReader, err := layer.Content.Uncompressed()
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
