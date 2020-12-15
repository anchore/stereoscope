package file

import (
	"fmt"
)

var nextID = 0

// ID is used for file tree manipulation to uniquely identify tree nodes.
type ID uint64

// Reference represents a unique file. This is useful when Path is not good enough (i.e. you have the same file Path for two files in two different container image layers, and you need to be able to distinguish them apart)
type Reference struct {
	id   ID
	Path Path
}

// NewFileReference creates a new unique file reference for the given Path.
func NewFileReference(path Path) Reference {
	nextID++
	return Reference{
		Path: path,
		id:   ID(nextID),
	}
}

// ID returns the unique ID for this file reference.
func (f *Reference) ID() ID {
	return f.id
}

// String returns a string representation of the Path with a unique ID.
func (f *Reference) String() string {
	return fmt.Sprintf("[%v] %v", f.id, f.Path)
}
