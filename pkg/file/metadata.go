package file

import (
	"archive/tar"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/sylabs/squashfs"
)

// Metadata represents all file metadata of interest (used today for in-tar file resolution).
type Metadata struct {
	// Path is the absolute path representation to the file
	Path string
	// TarHeaderName is the exact entry name as found within a tar header
	TarHeaderName string
	// TarSequence is the nth header in the tar file this entry was found
	TarSequence int64
	// Linkname is populated only for hardlinks / symlinks, can be an absolute or relative
	Linkname string
	// Size of the file in bytes
	Size    int64
	UserID  int
	GroupID int
	// TypeFlag is the tar.TypeFlag entry for the file
	TypeFlag byte
	IsDir    bool
	Mode     os.FileMode
	MIMEType string
}

func NewMetadata(header tar.Header, sequence int64, content io.Reader) Metadata {
	return Metadata{
		Path:          path.Clean(DirSeparator + header.Name),
		TarHeaderName: header.Name,
		TarSequence:   sequence,
		TypeFlag:      header.Typeflag,
		Linkname:      header.Linkname,
		Size:          header.FileInfo().Size(),
		Mode:          header.FileInfo().Mode(),
		UserID:        header.Uid,
		GroupID:       header.Gid,
		IsDir:         header.FileInfo().IsDir(),
		MIMEType:      MIMEType(content),
	}
}

// NewMetadataFromSquashFSFile populates Metadata for the entry at path, with details from f.
func NewMetadataFromSquashFSFile(path string, f *squashfs.File) (Metadata, error) {
	fi, err := f.Stat()
	if err != nil {
		return Metadata{}, err
	}

	md := Metadata{
		Path:     filepath.Clean(filepath.Join("/", path)),
		Linkname: f.SymlinkPath(),
		Size:     fi.Size(),
		IsDir:    f.IsDir(),
		Mode:     fi.Mode(),
	}

	if f.IsRegular() {
		md.MIMEType = MIMEType(f)
	}

	return md, nil
}
