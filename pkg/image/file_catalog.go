package image

import (
	"github.com/anchore/stereoscope/pkg/file"
)

// FileCatalog represents all file metadata and source tracing for all files contained within the image layer
// blobs (i.e. everything except for the image index/manifest/metadata files).
type FileCatalog struct {
	*file.Index
	layerByID map[file.ID]*Layer
	//openerByID map[file.ID]file.Opener
}

// FileCatalogEntry represents all stored metadata for a single file reference.
type FileCatalogEntry struct {
	file.IndexEntry
	Layer *Layer
}

// NewFileCatalog returns an empty FileCatalog.
func NewFileCatalog() *FileCatalog {
	return &FileCatalog{
		Index:     file.NewIndex(),
		layerByID: make(map[file.ID]*Layer),
		//openerByID: make(map[file.ID]file.Opener),
	}
}

// Add creates a new FileCatalogEntry for the given file reference and metadata, cataloged by the ID of the
// file reference (overwriting any existing entries without warning).
func (c *FileCatalog) Add(f file.Reference, m file.Metadata, l *Layer, opener file.Opener) {
	c.Index.Add(f, m, opener)

	c.Lock()
	defer c.Unlock()
	c.layerByID[f.ID()] = l
	//c.openerByID[f.ID()] = opener
}

//// FileContents reads the file contents for the given file reference from the underlying image/layer blob. An error
//// is returned if there is no file at the given path and layer or the read operation cannot continue.
//func (c *FileCatalog) FileContents(f file.Reference) (io.ReadCloser, error) {
//	c.RLock()
//	defer c.RUnlock()
//	opener, ok := c.openerByID[f.ID()]
//	if !ok {
//		return nil, fmt.Errorf("could not find file: %+v", f.RealPath)
//	}
//
//	if opener == nil {
//		return nil, fmt.Errorf("no contents available for file: %+v", f.RealPath)
//	}
//
//	return opener(), nil
//}

func (c *FileCatalog) Layer(f file.Reference) *Layer {
	c.RLock()
	defer c.RUnlock()

	return c.layerByID[f.ID()]
}
