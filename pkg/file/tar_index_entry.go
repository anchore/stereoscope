package file

import (
	"archive/tar"
	"io"
)

type TarIndexEntry struct {
	index        *TarIndex
	sequence     int64
	header       tar.Header
	seekPosition int64
}

func (t *TarIndexEntry) ToTarFileEntry() TarFileEntry {
	return TarFileEntry{
		Sequence: t.sequence,
		Header:   t.header,
		Reader:   t.Open(),
	}
}

// Open returns a reader over this entry's contents. The reader is a positional view onto the
// index's shared tar file: it supports Read, Seek and ReadAt, is safe to use concurrently with
// other entries' readers, and its Close releases nothing (the index owns the descriptor).
func (t *TarIndexEntry) Open() io.ReadCloser {
	return &sectionReadCloser{SectionReader: io.NewSectionReader(t.index.file, t.seekPosition, t.header.Size)}
}

var _ interface {
	io.ReadCloser
	io.ReaderAt
	io.Seeker
} = (*sectionReadCloser)(nil)

// sectionReadCloser is an io.SectionReader that satisfies io.ReadCloser for callers that close
// what they open.
type sectionReadCloser struct {
	*io.SectionReader
}

func (s *sectionReadCloser) Close() error { return nil }
