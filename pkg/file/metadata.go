package file

import "os"

type Metadata struct {
	Path          string
	TarHeaderName string
	Linkname      string
	Size          int64
	UserID        int
	GroupID       int
	TypeFlag      byte
	IsDir         bool
	Mode          os.FileMode
}
