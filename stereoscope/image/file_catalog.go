package image

import (
	"archive/tar"
	"fmt"
	"github.com/anchore/stereoscope/stereoscope/file"
	"io"
)

type FileCatalogEntry struct {
	File     *file.File
	Metadata file.Metadata
	Source   *Layer
}

type FileCatalog struct {
	catalog map[file.ID]FileCatalogEntry
}

func NewFileCatalog() FileCatalog {
	return FileCatalog{
		catalog: make(map[file.ID]FileCatalogEntry),
	}
}

func (c *FileCatalog) Add(f *file.File, m file.Metadata, s *Layer) {
	c.catalog[f.ID()] = FileCatalogEntry{
		File:     f,
		Metadata: m,
		Source:   s,
	}
}

func (c *FileCatalog) FileReader(f *file.File) (io.ReadCloser, error) {
	entry, ok := c.catalog[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.Path)
	}
	source, err := entry.Source.Content.Uncompressed()
	if err != nil {
		return nil, err
	}
	return extractFileFromTar(source, entry.Metadata.TarPath)
}

type tarFile struct {
	io.Reader
	io.Closer
}

func extractFileFromTar(reader io.ReadCloser, path string) (io.ReadCloser, error) {
	tf := tar.NewReader(reader)
	for {
		hdr, err := tf.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == path {
			return tarFile{
				Reader: tf,
				Closer: reader,
			}, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in tar", path)
}
