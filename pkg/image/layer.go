package image

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

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
	// layer is the raw layer metadata and content provider from the GCR lib
	layer v1.Layer
	// content provides an io.ReadCloser for the underlying layer tar (either directly from the GCR lib or a cache dir)
	content file.OpenerFn
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
func NewLayer(layer v1.Layer) *Layer {
	return &Layer{
		layer: layer,
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

// readMetadata populates layer metadata from the underlying layer tar.
func (l *Layer) readMetadata(imgMetadata Metadata, idx int, uncompressedLayersCacheDir string) error {
	metadata, err := readLayerMetadata(imgMetadata, l.layer, idx)
	if err != nil {
		return err
	}

	log.Debugf("layer metadata: index=%+v digest=%+v mediaType=%+v",
		metadata.Index,
		metadata.Digest,
		metadata.MediaType)

	l.Metadata = metadata
	l.Tree = tree.NewFileTree()

	if uncompressedLayersCacheDir != "" {
		rawReader, err := l.layer.Uncompressed()
		if err != nil {
			return err
		}

		tarPath := path.Join(uncompressedLayersCacheDir, l.Metadata.Digest+".tar")

		fh, err := os.Create(tarPath)
		if err != nil {
			return fmt.Errorf("unable to create layer cache dir=%q : %w", tarPath, err)
		}

		if _, err := io.Copy(fh, rawReader); err != nil {
			return fmt.Errorf("unable to populate layer cache dir=%q : %w", tarPath, err)
		}

		l.content = file.OpenerFromPath{Path: tarPath}.Open
	} else {
		l.content = l.layer.Uncompressed
	}
	return nil
}

// Read parses information from the underlying layer tar into this struct. This includes layer metadata, the layer
// file tree, and the layer squash tree.
func (l *Layer) Read(catalog *FileCatalog, imgMetadata Metadata, idx int, uncompressedLayersCacheDir string) error {
	if err := l.readMetadata(imgMetadata, idx, uncompressedLayersCacheDir); err != nil {
		return err
	}

	l.fileCatalog = catalog

	reader, err := l.content()
	if err != nil {
		return fmt.Errorf("unable to obtail layer=%q tar: %w", l.Metadata.Digest, err)
	}
	monitor := l.trackReadProgress(l.Metadata)

	for metadata := range file.EnumerateFileMetadataFromTar(reader) {
		var fileReference *file.Reference
		switch metadata.TypeFlag {
		case tar.TypeSymlink:
			// symlinks can by relative or absolute path references, take the data as is
			fileReference, err = l.Tree.AddLink(file.Path(metadata.Path), file.Path(metadata.Linkname))
			if err != nil {
				return err
			}
		case tar.TypeLink:
			// hard link MUST be interpreted as an absolute path
			p := filepath.Clean(file.DirSeparator + metadata.Linkname)
			fileReference, err = l.Tree.AddLink(file.Path(metadata.Path), file.Path(p))
			if err != nil {
				return err
			}
		default:
			fileReference, err = l.Tree.AddPath(file.Path(metadata.Path))
			if err != nil {
				return err
			}
		}
		if fileReference == nil {
			return fmt.Errorf("could not add path=%q link=%q during tar iteration", metadata.Path, metadata.Linkname)
		}

		l.Metadata.Size += metadata.Size
		catalog.Add(*fileReference, metadata, l)

		monitor.N++
	}

	// TODO: It's possible that directories can be added to the FileTree that aren't stored in the FileCatalog.
	//  Given this, we should think about the extent to which entries in the tree should be present in the catalog,
	//  and we should consider the impact to consumers as they query this library for "directories" in the image.

	monitor.SetCompleted()

	return nil
}

// FetchContents reads the file contents for the given path from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) FileContents(path file.Path) (io.ReadCloser, error) {
	return fetchFileContentsByPath(l.Tree, l.fileCatalog, path)
}

// MultipleFileContents reads the file contents for all given paths from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if any one file path does not exist or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) MultipleFileContents(paths ...file.Path) (map[file.Reference]io.ReadCloser, error) {
	return fetchMultipleFileContentsByPath(l.Tree, l.fileCatalog, paths...)
}

// FileContentsFromSquash reads the file contents for the given path from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) FileContentsFromSquash(path file.Path) (io.ReadCloser, error) {
	return fetchFileContentsByPath(l.SquashedTree, l.fileCatalog, path)
}

// MultipleFileContents reads the file contents for all given paths from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if any one file path does not exist or the read operation cannot continue.
// This is a convenience function provided by the FileCatalog.
func (l *Layer) MultipleFileContentsFromSquash(paths ...file.Path) (map[file.Reference]io.ReadCloser, error) {
	return fetchMultipleFileContentsByPath(l.SquashedTree, l.fileCatalog, paths...)
}
