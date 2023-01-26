package file

import (
	"fmt"
	"github.com/becheran/wildmatch-go"
	"io"
	"os"
	"path"
	"strings"
	"sync"
)

// Index represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type Index struct {
	*sync.RWMutex
	index       map[ID]IndexEntry
	byMIMEType  map[string][]ID
	byExtension map[string][]ID
	byBasename  map[string][]ID
	basenames   []string
}

// NewIndex returns an empty Index.
func NewIndex() *Index {
	return &Index{
		RWMutex:     &sync.RWMutex{},
		index:       make(map[ID]IndexEntry),
		byMIMEType:  make(map[string][]ID),
		byExtension: make(map[string][]ID),
		byBasename:  make(map[string][]ID),
	}
}

// IndexEntry represents all stored metadata for a single file reference.
type IndexEntry struct {
	Reference
	Metadata
	Opener
}

// Add creates a new IndexEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *Index) Add(f Reference, m Metadata, opener Opener) {
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
	c.basenames = append(c.basenames, basename)

	for _, ext := range fileExtensions(string(f.RealPath)) {
		c.byExtension[ext] = append(c.byExtension[ext], id)
	}

	c.index[id] = IndexEntry{
		Reference: f,
		Metadata:  m,
		Opener:    opener,
	}
}

// Exists indicates if the given file reference exists in the index.
func (c *Index) Exists(f Reference) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.index[f.ID()]
	return ok
}

// Get fetches a IndexEntry for the given file reference, or returns an error if the file reference has not
// been added to the index.
func (c *Index) Get(f Reference) (IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()
	value, ok := c.index[f.ID()]
	if !ok {
		return IndexEntry{}, os.ErrNotExist
	}
	return value, nil
}

func (c *Index) Basenames() []string {
	c.RLock()
	defer c.RUnlock()

	return c.basenames
}

func (c *Index) GetByMIMEType(mType string) ([]IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()

	fileIDs, ok := c.byMIMEType[mType]
	if !ok {
		return nil, nil
	}

	var entries []IndexEntry
	for _, id := range fileIDs {
		entry, ok := c.index[id]
		if !ok {
			return nil, os.ErrNotExist
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Index) GetByExtension(extension string) ([]IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()

	fileIDs, ok := c.byExtension[extension]
	if !ok {
		return nil, nil
	}

	var entries []IndexEntry
	for _, id := range fileIDs {
		entry, ok := c.index[id]
		if !ok {
			return nil, os.ErrNotExist
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Index) GetByBasename(basename string) ([]IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()

	if strings.Contains(basename, "/") {
		return nil, fmt.Errorf("found directory separator in a basename")
	}

	fileIDs, ok := c.byBasename[basename]
	if !ok {
		return nil, nil
	}

	var entries []IndexEntry
	for _, id := range fileIDs {
		entry, ok := c.index[id]
		if !ok {
			return nil, os.ErrNotExist
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Index) GetByBasenameGlob(glob string) ([]IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()

	if strings.Contains(glob, "**") {
		return nil, fmt.Errorf("basename glob patterns with '**' are not supported")
	}
	if strings.Contains(glob, "/") {
		return nil, fmt.Errorf("found directory separator in a basename")
	}

	patternObj := wildmatch.NewWildMatch(glob)

	var entries []IndexEntry
	for _, b := range c.Basenames() {
		if patternObj.IsMatch(b) {
			bns, err := c.GetByBasename(b)
			if err != nil {
				return nil, fmt.Errorf("unable to fetch file references by basename (%q): %w", b, err)
			}
			entries = append(entries, bns...)
		}
	}

	return entries, nil
}

// FileContents reads the file contents for the given file reference from the underlying image/layer blob. An error
// is returned if there is no file at the given path and layer or the read operation cannot continue.
func (c *Index) FileContents(f Reference) (io.ReadCloser, error) {
	c.RLock()
	defer c.RUnlock()
	entry, ok := c.index[f.ID()]
	if !ok {
		return nil, fmt.Errorf("could not find file: %+v", f.RealPath)
	}

	if entry.Opener == nil {
		return nil, fmt.Errorf("no contents available for file: %+v", f.RealPath)
	}

	return entry.Opener(), nil
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
