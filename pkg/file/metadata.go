package file

import (
	"archive/tar"
	"os"
	"path"
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
	Size int64
	// UserID is the numeric UID of the file
	UserID int
	// GroupID is the numeric GID of the file
	GroupID int
	// TypeFlag is the tar.TypeFlag entry for the file
	TypeFlag byte
	// Mode is the mode bits of the file
	Mode os.FileMode
	// PAXRecords are the PAX extended header key-value pairs where there may be extended file attributes
	PAXRecords map[string]string
}

func NewMetadata(header tar.Header, sequence int64) Metadata {
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
		PAXRecords:    header.PAXRecords,
	}
}
