package image

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Image struct {
	// The image metadata and content source
	Content v1.Image
	// Ordered listing of Layers
	Layers []Layer
	// A {file.Reference -> (file.Metadata, Layer, Path)} mapping for all files in all layers
	FileCatalog FileCatalog
	// Squashed FileTree of file.Reference's
	SquashedTree *tree.FileTree
}

func NewImage(image v1.Image) *Image {
	return &Image{
		Content:     image,
		FileCatalog: NewFileCatalog(),
	}
}

func (i *Image) Read() error {
	var layers = make([]Layer, 0)

	v1Layers, err := i.Content.Layers()
	if err != nil {
		return err
	}

	for idx, v1Layer := range v1Layers {
		layer := NewLayer(uint(idx), v1Layer)
		err := layer.Read(&i.FileCatalog)
		if err != nil {
			return err
		}
		layers = append(layers, layer)
	}

	// TODO: side effects are bad, but we don't want getting an image to necessarily read from disk, yes?
	i.Layers = layers
	return nil
}

func (i *Image) Squash() error {
	var unionTree = tree.NewUnionTree()
	for _, layer := range i.Layers {
		unionTree.PushTree(layer.Tree)
	}
	squashedTree, err := unionTree.Squash()
	if err != nil {
		return err
	}
	// TODO: side effects are bad, but this optional info belongs to the Image obj.
	i.SquashedTree = squashedTree
	return nil
}

func (i *Image) FileContents(path file.Path) (string, error) {
	return fetchFileContents(i.SquashedTree, &i.FileCatalog, path)
}

func (i *Image) MultipleFileContents(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContents(i.SquashedTree, &i.FileCatalog, paths...)
}
