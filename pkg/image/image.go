package image

import (
	"fmt"

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
	return i.FileCatalog.FileContents(ref)
}

func (i *Image) MultipleFileContentsByRef(refs ...file.Reference) (map[file.Reference]string, error) {
	return i.FileCatalog.MultipleFileContents(refs...)
}

func (i *Image) ResolveLinkByLayerSquash(ref file.Reference, layer int) (*file.Reference, error) {
	return resolveLink(ref, i.Layers[layer].SquashedTree, &i.FileCatalog)
}
func (i *Image) ResolveLinkByImageSquash(ref file.Reference) (*file.Reference, error) {
	return resolveLink(ref, i.Layers[len(i.Layers)-1].SquashedTree, &i.FileCatalog)
}
