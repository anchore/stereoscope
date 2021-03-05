package file

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/pkg/errors"
)

const perFileReadLimit = 2 * GB

var ErrTarStopIteration = fmt.Errorf("halt iterating tar")

// tarFile is a ReadCloser of a tar file on disk.
type tarFile struct {
	io.Reader
	io.Closer
}

// TarVisitor is a visitor function meant to be used in conjunction with the TarIterator.
type TarVisitor func(TarFileEntry) error

// TarContentsRequest is a map of tarHeaderNames -> file.References to aid in simplifying content retrieval.
type TarContentsRequest map[string]Reference

// ErrFileNotFound returned from ReaderFromTar if a file is not found in the given archive.
type ErrFileNotFound struct {
	Path string
}

func (e *ErrFileNotFound) Error() string {
	return fmt.Sprintf("file not found (path=%s)", e.Path)
}

// TarIterator is a function that reads across a tar and invokes a visitor function for each entry discovered. The iterator
// stops when there are no more entries to read, if there is an error in the underlying reader or visitor function,
// or if the visitor function returns a ErrTarStopIteration sentinel error.
func TarIterator(reader io.Reader, visitor TarVisitor) error {
	tarReader := tar.NewReader(reader)

	for {
		hdr, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if err := visitor(TarFileEntry{Header: hdr, Reader: tarReader}); err != nil {
			if errors.Is(err, ErrTarStopIteration) {
				return nil
			}
			return fmt.Errorf("failed to visit tar entry=%q : %w", hdr.Name, err)
		}
	}
	return nil
}

// ReaderFromTar returns a io.ReadCloser for the Path within a tar file.
func ReaderFromTar(reader io.ReadCloser, tarPath string) (io.ReadCloser, error) {
	var result io.ReadCloser

	visitor := func(entry TarFileEntry) error {
		if entry.Header.Name == tarPath {
			result = &tarFile{
				Reader: entry.Reader,
				Closer: reader,
			}
			return ErrTarStopIteration
		}
		return nil
	}
	if err := TarIterator(reader, visitor); err != nil {
		return nil, err
	}

	if result == nil {
		return nil, &ErrFileNotFound{tarPath}
	}

	return result, nil
}

// MetadataFromTar returns the tar metadata from the header info.
func MetadataFromTar(reader io.ReadCloser, tarPath string) (Metadata, error) {
	var metadata *Metadata
	visitor := func(entry TarFileEntry) error {
		if entry.Header.Name == tarPath {
			m := NewMetadata(entry.Header)
			metadata = &m
			return ErrTarStopIteration
		}
		return nil
	}
	if err := TarIterator(reader, visitor); err != nil {
		return Metadata{}, err
	}
	if metadata == nil {
		return Metadata{}, &ErrFileNotFound{tarPath}
	}
	return *metadata, nil
}

// TODO: delete but keep tests
// EnumerateFileMetadataFromTar populates and returns a Metadata object for all files in the tar.
func EnumerateFileMetadataFromTar(reader io.Reader) <-chan Metadata {
	result := make(chan Metadata)
	go func() {
		visitor := func(entry TarFileEntry) error {
			// always ensure relative Path notations are not parsed as part of the filename
			name := path.Clean(DirSeparator + entry.Header.Name)
			if name == "." {
				return nil
			}

			switch entry.Header.Typeflag {
			case tar.TypeXGlobalHeader:
				log.Errorf("unexpected tar file: (XGlobalHeader): type=%v name=%s", entry.Header.Typeflag, name)
			case tar.TypeXHeader:
				log.Errorf("unexpected tar file (XHeader): type=%v name=%s", entry.Header.Typeflag, name)
			default:
				result <- NewMetadata(entry.Header)
			}
			return nil
		}

		if err := TarIterator(reader, visitor); err != nil {
			log.Errorf("failed to extract metadata from tar: %w", err)
		}

		close(result)
	}()
	return result
}

// UntarToDirectory writes the contents of the given tar reader to the given destination
func UntarToDirectory(reader io.Reader, dst string) error {
	tr := tar.NewReader(reader)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case header == nil:
			continue
		}

		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// limit the reader on each file read to prevent decompression bomb attacks
			numBytes, err := io.Copy(f, io.LimitReader(tr, perFileReadLimit))
			if numBytes >= perFileReadLimit || errors.Is(err, io.EOF) {
				return fmt.Errorf("zip read limit hit (potential decompression bomb attack)")
			}
			if err != nil {
				return fmt.Errorf("unable to copy file: %w", err)
			}

			if err = f.Close(); err != nil {
				log.Errorf("failed to close file during untar of path=%q: %w", f.Name(), err)
			}
		}
	}
}
