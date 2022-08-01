package image

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/CalebQ42/squashfs"
	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"
)

const SingularitySquashFSLayer = "application/vnd.sylabs.sif.layer.v1.squashfs"

// Layer represents a single layer within a container image.
type Layer struct {
	// layer is the raw layer metadata and content provider from the GCR lib
	layer v1.Layer
	// indexedContent provides index access to the cached and unzipped layer tar
	indexedContent *file.TarIndex
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

func (l *Layer) uncompressedTarCache(uncompressedLayersCacheDir string) (string, error) {
	if uncompressedLayersCacheDir == "" {
		return "", fmt.Errorf("no cache directory given")
	}

	tarPath := path.Join(uncompressedLayersCacheDir, l.Metadata.Digest+".tar")

	if _, err := os.Stat(tarPath); !os.IsNotExist(err) {
		return tarPath, nil
	}

	rawReader, err := l.layer.Uncompressed()
	if err != nil {
		return "", err
	}

	fh, err := os.Create(tarPath)
	if err != nil {
		return "", fmt.Errorf("unable to create layer cache dir=%q : %w", tarPath, err)
	}

	if _, err := io.Copy(fh, rawReader); err != nil {
		return "", fmt.Errorf("unable to populate layer cache dir=%q : %w", tarPath, err)
	}

	return tarPath, nil
}

// Read parses information from the underlying layer tar into this struct. This includes layer metadata, the layer
// file tree, and the layer squash tree.
func (l *Layer) Read(catalog *FileCatalog, imgMetadata Metadata, idx int, uncompressedLayersCacheDir string) error {
	var err error
	l.Tree = filetree.NewFileTree()
	l.fileCatalog = catalog
	l.Metadata, err = newLayerMetadata(imgMetadata, l.layer, idx)
	if err != nil {
		return err
	}

	log.Debugf("layer metadata: index=%+v digest=%+v mediaType=%+v",
		l.Metadata.Index,
		l.Metadata.Digest,
		l.Metadata.MediaType)

	monitor := trackReadProgress(l.Metadata)

	switch l.Metadata.MediaType {
	case types.OCILayer,
		types.OCIUncompressedLayer,
		types.OCIRestrictedLayer,
		types.OCIUncompressedRestrictedLayer,
		types.DockerLayer,
		types.DockerForeignLayer,
		types.DockerUncompressedLayer:

		tarFilePath, err := l.uncompressedTarCache(uncompressedLayersCacheDir)
		if err != nil {
			return err
		}

		l.indexedContent, err = file.NewTarIndex(tarFilePath, l.indexer(monitor))
		if err != nil {
			return fmt.Errorf("failed to read layer=%q tar : %w", l.Metadata.Digest, err)
		}

	case SingularitySquashFSLayer:
		r, err := l.layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("failed to read layer=%q: %w", l.Metadata.Digest, err)
		}
		// defer r.Close() // TODO: if we close this here, we can't read file contents after we return.

		// Walk the more efficient walk if we're blessed with an io.ReaderAt.
		if ra, ok := r.(io.ReaderAt); ok {
			err = file.WalkSquashFS(ra, l.squashfsVisitor(monitor))
		} else {
			err = file.WalkSquashFSFromReader(r, l.squashfsVisitor(monitor))
		}
		if err != nil {
			return fmt.Errorf("failed to walk layer=%q: %w", l.Metadata.Digest, err)
		}

	default:
		return fmt.Errorf("unknown layer media type: %+v", l.Metadata.MediaType)
	}

	monitor.SetCompleted()

	return nil
}

// FetchContents reads the file contents for the given path from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
func (l *Layer) FileContents(path file.Path) (io.ReadCloser, error) {
	return fetchFileContentsByPath(l.Tree, l.fileCatalog, path)
}

// FileContentsFromSquash reads the file contents for the given path from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
func (l *Layer) FileContentsFromSquash(path file.Path) (io.ReadCloser, error) {
	return fetchFileContentsByPath(l.SquashedTree, l.fileCatalog, path)
}

// FilesByMIMEType returns file references for files that match at least one of the given MIME types relative to each layer tree.
func (l *Layer) FilesByMIMEType(mimeTypes ...string) ([]file.Reference, error) {
	var refs []file.Reference
	for _, ty := range mimeTypes {
		refsForType, err := fetchFilesByMIMEType(l.Tree, l.fileCatalog, ty)
		if err != nil {
			return nil, err
		}
		refs = append(refs, refsForType...)
	}
	return refs, nil
}

// FilesByMIMETypeFromSquash returns file references for files that match at least one of the given MIME types relative to the squashed file tree representation.
func (l *Layer) FilesByMIMETypeFromSquash(mimeTypes ...string) ([]file.Reference, error) {
	var refs []file.Reference
	for _, ty := range mimeTypes {
		refsForType, err := fetchFilesByMIMEType(l.SquashedTree, l.fileCatalog, ty)
		if err != nil {
			return nil, err
		}
		refs = append(refs, refsForType...)
	}
	return refs, nil
}

func (l *Layer) indexer(monitor *progress.Manual) file.TarIndexVisitor {
	return func(index file.TarIndexEntry) error {
		var err error
		var entry = index.ToTarFileEntry()

		var contents = index.Open()
		defer func() {
			if err := contents.Close(); err != nil {
				log.Warnf("unable to close file while indexing layer: %+v", err)
			}
		}()
		metadata := file.NewMetadata(entry.Header, entry.Sequence, contents)

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
		l.fileCatalog.Add(*fileReference, metadata, l, index.Open)

		monitor.N++
		return nil
	}
}

func (l *Layer) squashfsVisitor(monitor *progress.Manual) file.SquashFSVisitor {
	return func(fsys *squashfs.FS, path string, d fs.DirEntry) error {
		f, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		metadata, err := file.NewMetadataFromSquashFSFile(path, f.(*squashfs.File))
		if err != nil {
			return err
		}

		var fileReference *file.Reference

		switch f := f.(*squashfs.File); {
		case f.IsSymlink():
			fileReference, err = l.Tree.AddSymLink(file.Path(metadata.Path), file.Path(metadata.Linkname))
			if err != nil {
				return err
			}
		case f.IsDir():
			fileReference, err = l.Tree.AddDir(file.Path(metadata.Path))
			if err != nil {
				return err
			}
		case f.IsRegular():
			fileReference, err = l.Tree.AddFile(file.Path(metadata.Path))
			if err != nil {
				return err
			}
		}

		if fileReference == nil {
			return fmt.Errorf("could not add path=%q link=%q during squashfs iteration", metadata.Path, metadata.Linkname)
		}

		l.Metadata.Size += metadata.Size
		l.fileCatalog.Add(*fileReference, metadata, l, func() io.ReadCloser {
			r, err := fsys.Open(path)
			if err != nil {
				// The file.Opener interface doesn't give us a way to return an error, and callers
				// don't seem to handle a nil return. So, return a zero-byte reader.
				log.Debug(err)
				return io.NopCloser(bytes.NewReader(nil)) // TODO
			}
			return r
		})

		monitor.N++
		return nil
	}
}

func trackReadProgress(metadata LayerMetadata) *progress.Manual {
	p := &progress.Manual{}

	bus.Publish(partybus.Event{
		Type:   event.ReadLayer,
		Source: metadata,
		Value:  progress.Monitorable(p),
	})

	return p
}
