package file

import "io"

type ReadSeekAtCloser interface {
	io.Reader
	io.Seeker
	io.ReaderAt
	io.Closer
}
