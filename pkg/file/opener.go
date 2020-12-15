package file

import (
	"io"
	"os"
)

type Opener interface {
	Open() (io.ReadCloser, error)
}

type OpenerFn func() (io.ReadCloser, error)

// OpenerFromPath is an object that stores a Path to later be opened as a file.
type OpenerFromPath struct {
	Path string
}

// Open the stored Path as a io.ReadCloser.
func (o OpenerFromPath) Open() (io.ReadCloser, error) {
	return os.Open(o.Path)
}
