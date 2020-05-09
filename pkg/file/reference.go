package file

import (
	"fmt"
)

var nextId = 0

type ID uint64

type Reference struct {
	Id   ID
	Path Path
}

func NewFileReference(path Path) Reference {
	nextId++
	return Reference{
		Path: path,
		Id:   ID(nextId),
	}
}

func (f *Reference) ID() ID {
	return f.Id
}

func (f *Reference) String() string {
	return fmt.Sprintf("[%v] %v", f.Id, f.Path)
}
