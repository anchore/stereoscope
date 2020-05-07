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

type Reference struct {
	Id   ID
	Path Path
}

func NewFileReference(path Path) Reference {
	i, err := uuidGen.NextID()
	if err != nil {
		panic(err)
	}
	return Reference{
		Path: path,
		Id:   ID(i),
	}
}

func (f *Reference) ID() ID {
	return f.Id
}

func (f *Reference) String() string {
	return fmt.Sprintf("[%v] %v", f.Id, f.Path)
}
