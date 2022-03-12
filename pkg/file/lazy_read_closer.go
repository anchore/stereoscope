package file

import (
	"errors"
	"io"
	"os"
)

var _ io.ReadCloser = (*LazyReadCloser)(nil)
var _ LazyReader = (*LazyReadCloser)(nil)

// LazyReader is a single interface with all reading functions we want to use
// across clients. The transition from io.ReadCloser should happen gradually.
type LazyReader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

// LazyReadCloser is a "lazy" read closer, allocating a file descriptor for the given path only upon the first Read() call.
type LazyReadCloser struct {
	// path is the path to be opened
	path string
	// file is the io.ReadCloser source for the path
	file *os.File
}

// NewLazyReadCloser creates a new LazyReadCloser for the given path.
func NewLazyReadCloser(path string) *LazyReadCloser {
	return &LazyReadCloser{
		path: path,
	}
}

// Read implements the io.Reader interface for the previously loaded path, opening the file upon the first invocation.
func (d *LazyReadCloser) Read(b []byte) (n int, err error) {
	//if d.file == nil {
	//	var err error
	//	d.file, err = os.Open(d.path)
	//	if err != nil {
	//		return 0, err
	//	}
	//}
	if err := d.isFileOpen(); err != nil {
		return 0, err
	}
	return d.file.Read(b)
}

// Close implements the io.Closer interface for the previously loaded path / opened file.
func (d *LazyReadCloser) Close() error {
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

func (d *LazyReadCloser) Seek(offset int64, whence int) (int64, error) {
	if err := d.isFileOpen(); err != nil {
		return 0, err
	}

	return d.file.Seek(offset, whence)
}

func (d *LazyReadCloser) ReadAt(p []byte, off int64) (n int, err error) {
	if err := d.isFileOpen(); err != nil {
		return 0, err
	}

	return d.file.ReadAt(p, off)
}

func (d *LazyReadCloser) isFileOpen() error {
	if d.file != nil {
		return nil
	}

	var err error
	d.file, err = os.Open(d.path)
	return err
}
