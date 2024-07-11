package filetree

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/file"
)

// Builder is a helper for building a filetree and accompanying index in a coordinated fashion.
type Builder struct {
	tree  Writer
	index IndexWriter
}

func NewBuilder(tree Writer, index IndexWriter) *Builder {
	return &Builder{
		tree:  tree,
		index: index,
	}
}

func (b *Builder) Add(metadata file.Metadata) (*file.Reference, error) {
	var (
		ref *file.Reference
		err error
	)
	switch metadata.Type {
	case file.TypeSymLink:
		ref, err = b.tree.AddSymLink(file.Path(metadata.RealPath), file.Path(metadata.LinkDestination))
		if err != nil {
			return nil, err
		}
	case file.TypeHardLink:
		ref, err = b.tree.AddHardLink(file.Path(metadata.RealPath), file.Path(metadata.LinkDestination))
		if err != nil {
			return nil, err
		}
	case file.TypeDirectory:
		ref, err = b.tree.AddDir(file.Path(metadata.RealPath))
		if err != nil {
			return nil, err
		}
	default:
		ref, err = b.tree.AddFile(file.Path(metadata.RealPath))
		if err != nil {
			return nil, err
		}
	}
	if ref == nil {
		return nil, fmt.Errorf("could not add path=%q link=%q during tar iteration", metadata.RealPath, metadata.LinkDestination)
	}

	b.index.Add(*ref, metadata)

	return ref, nil
}
