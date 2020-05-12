package file

import "os"

type Metadata struct {
	Path          string
	TarHeaderName string
	TypeFlag      byte
	Linkname      string
	Size          int64
	Mode          os.FileMode
	Uid           int
	Gid           int
	IsDir         bool
}
