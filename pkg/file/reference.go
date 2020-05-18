package file

import (
	"fmt"
)

var nextID = 0

type ID uint64

type Reference struct {
	id   ID
	Path Path
}

func NewFileReference(path Path) Reference {
	nextID++
	return Reference{
		Path: path,
		id:   ID(nextID),
	}
}

func (f *Reference) ID() ID {
	return f.id
}

func (f *Reference) String() string {
	return fmt.Sprintf("[%v] %v", f.id, f.Path)
}
