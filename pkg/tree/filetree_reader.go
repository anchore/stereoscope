package tree

import (
	"github.com/anchore/stereoscope/pkg/file"
)

type FileTreeReader interface {
	AllFiles() []file.Reference
	HasPath(path file.Path) bool
	File(path file.Path) *file.Reference
	Walk(fn func(f file.Reference))
	FilesByGlob(string) ([]file.Reference, error)
}
