package file

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/sylabs/squashfs"

	"github.com/anchore/stereoscope/internal/log"
)

// Metadata represents all file metadata of interest.
type Metadata struct {
	fs.FileInfo

	// Path is the absolute path representation to the file
	Path string
	// LinkDestination is populated only for hardlinks / symlinks, can be an absolute or relative
	LinkDestination string
	UserID          int
	GroupID         int
	Type            Type
	MIMEType        string
}

func NewMetadata(header tar.Header, content io.Reader) Metadata {
	return Metadata{
		FileInfo:        header.FileInfo(),
		Path:            path.Clean(DirSeparator + header.Name),
		Type:            TypeFromTarType(header.Typeflag),
		LinkDestination: header.Linkname,
		UserID:          header.Uid,
		GroupID:         header.Gid,
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
		ty = TypeDirectory
	case f.IsRegular():
		ty = TypeRegular
	case f.IsSymlink():
		ty = TypeSymLink
	default:
		switch fi.Mode() & os.ModeType {
		case os.ModeNamedPipe:
			ty = TypeFIFO
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
		FileInfo:        fi,
		Path:            filepath.Clean(filepath.Join("/", path)),
		LinkDestination: f.SymlinkPath(),
		UserID:          -1,
		GroupID:         -1,
		Type:            ty,
	}

	if f.IsRegular() {
		md.MIMEType = MIMEType(f)
	}

	return md, nil
}

func NewMetadataFromPath(path string, info os.FileInfo) Metadata {
	var mimeType string
	uid, gid := getXid(info)

	ty := TypeFromMode(info.Mode())

	if ty == TypeRegular {
		f, err := os.Open(path)
		if err != nil {
			// TODO: it may be that the file is inaccessible, however, this is not an error or a warning. In the future we need to track these as known-unknowns
			f = nil
		} else {
			defer func() {
				if err := f.Close(); err != nil {
					log.Warnf("unable to close file while obtaining metadata: %s", path)
				}
			}()
		}

		mimeType = MIMEType(f)
	}

	return Metadata{
		FileInfo: info,
		Path:     path,
		Type:     ty,
		// unsupported across platforms
		UserID:   uid,
		GroupID:  gid,
		MIMEType: mimeType,
	}
}
