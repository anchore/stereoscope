package image

import (
	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"
)

// Layer represents a single layer within a container image.
type Layer struct {
	// content is the layer metadata and content provider
	content v1.Layer
	// Metadata contains select layer attributes
	Metadata LayerMetadata
	// Tree is a filetree that represents the structure of the layer tar contents ("diff tree")
	Tree *tree.FileTree
	// SquashedTree is a filetree that represents the combination of this layers diff tree and all diff trees
	// in lower layers relative to this one.
	SquashedTree *tree.FileTree
	// fileCatalog contains all file metadata for all files in all layers (not just this layer)
	fileCatalog *FileCatalog
}

// NewLayer provides a new, unread layer object.
func NewLayer(content v1.Layer) *Layer {
	return &Layer{
		content: content,
	}
}

func (l *Layer) trackReadProgress(metadata LayerMetadata) *progress.Manual {
	prog := &progress.Manual{}

	bus.Publish(partybus.Event{
		Type:   event.ReadLayer,
		Source: metadata,
		Value:  progress.Monitorable(prog),
	})

	return prog
}

// Read parses information from the underlying layer tar into this struct. This includes layer metadata, the layer
// file tree, and the layer squash tree.
func (l *Layer) Read(catalog *FileCatalog, imgMetadata Metadata, idx int) error {
	// TODO: side effects are bad
	metadata, err := readLayerMetadata(imgMetadata, l.content, idx)
	if err != nil {
		return err
	}

	log.Debugf("layer metadata: index=%+v digest=%+v mediaType=%+v",
		metadata.Index,
		metadata.Digest,
		metadata.MediaType)

	l.Metadata = metadata
	l.Tree = tree.NewFileTree()
	l.fileCatalog = catalog

	monitor := l.trackReadProgress(metadata)

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

		monitor.N++
	}
	monitor.SetCompleted()

	return nil
}

// FetchContents reads the file contents for the given path from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) FileContents(path file.Path) (string, error) {
	return fetchFileContentsByPath(l.Tree, l.fileCatalog, path)
}

// MultipleFileContents reads the file contents for all given paths from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if any one file path does not exist or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) MultipleFileContents(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(l.Tree, l.fileCatalog, paths...)
}

// FileContentsFromSquash reads the file contents for the given path from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) FileContentsFromSquash(path file.Path) (string, error) {
	return fetchFileContentsByPath(l.SquashedTree, l.fileCatalog, path)
}

// MultipleFileContents reads the file contents for all given paths from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if any one file path does not exist or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) MultipleFileContentsFromSquash(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(l.SquashedTree, l.fileCatalog, paths...)
}
