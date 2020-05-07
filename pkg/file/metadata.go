package file

import "os"

type Metadata struct {
	Path     string
	TarPath  string
	TypeFlag byte
	Linkname string
	Size     int64
	Mode     os.FileMode
	Uid      int
	Gid      int
	IsDir    bool
}
