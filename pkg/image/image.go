package image

import (
	"fmt"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type Image struct {
	// The image metadata and content source
	Content      v1.Image
	// Ordered listing of Layers
	Layers       []Layer
	// A {file.Reference -> (file.Metadata, Layer, Path)} mapping for all files in all layers
	FileCatalog  FileCatalog
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
	var unionTree = tree.NewUnionTree()
	var layers = make([]Layer, 0)

	v1Layers, err := i.Content.Layers()
	if err != nil {
		return err
	}

	for idx, v1Layer := range v1Layers {
		l, err := ReadLayer(uint(idx), v1Layer, &i.FileCatalog)
		if err != nil {
			return err
		}
		unionTree.PushTree(l.Tree)
		layers = append(layers, l)
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


func (i *Image) SquashedFileContents(path file.Path) (string, error) {
	if i.SquashedTree == nil {
		return "", fmt.Errorf("no squash tree found")
	}

	fileReference := i.SquashedTree.File(path)
	if fileReference == nil {
		return "", fmt.Errorf("could not find file path in squashed Tree")
	}

	content, err := i.FileCatalog.FileContent(*fileReference)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (i *Image) LayerFileContents(path file.Path, layer *Layer) (string, error) {
	fileReference := layer.Tree.File(path)
	if fileReference == nil {
		return "", fmt.Errorf("could not find file path in layer Tree")
	}

	content, err := i.FileCatalog.FileContent(*fileReference)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
