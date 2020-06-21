package image

import (
	"fmt"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"
)

// Image represents a container image.
type Image struct {
	// content is the image metadata and content provider
	content v1.Image
	// Metadata contains select image attributes
	Metadata Metadata
	// Layers contains the rich layer objects in build order
	Layers []*Layer
	// FileCatalog contains all file metadata for all files in all layers
	FileCatalog FileCatalog
}

// NewImage provides a new, unread image object.
func NewImage(image v1.Image) *Image {
	return &Image{
		content:     image,
		FileCatalog: NewFileCatalog(),
	}
}

// NewImageWithTags provides a new, unread image object, represented by a set of named image tags.
func NewImageWithTags(image v1.Image, tags []name.Tag) *Image {
	return &Image{
		content:     image,
		FileCatalog: NewFileCatalog(),
		Metadata: Metadata{
			Tags: tags,
		},
	}
}

func (i *Image) IDs() []string {
	var ids = make([]string, len(i.Metadata.Tags))
	for idx, t := range i.Metadata.Tags {
		ids[idx] = t.String()
	}
	ids = append(ids, i.Metadata.Digest)
	return ids
}

func (i *Image) trackReadProgress(metadata Metadata) *progress.Manual {
	prog := &progress.Manual{
		// x2 for read and squash of each layer
		Total: int64(len(metadata.Config.RootFS.DiffIDs) * 2),
	}

	bus.Publish(partybus.Event{
		Type:   event.ReadImage,
		Source: metadata,
		Value:  progress.Progressable(prog),
	})

	return prog
}

// Read parses information from the underlaying image tar into this struct. This includes image metadata, layer
// metadata, layer file trees, and layer squash trees (which implies the image squash tree).
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

	// let consumers know of a monitorable event (image save + copy stages)
	readProg := i.trackReadProgress(metadata)

	for idx, v1Layer := range v1Layers {
		layer := NewLayer(v1Layer)
		err := layer.Read(&i.FileCatalog, i.Metadata, idx)
		if err != nil {
			return err
		}
		i.Metadata.Size += layer.Metadata.Size
		layers = append(layers, layer)

		readProg.N++
	}

	i.Layers = layers

	// in order to resolve symlinks all squashed trees must be available
	return i.squash(readProg)
}

// squash generates a squash tree for each layer in the image. For instance, layer 2 squash =
// squash(layer 0, layer 1, layer 2), layer 3 squash = squash(layer 0, layer 1, layer 2, layer 3), and so on.
func (i *Image) squash(prog *progress.Manual) error {
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

		prog.N++
	}

	prog.SetCompleted()

	return nil
}

// SquashTree returns the pre-computed image squash file tree.
func (i *Image) SquashedTree() *tree.FileTree {
	return i.Layers[len(i.Layers)-1].SquashedTree
}

// FileContentsFromSquash fetches file contents for a single path, relative to the image squash tree.
// If the path does not exist an error is returned.
func (i *Image) FileContentsFromSquash(path file.Path) (string, error) {
	return fetchFileContentsByPath(i.SquashedTree(), &i.FileCatalog, path)
}

// MultipleFileContentsFromSquash fetches file contents for all given paths, relative to the image squash tree.
// If any one path does not exist an error is returned for the entire request.
func (i *Image) MultipleFileContentsFromSquash(paths ...file.Path) (map[file.Reference]string, error) {
	return fetchMultipleFileContentsByPath(i.SquashedTree(), &i.FileCatalog, paths...)
}

// FileContentsByRef fetches file contents for a single file reference, irregardless of the source layer.
// If the path does not exist an error is returned.
// This is a convenience function provided by the FileCatalog.
func (i *Image) FileContentsByRef(ref file.Reference) (string, error) {
	return i.FileCatalog.FileContents(ref)
}

// FileContentsByRef fetches file contents for all file references given, irregardless of the source layer.
// If any one path does not exist an error is returned for the entire request.
func (i *Image) MultipleFileContentsByRef(refs ...file.Reference) (map[file.Reference]string, error) {
	return i.FileCatalog.MultipleFileContents(refs...)
}

// ResolveLinkByLayerSquash resolves a symlink or hardlink for the given file reference relative to the result from
// the layer squash of the given layer index argument.
// If the given file reference is not a link type, or is a unresolvable (dead) link, then the given file reference is returned.
func (i *Image) ResolveLinkByLayerSquash(ref file.Reference, layer int) (*file.Reference, error) {
	return resolveLink(ref, i.Layers[layer].SquashedTree, &i.FileCatalog)
}

// ResolveLinkByLayerSquash resolves a symlink or hardlink for the given file reference relative to the result from the image squash.
// If the given file reference is not a link type, or is a unresolvable (dead) link, then the given file reference is returned.
func (i *Image) ResolveLinkByImageSquash(ref file.Reference) (*file.Reference, error) {
	return resolveLink(ref, i.Layers[len(i.Layers)-1].SquashedTree, &i.FileCatalog)
}
