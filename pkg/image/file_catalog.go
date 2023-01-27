package image

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/scylladb/go-set/strset"

	"github.com/becheran/wildmatch-go"

	"github.com/anchore/stereoscope/pkg/file"
)

var ErrFileNotFound = fmt.Errorf("could not find file")

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	sync.RWMutex
	catalog     map[file.ID]FileCatalogEntry
	byMIMEType  map[string][]file.ID
	byExtension map[string][]file.ID
	byBasename  map[string][]file.ID
	basenames   *strset.Set
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
		catalog:     make(map[file.ID]FileCatalogEntry),
		byMIMEType:  make(map[string][]file.ID),
		byExtension: make(map[string][]file.ID),
		byBasename:  make(map[string][]file.ID),
		basenames:   strset.New(),
	}
}

// Add creates a new FileCatalogEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, l *Layer, opener file.Opener) {
	c.Lock()
	defer c.Unlock()
	id := f.ID()

	if m.MIMEType != "" {
		// an empty MIME type means that we didn't have the contents of the file to determine the MIME type. If we have
		// the contents and the MIME type could not be determined then the default value is application/octet-stream.
		c.byMIMEType[m.MIMEType] = append(c.byMIMEType[m.MIMEType], id)
	}

	basename := path.Base(string(f.RealPath))
	c.byBasename[basename] = append(c.byBasename[basename], id)
	c.basenames.Add(basename)

	// fmt.Println("Adding file to catalog: ", f.RealPath, " (", id, ")")
	for _, ext := range fileExtensions(string(f.RealPath)) {
		c.byExtension[ext] = append(c.byExtension[ext], id)
		// fmt.Println("   Extensions ("+ext+"): ", c.byExtension[ext])
	}

	c.catalog[id] = FileCatalogEntry{
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

func (c *FileCatalog) Basenames() []string {
	c.RLock()
	defer c.RUnlock()

	bns := c.basenames.List()
	sort.Strings(bns)
	return bns
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
			return nil, ErrFileNotFound
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *FileCatalog) GetByExtension(extension string) ([]FileCatalogEntry, error) {
	c.RLock()
	defer c.RUnlock()

	fileIDs, ok := c.byExtension[extension]
	if !ok {
		return nil, nil
	}

	var entries []FileCatalogEntry
	for _, id := range fileIDs {
		entry, ok := c.catalog[id]
		if !ok {
			return nil, ErrFileNotFound
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *FileCatalog) GetByBasename(basename string) ([]FileCatalogEntry, error) {
	c.RLock()
	defer c.RUnlock()

	if strings.Contains(basename, "/") {
		return nil, fmt.Errorf("found directory separator in a basename")
	}

	fileIDs, ok := c.byBasename[basename]
	if !ok {
		return nil, nil
	}

	var entries []FileCatalogEntry
	for _, id := range fileIDs {
		entry, ok := c.catalog[id]
		if !ok {
			return nil, ErrFileNotFound
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *FileCatalog) GetByBasenameGlob(globs ...string) ([]FileCatalogEntry, error) {
	c.RLock()
	defer c.RUnlock()

	var fileEntries []FileCatalogEntry
	basenames := c.Basenames()

	for _, glob := range globs {
		if strings.Contains(glob, "**") {
			return nil, fmt.Errorf("basename glob patterns with '**' are not supported")
		}
		if strings.Contains(glob, "/") {
			return nil, fmt.Errorf("found directory separator in a basename")
		}

		patternObj := wildmatch.NewWildMatch(glob)

		for _, b := range basenames {
			if patternObj.IsMatch(b) {
				bns, err := c.GetByBasename(b)
				if err != nil {
					return nil, fmt.Errorf("unable to fetch file references by basename (%q): %w", b, err)
				}
				fileEntries = append(fileEntries, bns...)
			}
		}
	}

	return fileEntries, nil
}

// FileContents reads the file contents for the given file reference from the underlying image/layer blob. An error
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

func fileExtensions(p string) []string {
	var exts []string
	p = strings.TrimSpace(p)

	// ignore oddities
	if strings.HasSuffix(p, ".") {
		return exts
	}

	// ignore directories
	if strings.HasSuffix(p, "/") {
		return exts
	}

	// ignore . which indicate a hidden file
	p = strings.TrimLeft(path.Base(p), ".")
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '.' {
			exts = append(exts, p[i:])
		}
	}
	return exts
}
