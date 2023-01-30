package filetree

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/becheran/wildmatch-go"
	"github.com/scylladb/go-set/strset"
)

type Index interface {
	IndexReader
	IndexWriter
}

type IndexReader interface {
	Exists(f file.Reference) bool
	Get(f file.Reference) (IndexEntry, error)
	GetByMIMEType(mType string) ([]IndexEntry, error)
	GetByExtension(extension string) ([]IndexEntry, error)
	GetByBasename(basename string) ([]IndexEntry, error)
	GetByBasenameGlob(globs ...string) ([]IndexEntry, error)
	Basenames() []string
}

type IndexWriter interface {
	Add(f file.Reference, m file.Metadata)
}

// Index represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type index struct {
	*sync.RWMutex
	index       map[file.ID]IndexEntry
	byMIMEType  map[string][]file.ID
	byExtension map[string][]file.ID
	byBasename  map[string][]file.ID
	basenames   *strset.Set
}

// NewIndex returns an empty Index.
func NewIndex() Index {
	return &index{
		RWMutex:     &sync.RWMutex{},
		index:       make(map[file.ID]IndexEntry),
		byMIMEType:  make(map[string][]file.ID),
		byExtension: make(map[string][]file.ID),
		byBasename:  make(map[string][]file.ID),
		basenames:   strset.New(),
	}
}

// IndexEntry represents all stored metadata for a single file reference.
type IndexEntry struct {
	file.Reference
	file.Metadata
}

// Add creates a new IndexEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *index) Add(f file.Reference, m file.Metadata) {
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

	for _, ext := range fileExtensions(string(f.RealPath)) {
		c.byExtension[ext] = append(c.byExtension[ext], id)
	}

	c.index[id] = IndexEntry{
		Reference: f,
		Metadata:  m,
	}
}

// Exists indicates if the given file reference exists in the index.
func (c *index) Exists(f file.Reference) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.index[f.ID()]
	return ok
}

// Get fetches a IndexEntry for the given file reference, or returns an error if the file reference has not
// been added to the index.
func (c *index) Get(f file.Reference) (IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()
	value, ok := c.index[f.ID()]
	if !ok {
		return IndexEntry{}, os.ErrNotExist
	}
	return value, nil
}

func (c *index) Basenames() []string {
	c.RLock()
	defer c.RUnlock()

	return c.basenames.List()
}

func (c *index) GetByMIMEType(mType string) ([]IndexEntry, error) {
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

func (c *index) GetByExtension(extension string) ([]IndexEntry, error) {
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

func (c *index) GetByBasename(basename string) ([]IndexEntry, error) {
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

func (c *index) GetByBasenameGlob(globs ...string) ([]IndexEntry, error) {
	c.RLock()
	defer c.RUnlock()

	var entries []IndexEntry
	for _, glob := range globs {
		if strings.Contains(glob, "**") {
			return nil, fmt.Errorf("basename glob patterns with '**' are not supported")
		}
		if strings.Contains(glob, "/") {
			return nil, fmt.Errorf("found directory separator in a basename")
		}

		patternObj := wildmatch.NewWildMatch(glob)
		for _, b := range c.Basenames() {
			if patternObj.IsMatch(b) {
				bns, err := c.GetByBasename(b)
				if err != nil {
					return nil, fmt.Errorf("unable to fetch file references by basename (%q): %w", b, err)
				}
				entries = append(entries, bns...)
			}
		}
	}

	return entries, nil
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
