package file

import "os"

// Metadata represents all file metadata of interest (used today for in-tar file resolution).
type Metadata struct {
	// Path is the absolute path representation to the file
	Path string
	// TarHeaderName is the exact entry name as found within a tar header
	TarHeaderName string
	// Linkname is populated only for hardlinks / symlinks, can be an absolute or relative.
	Linkname string
	// Size of the file in bytes.
	Size    int64
	UserID  int
	GroupID int
	// TypeFlag is the tar.TypeFlag entry for the file
	TypeFlag byte
	IsDir    bool
	Mode     os.FileMode
}
