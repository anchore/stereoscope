package filetree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

type FileNode struct {
	RealPath  file.Path // all constituent paths cannot have links (the base may be a link however)
	FileType  file.Type
	LinkPath  file.Path // a relative or absolute path to another file
	Reference *file.Reference
}

func newDir(p file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		RealPath:  p,
		FileType:  file.TypeDir,
		Reference: ref,
	}
}

func newFile(p file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		RealPath:  p,
		FileType:  file.TypeReg,
		Reference: ref,
	}
}

func newSymLink(p, linkPath file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		RealPath:  p,
		FileType:  file.TypeSymlink,
		LinkPath:  linkPath,
		Reference: ref,
	}
}

func newHardLink(p, linkPath file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		RealPath:  p,
		FileType:  file.TypeHardLink,
		LinkPath:  linkPath,
		Reference: ref,
	}
}

func (n *FileNode) ID() node.ID {
	return idByPath(n.RealPath)
}

func (n *FileNode) Copy() node.Node {
	return &FileNode{
		RealPath:  n.RealPath,
		FileType:  n.FileType,
		LinkPath:  n.RealPath,
		Reference: n.Reference,
	}
}

func (n *FileNode) isLink() bool {
	return n.FileType == file.TypeHardLink || n.FileType == file.TypeSymlink
}

func idByPath(p file.Path) node.ID {
	return node.ID(p.Normalize())
}
