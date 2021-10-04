package file

import (
	"errors"
	"os"
)

var _ ReadSeekAtCloser = (*LazyReader)(nil)

// LazyReader is a "lazy" read closer, allocating a file descriptor for the given path only upon the first Read() call.
type LazyReader struct {
	// path is the path to be opened
	path string
	// file is the io.ReadCloser source for the path
	file *os.File
}

// NewLazyReader creates a new LazyReader for the given path.
func NewLazyReader(path string) *LazyReader {
	return &LazyReader{
		path: path,
	}
}

func (d *LazyReader) checkOpen() error {
	if d.file == nil {
		var err error
		d.file, err = os.Open(d.path)
		if err != nil {
			return err
		}
	}
	return nil
}

// Read implements the io.Reader interface for the previously loaded path, opening the file upon the first invocation.
func (d *LazyReader) Read(b []byte) (n int, err error) {
	if err = d.checkOpen(); err != nil {
		return 0, err
	}
	return d.file.Read(b)
}

func (d *LazyReader) ReadAt(p []byte, off int64) (n int, err error) {
	if err = d.checkOpen(); err != nil {
		return 0, err
	}
	return d.file.ReadAt(p, off)
}

func (d *LazyReader) Seek(offset int64, whence int) (n int64, err error) {
	if err = d.checkOpen(); err != nil {
		return 0, err
	}
	return d.file.Seek(offset, whence)
}

// Close implements the io.Closer interface for the previously loaded path / opened file.
func (d *LazyReader) Close() error {
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
