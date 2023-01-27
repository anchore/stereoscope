package file

import (
	"fmt"
)

var nextID = 0

// ID is used for file tree manipulation to uniquely identify tree nodes.
type ID uint64

type LinkResolution struct {
	AncestorResolution []ReferenceAccess
	LeafResolution     []ReferenceAccess
}

// ReferenceAccess represents the fetching of a file reference via a (possibly different) path.
type ReferenceAccess struct {
	RequestPath Path
	*Reference
}

// ReferenceVia represents a unique file, and how it was accessed, showing full symlink resolution.
type ReferenceVia struct {
	ReferenceAccess
	LinkResolution
}

// RequestPaths represents the traversal through the filesystem to access to current reference, including all symlink and hardlink resolution.
func (f *ReferenceVia) RequestPaths() []Path {
	//paths := []Path{f.RequestPath}
	var paths []Path
	for _, p := range f.LeafResolution {
		paths = append(paths, p.RequestPath)
	}
	return paths
}

// AccessReferences represents the traversal through the filesystem to access to current reference, including all symlink and hardlink resolution.
func (f *ReferenceVia) AccessReferences() []*Reference {
	var refs []*Reference
	for _, p := range f.LeafResolution {
		refs = append(refs, p.Reference)
	}
	//refs = append(refs, f.Reference)
	return refs
}

// RealPaths represents the traversal through the filesystem to access to current reference, including all symlink and hardlink resolution.
func (f *ReferenceVia) RealPaths() []Path {
	var refs []Path
	for _, p := range f.LeafResolution {
		if p.Reference != nil {
			refs = append(refs, p.Reference.RealPath)
		}
	}
	//if f.Reference != nil {
	//	refs = append(refs, f.Reference.RealPath)
	//}
	return refs
}

// Reference represents a unique file. This is useful when path is not good enough (i.e. you have the same file path for two files in two different container image layers, and you need to be able to distinguish them apart)
type Reference struct {
	id       ID
	RealPath Path // file path with NO symlinks or hardlinks in constituent paths
}

// NewFileReferenceVia shows how a reference was accessed.
func NewFileReferenceVia(path Path, ref *Reference, ancestors []ReferenceAccess, leafs []ReferenceAccess) *ReferenceVia {
	return &ReferenceVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: path,
			Reference:   ref,
		},
		LinkResolution: LinkResolution{
			AncestorResolution: ancestors,
			LeafResolution:     leafs,
		},
	}
}

// NewFileReference creates a new unique file reference for the given path.
func NewFileReference(path Path) *Reference {
	nextID++
	return &Reference{
		RealPath: path,
		id:       ID(nextID),
	}
}

// ID returns the unique ID for this file reference.
func (f *Reference) ID() ID {
	return f.id
}

// String returns a string representation of the path with a unique ID.
func (f *Reference) String() string {
	if f == nil {
		return "[nil]"
	}
	return fmt.Sprintf("[%v] real=%q", f.id, f.RealPath)
}
