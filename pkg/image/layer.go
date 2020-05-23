package image

import (
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Layer struct {
	content  v1.Layer
	Metadata LayerMetadata
	Tree     *tree.FileTree
	// note: this is a reference to all files in the image, not just this layer
	fileCatalog *FileCatalog
}

func NewLayer(content v1.Layer) Layer {
	return Layer{
		content: content,
	}
}

func (l *Layer) Read(catalog *FileCatalog, imgMetadata Metadata, idx int) error {
	// TODO: side effects are bad
	metadata, err := readLayerMetadata(imgMetadata, l.content, idx)
	if err != nil {
		return err
	}

	log.Debugf("layer metadata:\n\tdigest=%+v\n\tmediaType=%+v\n\tindex=%+v",
		metadata.Digest,
		metadata.MediaType,
		metadata.Index)

	l.Metadata = metadata
	l.Tree = tree.NewFileTree()
	l.fileCatalog = catalog

	reader, err := l.content.Uncompressed()
	if err != nil {
		return err
	}

	for metadata := range file.EnumerateFileMetadataFromTar(reader) {
		fileNode, err := l.Tree.AddPath(file.Path(metadata.Path))
		l.Metadata.Size += metadata.Size

		catalog.Add(fileNode, metadata, l)

		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Layer) FileContents(path file.Path) (string, error) {
	return fetchFileContentsByPath(l.Tree, l.fileCatalog, path)
}

func (l *Layer) MultipleFileContents(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(l.Tree, l.fileCatalog, paths...)
}
