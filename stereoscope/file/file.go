package file

import (
	"fmt"
	"github.com/sony/sonyflake"
	"time"
)

var uuidGen *sonyflake.Sonyflake

func init() {
	var st sonyflake.Settings
	st.StartTime = time.Now()
	uuidGen = sonyflake.NewSonyflake(st)
}

type ID uint64

type File struct {
	Id   ID
	Path Path
}

func NewFile(path Path) *File {
	i, err := uuidGen.NextID()
	if err != nil {
		panic(err)
	}
	return &File{
		Path: path,
		Id:   ID(i),
	}
}

func (f *File) ID() ID {
	return f.Id
}

func (f *File) String() string {
	return fmt.Sprintf("[%v] %v", f.Id, f.Path)
}
