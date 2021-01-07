package file

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
)

const tarBlockSize = 512

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

type TarFileEntry struct {
	Header *tar.Header
	Reader io.Reader
}

type tarIndexEntry struct {
	headerPosition int64
	bodyPosition   int64
	size           int64
}

type IndexedTarReader struct {
	reader    ReadSeekCloser
	tarReader *tar.Reader
	index     map[string][]tarIndexEntry
	onIndex   []TarVisitor
}

func NewIndexedTarReader(reader ReadSeekCloser, onIndex ...TarVisitor) (*IndexedTarReader, error) {
	t := &IndexedTarReader{
		reader:    reader,
		tarReader: tar.NewReader(reader),
		index:     make(map[string][]tarIndexEntry),
		onIndex:   onIndex,
	}
	return t, t.indexEntries()
}

func (t *IndexedTarReader) indexEntries() error {
	var lastBodyPosition int64
	for {
		f, err := t.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to read tar entry: %v", err)
		}

		// index the file at this position
		if f.Header == nil {
			return fmt.Errorf("found an empty tar header")
		}

		bodyPosition, err := t.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read body position in tar during index: %v", err)
		}

		entry := tarIndexEntry{
			headerPosition: lastBodyPosition,
			bodyPosition:   bodyPosition,
			size:           f.Header.Size,
		}

		t.index[f.Header.Name] = append(t.index[f.Header.Name], entry)

		// run though the visitors
		for _, visitor := range t.onIndex {
			if err := visitor(TarFileEntry{
				Header: f.Header,
				Reader: nil, // we can't allow visitors to read the contents
			}); err != nil {
				return fmt.Errorf("failed visitor on tar index: %w", err)
			}
		}

		// get to the start of the next header
		if _, err = io.Copy(ioutil.Discard, t.tarReader); err != nil {
			return err
		}
		lastBodyPosition, err = t.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read after body position in tar during index: %v", err)
		}

		remaining := lastBodyPosition % tarBlockSize

		if remaining > 0 {
			// we don't need to read this, the tar lib will do this for us. We need to just find the start of the next header.
			lastBodyPosition += tarBlockSize - remaining
		}
	}

	return t.reset()
}

func (t *IndexedTarReader) reset() error {
	// reset the reader for general use
	_, err := t.reader.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	t.tarReader = tar.NewReader(t.reader)
	return nil
}

func (t *IndexedTarReader) next() (TarFileEntry, error) {
	header, err := t.tarReader.Next()
	if errors.Is(err, io.EOF) {
		return TarFileEntry{}, err
	}
	if err != nil {
		return TarFileEntry{}, fmt.Errorf("could not get next header entry: %w", err)
	}

	file := TarFileEntry{
		Header: header,
		Reader: t.tarReader,
	}

	return file, nil
}

func (t *IndexedTarReader) readEntryAt(target int64) (TarFileEntry, error) {
	current, err := t.reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return TarFileEntry{}, fmt.Errorf("unable to read current position in tar: %v", err)
	}
	if final, err := t.reader.Seek(target-current, io.SeekCurrent); err != nil {
		return TarFileEntry{}, fmt.Errorf("unable to reach target position in tar: %v", err)
	} else if final != target {
		return TarFileEntry{}, fmt.Errorf("seek failed to arrive at target=%d position=%d", target, final)
	}

	// reset the internal state of the reader, but we don't necessarily need to reset the
	t.tarReader = tar.NewReader(t.reader)
	return t.next()
}

func (t *IndexedTarReader) Entries(name string) ([]TarFileEntry, error) {
	if indexes, exists := t.index[name]; exists {
		entries := make([]TarFileEntry, len(indexes))
		for i, index := range indexes {
			entry, err := t.readEntryAt(index.headerPosition)
			if err != nil {
				return nil, err
			}
			entries[i] = entry
		}
		return entries, nil
	}
	return nil, nil
}

func (t *IndexedTarReader) Entry(name string) (*TarFileEntry, error) {
	if indexes, exists := t.index[name]; exists {
		if len(indexes) >= 1 {
			entry, err := t.readEntryAt(indexes[0].headerPosition)
			if err != nil {
				return nil, fmt.Errorf("could not fetch entry for name=%q : %w", name, err)
			}
			return &entry, nil
		}
	}
	return nil, nil
}
