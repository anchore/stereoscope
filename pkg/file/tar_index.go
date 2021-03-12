package file

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

const tarBlockSize = 512

type tarIndexEntry struct {
	header          tar.Header
	sequence        int64
	headerLocation  int64
	payloadLocation int64
}

// TarIndex is a tar reader capable of O(1) fetching of entry contents.
type TarIndex struct {
	file        *os.File
	indexByName map[string][]tarIndexEntry
	onIndex     []TarVisitor
}

// NewTarIndex creates a new TarIndex that is already indexed.
func NewTarIndex(file *os.File, onIndex ...TarVisitor) (*TarIndex, error) {
	t := &TarIndex{
		file:        file,
		indexByName: make(map[string][]tarIndexEntry),
		onIndex:     onIndex,
	}
	return t, t.indexEntries()
}

// indexEntries records all tar header locations indexed by header names. Note: since access to the underlying file
// for direct seeking is required, and content is dispatched in a deferred manner based on the path on disk to the tar,
// the TarIterator cannot be used here.
func (t *TarIndex) indexEntries() error {
	var targetHeaderPosition, sequence int64
	tarReader := tar.NewReader(t.file)
	for {
		// calling next will read and parse the next header, leaving the current seek position to be at the beginning
		// of the entry (file) contents.
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to read tar entry: %v", err)
		}

		if header == nil {
			return fmt.Errorf("found an empty tar header")
		}

		payloadLocation, err := t.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read current position in tar: %v", err)
		}

		// keep track of the header position for this entry. Note: the current file position is where the entry
		// body payload starts (after the header has been read), so we need to use information from the last entry
		// iteration to derive where the start of the current header is.
		index := tarIndexEntry{
			sequence:        sequence,
			headerLocation:  targetHeaderPosition,
			payloadLocation: payloadLocation,
			header:          *header,
		}
		t.indexByName[header.Name] = append(t.indexByName[header.Name], index)

		// run though the visitors
		for _, visitor := range t.onIndex {
			if err := visitor(TarFileEntry{
				Sequence: sequence,
				Header:   *header,
				Reader:   nil, // we can't allow visitors to read the contents while indexing
			}); err != nil {
				return fmt.Errorf("failed visitor on tar index: %w", err)
			}
		}

		// get to the start of the next header. Note that there may be padding between the end of the current entry
		// contents and the next header. We should set the target position based on the current position and adjust
		// if the contents do not fit evenly within a standard tar block size.
		if _, err = io.Copy(ioutil.Discard, tarReader); err != nil {
			return err
		}

		// first assume the next header starts right after the current entry's body payload to start... soon we'll verify
		// if we need to account for block padding.
		targetHeaderPosition, err = t.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("unable to read after body position in tar during index: %v", err)
		}

		remaining := targetHeaderPosition % tarBlockSize
		if remaining > 0 {
			// there is block padding, so the next header does not immediately follow the current entry's body payload.
			// Note: We don't need to actually read the padding bytes since the tar lib will do this for us. We only
			// need to find when the padding will end (aka, where the start of the next header is).
			targetHeaderPosition += tarBlockSize - remaining
		}
		sequence++
	}

	// reset the internal state
	_, err := t.file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

// EntriesByName fetches all TarFileEntries for the given tar header name.
func (t *TarIndex) EntriesByName(name string) ([]TarFileEntry, error) {
	if indexes, exists := t.indexByName[name]; exists {
		entries := make([]TarFileEntry, len(indexes))
		for i, index := range indexes {
			entries[i] = index.toTarFileEntry(t.file.Name())
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
