package image

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Layer struct {
	Index   uint
	Content v1.Layer
	Tree    *tree.FileTree
	// note: this is a reference to all files in the image, not just this layer
	fileCatalog *FileCatalog
}

func NewLayer(index uint, content v1.Layer) Layer {
	return Layer{
		Index:   index,
		Content: content,
	}
}

func (l *Layer) Read(catalog *FileCatalog) error {
	// TODO: side effects are bad
	l.Tree = tree.NewFileTree()
	l.fileCatalog = catalog

	reader, err := l.Content.Uncompressed()
	if err != nil {
		return err
	}

	for metadata := range file.EnumerateFileMetadataFromTar(reader) {
		fileNode, err := l.Tree.AddPath(file.Path(metadata.Path))

		catalog.Add(fileNode, metadata, l)

		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Layer) FileContents(path file.Path) (string, error) {
	return fetchFileContents(l.Tree, l.fileCatalog, path)
}

func (l *Layer) MultipleFileContents(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContents(l.Tree, l.fileCatalog, paths...)
}
