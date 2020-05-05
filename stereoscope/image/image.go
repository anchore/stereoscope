package image

import (
	"fmt"
	"github.com/anchore/stereoscope/stereoscope/file"
	"github.com/anchore/stereoscope/stereoscope/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"io"
)

type Image struct {
	Content   v1.Image
	Layers    []Layer
	Catalog   FileCatalog
	Structure *tree.FileTree
}

func NewImage(image v1.Image) *Image {
	return &Image{
		Content: image,
		Catalog: NewFileCatalog(),
	}
}

func (i *Image) GetFileReader(path file.Path) (io.ReadCloser, error) {
	fileNode := i.Structure.Node(path)
	if fileNode == nil {
		return nil, fmt.Errorf("could not find file path in squashed tree")
	}

	return i.Catalog.FileReader(fileNode)
}

func (i *Image) GetFileReaderFromLayer(path file.Path, layer *Layer) (io.ReadCloser, error) {
	fileNode := layer.Structure.Node(path)
	if fileNode == nil {
		return nil, fmt.Errorf("could not find file path in squashed tree")
	}

	return i.Catalog.FileReader(fileNode)
}

func (i *Image) Read() error {
	var unionTree = tree.NewUnionTree()
	var layers = make([]Layer, 0)

	v1Layers, err := i.Content.Layers()
	if err != nil {
		return err
	}

	for idx, v1Layer := range v1Layers {
		l, err := ReadLayer(uint(idx), v1Layer, &i.Catalog)
		if err != nil {
			return err
		}
		unionTree.PushTree(l.Structure)
		layers = append(layers, l)
	}

	// TODO: squashing should be an optional step
	squashedTree, err := unionTree.Squash()
	if err != nil {
		return err
	}

	i.Layers = layers
	i.Structure = squashedTree

	return nil
}
