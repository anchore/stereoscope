package image

import (
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
	Layers []Layer
	// A {file.Reference -> (file.Metadata, Layer, Path)} mapping for all files in all layers
	FileCatalog FileCatalog
	// Squashed FileTree of file.Reference's
	SquashedTree *tree.FileTree
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
	var layers = make([]Layer, 0)

	metadata, err := readImageMetadata(i.content)
	if err != nil {
		return err
	}
	if i.Metadata.Tags != nil {
		metadata.Tags = i.Metadata.Tags
	}
	i.Metadata = metadata

	log.Debugf("image metadata:\n\tdigest=%+v\n\tmediaType=%+v\n\ttags=%+v",
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

func (i *Image) FileContentsFromSquash(path file.Path) (string, error) {
	return fetchFileContentsByPath(i.SquashedTree, &i.FileCatalog, path)
}

func (i *Image) MultipleFileContentsFromSquash(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(i.SquashedTree, &i.FileCatalog, paths...)
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
