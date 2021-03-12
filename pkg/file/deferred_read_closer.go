package file

import (
	"errors"
	"io"
	"os"
)

var _ io.ReadCloser = (*DeferredReadCloser)(nil)

// DeferredReadCloser is a "lazy" read closer, allocating a file descriptor for the given path only upon the first Read() call.
type DeferredReadCloser struct {
	// path is the path to be opened
	path string
	// file is the io.ReadCloser source for the path
	file *os.File
}

// NewDeferredReadCloser creates a new DeferredReadCloser for the given path.
func NewDeferredReadCloser(path string) *DeferredReadCloser {
	return &DeferredReadCloser{
		path: path,
	}
}

// Read implements the io.Reader interface for the previously loaded path, opening the file upon the first invocation.
func (d *DeferredReadCloser) Read(b []byte) (n int, err error) {
	if d.file == nil {
		var err error
		d.file, err = os.Open(d.path)
		if err != nil {
			return 0, err
		}
	}
	return d.file.Read(b)
}

// Close implements the io.Closer interface for the previously loaded path / opened file.
func (d *DeferredReadCloser) Close() error {
	if d.file == nil {
		return nil
	}

	err := d.file.Close()
	if err != nil && errors.Is(err, os.ErrClosed) {
		err = nil
	}
	d.file = nil
	return err
}
