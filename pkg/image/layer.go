package image

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
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
	Tree *filetree.FileTree
	// SquashedTree is a filetree that represents the combination of this layers diff tree and all diff trees
	// in lower layers relative to this one.
	SquashedTree *filetree.FileTree
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
	l.Tree = filetree.NewFileTree()

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
		// note: the tar header name is independent of surrounding structure, for example, there may be a tar header entry
		// for /some/path/to/file.txt without any entries to constituent paths (/some, /some/path, /some/path/to ).
		// This is ok, and the FileTree will account for this by automatically adding directories for non-existing
		// constituent paths. If later there happens to be a tar header entry for an already added constituent path
		// the FileNode will be updated with the new file.Reference. If there is no tar header entry for constituent
		// paths the FileTree is still structurally consistent (all paths can be iterated even though there may not have
		// been a tar header entry for part of the given path).
		//
		// In summary: the set of all FileTrees can have NON-leaf nodes that don't exist in the FileCatalog, but
		// the FileCatalog should NEVER have entries that don't appear in one (or more) FileTree(s).
		var fileReference *file.Reference
		switch metadata.TypeFlag {
		case tar.TypeSymlink:
			fileReference, err = l.Tree.AddSymLink(file.Path(metadata.Path), file.Path(metadata.Linkname))
			if err != nil {
				return err
			}
		case tar.TypeLink:
			fileReference, err = l.Tree.AddHardLink(file.Path(metadata.Path), file.Path(metadata.Linkname))
			if err != nil {
				return err
			}
		case tar.TypeDir:
			fileReference, err = l.Tree.AddDir(file.Path(metadata.Path))
			if err != nil {
				return err
			}
		default:
			fileReference, err = l.Tree.AddFile(file.Path(metadata.Path))
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
