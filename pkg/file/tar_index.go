package file

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
)

type tarIndexEntry struct {
	header          tar.Header
	sequence        int64
	payloadLocation int64
}

// TarIndex is a tar reader capable of O(1) fetching of entry contents after the first read.
type TarIndex struct {
	filePath    string
	indexByName map[string][]tarIndexEntry
}

// NewTarIndex creates a new TarIndex that is already indexed.
func NewTarIndex(tarFilePath string, onIndex ...TarVisitor) (*TarIndex, error) {
	t := &TarIndex{
		filePath:    tarFilePath,
		indexByName: make(map[string][]tarIndexEntry),
	}
	fh, err := os.Open(tarFilePath)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return t, t.indexEntries(fh, onIndex...)
}

// indexEntries records all tar header locations indexed by header names.
func (t *TarIndex) indexEntries(file *os.File, onIndex ...TarVisitor) error {
	visitor := func(entry TarFileEntry) error {
		payloadLocation, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read current position in tar: %v", err)
		}

		// keep track of the header position for this entry; the current file position is where the entry
		// body payload starts (after the header has been read).
		index := tarIndexEntry{
			sequence:        entry.Sequence,
			payloadLocation: payloadLocation,
			header:          entry.Header,
		}
		t.indexByName[entry.Header.Name] = append(t.indexByName[entry.Header.Name], index)

		// run though the visitors
		for _, visitor := range onIndex {
			if err := visitor(index.toTarFileEntry(file.Name())); err != nil {
				return fmt.Errorf("failed visitor on tar index: %w", err)
			}
		}

		return nil
	}

	return TarIterator(file, visitor)
}

// EntriesByName fetches all TarFileEntries for the given tar header name.
func (t *TarIndex) EntriesByName(name string) ([]TarFileEntry, error) {
	if indexes, exists := t.indexByName[name]; exists {
		entries := make([]TarFileEntry, len(indexes))
		for i, index := range indexes {
			entries[i] = index.toTarFileEntry(t.filePath)
		}
		return entries, nil
	}
	return nil, nil
}

func (t *tarIndexEntry) toTarFileEntry(tarFilePath string) TarFileEntry {
	return TarFileEntry{
		Header: t.header,
		Reader: newDeferredPartialReadCloser(tarFilePath, t.payloadLocation, t.header.Size),
	}
}
