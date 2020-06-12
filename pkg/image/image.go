package image

import (
	"archive/tar"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Image struct {
	// The image metadata and content source
	content v1.Image
	// Select image attributes
	Metadata Metadata
	// Ordered listing of Layers
	Layers []*Layer
	// A {file.Reference -> (file.Metadata, Layer, Path)} mapping for all files in all layers
	FileCatalog FileCatalog
}

func NewImage(image v1.Image) *Image {
	return &Image{
		content:     image,
		FileCatalog: NewFileCatalog(),
	}
}

func NewImageWithTags(image v1.Image, tags []name.Tag) *Image {
	return &Image{
		content:     image,
		FileCatalog: NewFileCatalog(),
		Metadata: Metadata{
			Tags: tags,
		},
	}
}

func (i *Image) Read() error {
	var layers = make([]*Layer, 0)

	metadata, err := readImageMetadata(i.content)
	if err != nil {
		return err
	}
	if i.Metadata.Tags != nil {
		metadata.Tags = i.Metadata.Tags
	}
	i.Metadata = metadata

	log.Debugf("image metadata: digest=%+v mediaType=%+v tags=%+v",
		metadata.Digest,
		metadata.MediaType,
		metadata.Tags)

	v1Layers, err := i.content.Layers()
	if err != nil {
		return err
	}

	for idx, v1Layer := range v1Layers {
		layer := NewLayer(v1Layer)
		err := layer.Read(&i.FileCatalog, i.Metadata, idx)
		if err != nil {
			return err
		}
		i.Metadata.Size += layer.Metadata.Size
		layers = append(layers, layer)
	}

	i.Layers = layers

	// in order to resolve symlinks all squashed trees must be available
	return i.squash()
}

func (i *Image) squash() error {
	var lastSquashTree *tree.FileTree
	for idx, layer := range i.Layers {
		if idx == 0 {
			lastSquashTree = layer.Tree
			layer.SquashedTree = layer.Tree
			continue
		}

		var unionTree = tree.NewUnionTree()
		unionTree.PushTree(lastSquashTree)
		unionTree.PushTree(layer.Tree)

		squashedTree, err := unionTree.Squash()
		if err != nil {
			return fmt.Errorf("failed to squash tree %d: %w", idx, err)
		}

		layer.SquashedTree = squashedTree
		lastSquashTree = squashedTree
	}

	return nil
}

func (i *Image) SquashedTree() *tree.FileTree {
	return i.Layers[len(i.Layers)-1].SquashedTree
}

func (i *Image) FileContentsFromSquash(path file.Path) (string, error) {
	return fetchFileContentsByPath(i.SquashedTree(), &i.FileCatalog, path)
}

func (i *Image) MultipleFileContentsFromSquash(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(i.SquashedTree(), &i.FileCatalog, paths...)
}

func (i *Image) FileContentsByRef(ref file.Reference) (string, error) {
	content, err := i.FileCatalog.FileContents(ref)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (i *Image) MultipleFileContentsByRef(refs ...file.Reference) (map[file.Reference]string, error) {
	content, err := i.FileCatalog.MultipleFileContents(refs...)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (i *Image) ResolveLinkByLayerSquash(ref file.Reference, layer int) (*file.Reference, error) {
	return i.resolveLink(ref, i.Layers[layer].SquashedTree)
}
func (i *Image) ResolveLinkByImageSquash(ref file.Reference) (*file.Reference, error) {
	return i.resolveLink(ref, i.Layers[len(i.Layers)-1].SquashedTree)
}

func (i *Image) resolveLink(ref file.Reference, t *tree.FileTree) (*file.Reference, error) {
	alreadySeen := file.NewFileReferenceSet()
	currentRef := &ref
	for {
		if alreadySeen.Contains(*currentRef) {
			return nil, fmt.Errorf("cycle during symlink resolution: %+v", currentRef)
		}

		entry, err := i.FileCatalog.Get(*currentRef)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve link metadata (%+v): %w", currentRef, err)
		}

		if entry.Metadata.TypeFlag != tar.TypeSymlink && entry.Metadata.TypeFlag != tar.TypeLink {
			// resolve the link to a file
			return currentRef, nil
		} else if entry.Metadata.Linkname == "" {
			// no resolution and there is no next link (pseudo dead link)... return what you found
			// any content fetches will fail, but that's ok
			return currentRef, nil
		}

		// prepare for the next iteration
		alreadySeen.Add(*currentRef)

		var nextPath string
		if strings.HasPrefix(entry.Metadata.Linkname, "/") {
			// use linked to absolute paths blindly
			nextPath = entry.Metadata.Linkname
		} else {
			var parentDir string
			switch entry.Metadata.TypeFlag {
			case tar.TypeSymlink:
				parentDir, _ = filepath.Split(string(currentRef.Path))
			case tar.TypeLink:
				parentDir = "/"
			default:
				return nil, fmt.Errorf("unknown link type: %+v", entry.Metadata.TypeFlag)
			}

			// assemble relative link path: normalize("/cur/dir/../file1.txt") = "/cur/file1.txt"
			nextPath = filepath.Clean(path.Join(parentDir, entry.Metadata.Linkname))
		}

		nextRef := t.File(file.Path(nextPath))

		// if there is no next path, return this reference (dead link)
		if nextRef == nil {
			return currentRef, nil
		}
		currentRef = nextRef
	}
}
