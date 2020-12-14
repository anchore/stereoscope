package file

import (
	"io"
	"os"
)

var _ io.ReadCloser = (*DeferredReadCloser)(nil)

type DeferredReadCloser struct {
	path string
	file *os.File
}

func NewDeferredReadCloser(path string) *DeferredReadCloser {
	return &DeferredReadCloser{
		path: path,
	}
}

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

func (d *DeferredReadCloser) Close() error {
	if d.file == nil {
		return nil
	}
	return d.file.Close()
}
