package file

import (
	"fmt"
	"io"
	"os"
)

type TarIndexVisitor func(TarIndexEntry) error

// TarIndex is a tar reader capable of O(1) fetching of entry contents after the first read.
//
// The tar file stays open for the life of the index and every entry reads through that one
// descriptor with positional reads (pread), so opening an entry costs no syscalls. Previously each
// Open() re-opened the tar, which on an image with tens of thousands of files - each opened by
// the indexer's MIME sniff, again by syft's digest cataloger, again by every other consumer -
// added up to hundreds of thousands of open/close pairs that serialized on the file's vnode.
type TarIndex struct {
	file        *os.File
	indexByName map[string][]TarIndexEntry
}

// NewTarIndex creates a new TarIndex that is already indexed. Call Close when the index is no
// longer needed to release the tar file.
func NewTarIndex(tarFilePath string, onIndex TarIndexVisitor) (*TarIndex, error) {
	tarFileHandle, err := os.Open(tarFilePath)
	if err != nil {
		return nil, err
	}
	t := &TarIndex{
		file:        tarFileHandle,
		indexByName: make(map[string][]TarIndexEntry),
	}

	visitor := func(entry TarFileEntry) error {
		// keep track of the current location (just after reading the tar header) as this is the file content for the
		// current entry being processed.
		entrySeekPosition, err := tarFileHandle.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read current position in tar: %v", err)
		}

		// keep track of the header position for this entry; the current tarFileHandle position is where the entry
		// body payload starts (after the header has been read).
		indexEntry := TarIndexEntry{
			index:        t,
			sequence:     entry.Sequence,
			header:       entry.Header,
			seekPosition: entrySeekPosition,
		}
		t.indexByName[entry.Header.Name] = append(t.indexByName[entry.Header.Name], indexEntry)

		// run though the visitors
		if onIndex != nil {
			if err := onIndex(indexEntry); err != nil {
				return fmt.Errorf("failed visitor on tar indexEntry: %w", err)
			}
		}

		return nil
	}

	if err := IterateTar(tarFileHandle, visitor); err != nil {
		_ = tarFileHandle.Close()
		return nil, err
	}
	return t, nil
}

// Close releases the tar file. Entries opened afterwards fail to read.
func (t *TarIndex) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	err := t.file.Close()
	t.file = nil
	return err
}

// EntriesByName fetches all TarFileEntries for the given tar header name.
func (t *TarIndex) EntriesByName(name string) ([]TarFileEntry, error) {
	if indexes, exists := t.indexByName[name]; exists {
		entries := make([]TarFileEntry, len(indexes))
		for i, index := range indexes {
			entries[i] = index.ToTarFileEntry()
		}
		return entries, nil
	}
	return nil, nil
}
