package tree

import (
	"os"
	"path"
	"time"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
	"github.com/bmatcuk/doublestar/v2"
)

type fileAdapter struct {
	os   *osAdapter
	name string
}

func (f *fileAdapter) Close() error {
	return nil
}

// Readdir reads the contents of the directory associated with file and
// returns a slice of up to n FileInfo values, as would be returned
// by Lstat, in directory order. Subsequent calls on the same file will yield
// further FileInfos.
//
// If n > 0, Readdir returns at most n FileInfo structures. In this case, if
// Readdir returns an empty slice, it will return a non-nil error
// explaining why. At the end of a directory, the error is io.EOF.
//
// If n <= 0, Readdir returns all the FileInfo from the directory in
// a single slice. In this case, if Readdir succeeds (reads all
// the way to the end of the directory), it returns the slice and a
// nil error. If it encounters an error before the end of the
// directory, Readdir returns the FileInfo read until that point
// and a non-nil error.
func (f *fileAdapter) Readdir(n int) ([]os.FileInfo, error) {
	if f == nil {
		return nil, os.ErrInvalid
	}
	var ret = make([]os.FileInfo, 0)
	for idx, child := range f.os.ft.tree.Children(file.Path(f.name)) {
		if idx == n && n != -1 {
			break
		}
		r, err := f.os.Lstat(string(child.ID()))
		if err != nil {
			return nil, err
		}
		ret = append(ret, r)
	}
	return ret, nil
}

type osAdapter struct {
	ft *FileTree
}

// Lstat returns a FileInfo describing the named file. If the file is a symbolic link, the returned
// FileInfo describes the symbolic link. Lstat makes no attempt to follow the link. If there is an error,
// it will be of type *PathError.
func (a *osAdapter) Lstat(name string) (os.FileInfo, error) {
	_, ok := a.ft.pathToFileRef[node.ID(name)]
	if !ok {
		return &fileinfoAdapter{}, os.ErrNotExist
	}
	isDir := len(a.ft.tree.Children(file.Path(name))) > 0
	ref := a.ft.File(file.Path(name))
	isLink := false
	if ref != nil {
		isLink = ref.LinkPath != ""
	}
	return &fileinfoAdapter{
		name:    name,
		dir:     isDir,
		symlink: isLink,
	}, nil
}

func (a *osAdapter) Open(name string) (doublestar.File, error) {
	return &fileAdapter{a, name}, nil
}

func (a *osAdapter) PathSeparator() rune {
	return []rune(file.DirSeparator)[0]
}

// Stat returns a FileInfo describing the named file. If there is an error, it will be of type *PathError.
func (a *osAdapter) Stat(name string) (os.FileInfo, error) {
	exists, _, ref, err := a.ft.resolveLinkPathToFile(file.Path(name))
	if err != nil {
		return &fileinfoAdapter{}, err
	}
	if !exists {
		return &fileinfoAdapter{}, os.ErrNotExist
	}
	isDir := len(a.ft.tree.Children(file.Path(name))) > 0
	isLink := false
	if ref != nil {
		isLink = ref.LinkPath != ""
	}
	return &fileinfoAdapter{
		name:    name,
		dir:     isDir,
		symlink: isLink,
	}, nil
}

type fileinfoAdapter struct {
	name    string
	dir     bool
	symlink bool
}

// base name of the file
func (a *fileinfoAdapter) Name() string {
	return path.Base(a.name)
}

// length in bytes for regular files; system-dependent for others
func (a *fileinfoAdapter) Size() int64 {
	return 1
}

// file mode bits
func (a *fileinfoAdapter) Mode() os.FileMode {
	mode := os.FileMode(0o755)
	if a.dir {
		mode |= os.ModeDir
	}
	if a.symlink {
		mode |= os.ModeSymlink
	}
	return mode
}

// modification time
func (a *fileinfoAdapter) ModTime() time.Time {
	return time.Now()
}

// abbreviation for Mode().IsDir()
func (a *fileinfoAdapter) IsDir() bool {
	return a.dir
}

// underlying data source (can return nil)
func (a *fileinfoAdapter) Sys() interface{} {
	return nil
}
