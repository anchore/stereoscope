package image

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
)

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
	Source   *Layer
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
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, s *Layer) {
	c.catalog[f.ID()] = &FileCatalogEntry{
		File:     f,
		Metadata: m,
		Source:   s,
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
		return FileCatalogEntry{}, fmt.Errorf("could not find file")
	}
	return *value, nil
}

// handleContentResponse returns a io.ReadCloser for the given file reference that does not take up precious file
// descriptors until the first Read() call on the io.ReadCloser. This function is additionally responsible for handling
// caching of previous results into a cache directory in case future calls are interested in the results as well as
// provide a non-memory-intensive Reader for the file reference by storing to disk.
func (c *FileCatalog) handleContentResponse(ref file.Reference, contents io.Reader) (io.ReadCloser, error) {
	entry, err := c.Get(ref)
	if err != nil {
		return nil, err
	}

	if entry.Metadata.Size <= cacheFileSizeThreshold {
		return ioutil.NopCloser(contents), nil
	}

	// check to see if this is already in the cache, if so, return a reader to the cache reference instead
	if p, ok := c.contentsCachePath[ref.ID()]; ok {
		return file.NewDeferredReadCloser(p), nil
	}

	// cache the result to a directory and return a DeferredReadCloser to not allocate file handles unless they are
	// actively being used.
	tempFile, err := ioutil.TempFile(c.contentsCacheDir, ref.Path.Basename())
	if err != nil {
		return nil, err
	}
	defer tempFile.Close()

	// stream the contents from the reader directly into the temp file
	if _, err := io.Copy(tempFile, contents); err != nil {
		return nil, err
	}

	// provide a io.ReadCloser that allocates a file handle on upon the first Read() call.
	return file.NewDeferredReadCloser(tempFile.Name()), nil
}

// FetchContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *FileCatalog) FileContents(f file.Reference) (io.ReadCloser, error) {
	entry, ok := c.catalog[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.Path)
	}
	sourceTarReader, err := entry.Source.content.Uncompressed()
	if err != nil {
		return nil, err
	}
	// note: this is the reader for the underlying tar, we should
	fileReader, err := file.ReaderFromTar(sourceTarReader, entry.Metadata.TarHeaderName)
	if err != nil {
		return nil, err
	}
	defer fileReader.Close()

	return c.handleContentResponse(f, fileReader)
}

// MultipleFileContents returns the contents of all provided file references. Returns an error if any of the file
// references does not exist in the underlying layer tars.
func (c *FileCatalog) MultipleFileContents(files ...file.Reference) (map[file.Reference]io.ReadCloser, error) {
	requestsByLayer, err := c.buildTarContentsRequests(files...)
	if err != nil {
		return nil, err
	}

	results := make(map[file.Reference]io.ReadCloser)
	for layer, tarHeaderNameToFileReference := range requestsByLayer {
		sourceTarReader, err := layer.content.Uncompressed()
		if err != nil {
			return nil, err
		}
		discoveredFiles := 0

		// we generate the TarVisitor dynamically to prevent usage of the loop variables within the function literal
		visitor := func(tarHeaderNameToFileReference file.TarContentsRequest) file.TarVisitor {
			// create a visitor function tailored for reading the contents of files in the current request and
			// handling the content request via the FileCatalog (for caching and normalizing the io.ReadCloser returned)
			return func(header *tar.Header, contents io.Reader) error {
				if fileRef, ok := tarHeaderNameToFileReference[header.Name]; ok {
					discoveredFiles++
					// process the given tar entry
					if _, ok := results[fileRef]; ok {
						return fmt.Errorf("duplicate entries: %+v", fileRef)
					}
					results[fileRef], err = c.handleContentResponse(fileRef, contents)
					if err != nil {
						return err
					}
				}

				if discoveredFiles == len(tarHeaderNameToFileReference) {
					return file.ErrTarStopIteration
				}
				return nil
			}
		}(tarHeaderNameToFileReference)

		if err := file.TarIterator(sourceTarReader, visitor); err != nil {
			return nil, err
		}
	}

	return results, nil
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

// TODO: translate this to a leaf-check? Also does this need to be directly on the FileCatalog?
// HasEntriesForAllFilesInTree checks to see if the catalog has an entry for
// every node ( file / directory) in the FileTree.
func (c *FileCatalog) HasEntriesForAllFilesInTree(tree tree.FileTree) bool {
	for _, f := range tree.AllFiles() {
		if !c.Exists(f) {
			return false
		}
	}

	return true
}
