package file

import (
	"os"
)

// OpenerFn is a function that can open a data source and provide a ReadSeekCloser for it.
type Opener interface {
	Open() (ReadSeekCloser, error)
}

// OpenerFromPath is an object that stores a Path to later be opened as a file.
type OpenerFromPath struct {
	Path string
}

// Open the stored Path as a ReadSeekCloser.
func (o OpenerFromPath) Open() (ReadSeekCloser, error) {
	return os.Open(o.Path)
}
