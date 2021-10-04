package file

import (
	"archive/tar"
)

type TarIndexEntry struct {
	path         string
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

func (t *TarIndexEntry) Open() ReadSeekAtCloser {
	return newLazyBoundedReadSeekAtCloser(t.path, t.seekPosition, t.header.Size)
}
