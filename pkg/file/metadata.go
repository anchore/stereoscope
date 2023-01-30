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
	// LinkDestination is populated only for hardlinks / symlinks, can be an absolute or relative
	LinkDestination string
	// Size of the file in bytes
	Size    int64
	UserID  int
	GroupID int
	// Type is the tar.Type entry for the file
	Type     Type
	IsDir    bool
	Mode     os.FileMode
	MIMEType string
}

func NewMetadata(header tar.Header, content io.Reader) Metadata {
	return Metadata{
		Path:            path.Clean(DirSeparator + header.Name),
		Type:            TypeFromTarType(header.Typeflag),
		LinkDestination: header.Linkname,
		Size:            header.FileInfo().Size(),
		Mode:            header.FileInfo().Mode(),
		UserID:          header.Uid,
		GroupID:         header.Gid,
		IsDir:           header.FileInfo().IsDir(),
		MIMEType:        MIMEType(content),
	}
}

// NewMetadataFromSquashFSFile populates Metadata for the entry at path, with details from f.
func NewMetadataFromSquashFSFile(path string, f *squashfs.File) (Metadata, error) {
	fi, err := f.Stat()
	if err != nil {
		return Metadata{}, err
	}

	var ty Type
	switch {
	case fi.IsDir():
		ty = TypeDir
	case f.IsRegular():
		ty = TypeReg
	case f.IsSymlink():
		ty = TypeSymlink
	default:
		switch fi.Mode() & os.ModeType {
		case os.ModeNamedPipe:
			ty = TypeFifo
		case os.ModeSocket:
			ty = TypeSocket
		case os.ModeDevice:
			ty = TypeBlockDevice
		case os.ModeCharDevice:
			ty = TypeCharacterDevice
		case os.ModeIrregular:
			ty = TypeIrregular
		}
		// note: cannot determine hardlink from squashfs.File (but case us not possible)
	}

	md := Metadata{
		Path:            filepath.Clean(filepath.Join("/", path)),
		LinkDestination: f.SymlinkPath(),
		Size:            fi.Size(),
		IsDir:           f.IsDir(),
		Mode:            fi.Mode(),
		Type:            ty,
	}

	if f.IsRegular() {
		md.MIMEType = MIMEType(f)
	}

	return md, nil
}
