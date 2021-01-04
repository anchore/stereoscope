package filetree

import (
	"os"
	"path/filepath"
	"time"

	"github.com/anchore/stereoscope/pkg/filetree/filenode"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/bmatcuk/doublestar/v2"
)

// basic interface assertion
var _ doublestar.File = (*fileAdapter)(nil)
var _ doublestar.OS = (*osAdapter)(nil)
var _ os.FileInfo = (*fileinfoAdapter)(nil)

type GlobResult struct {
	MatchPath  file.Path
	RealPath   file.Path
	IsDeadLink bool
	Reference  file.Reference
}

// fileAdapter is an object meant to implement the doublestar.File for getting Lstat results for an entire directory.
type fileAdapter struct {
	os       *osAdapter
	filetree *FileTree
	name     string
}

// Close implements io.Closer but is a nop
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
	fn, err := f.filetree.node(file.Path(f.name), linkResolutionStrategy{
		FollowAncestorLinks: true,
		FollowBasenameLinks: true,
	})
	if err != nil {
		return ret, err
	}
	if fn == nil {
		return ret, nil
	}

	for idx, child := range f.filetree.tree.Children(fn) {
		if idx == n && n != -1 {
			break
		}
		requestPath := filepath.Join(f.name, filepath.Base(string(child.ID())))
		r, err := f.os.Lstat(requestPath)
		if err == nil {
			// Lstat by default returns an error when the path cannot be found
			ret = append(ret, r)
		}
	}
	return ret, nil
}

// fileAdapter is an object meant to implement the doublestar.OS for basic file queries (stat, lstat, and open).
type osAdapter struct {
	filetree                     *FileTree
	doNotFollowDeadBasenameLinks bool
}

// Lstat returns a FileInfo describing the named file. If the file is a symbolic link, the returned
// FileInfo describes the symbolic link. Lstat makes no attempt to follow the link.
func (a *osAdapter) Lstat(name string) (os.FileInfo, error) {
	fn, err := a.filetree.node(file.Path(name), linkResolutionStrategy{
		FollowAncestorLinks: true,
		// Lstat by definition requires that basename symlinks are not followed
		FollowBasenameLinks:          false,
		DoNotFollowDeadBasenameLinks: false,
	})
	if err != nil {
		return &fileinfoAdapter{}, err
	}
	if fn == nil {
		return &fileinfoAdapter{}, os.ErrNotExist
	}

	return &fileinfoAdapter{
		VirtualPath: file.Path(name),
		Node:        *fn,
	}, nil
}

// Open the given file path and return a doublestar.File.
func (a *osAdapter) Open(name string) (doublestar.File, error) {
	return &fileAdapter{
		os:       a,
		filetree: a.filetree,
		name:     name,
	}, nil
}

// PathSeparator returns the standard separator between path entries for the underlying filesystem.
func (a *osAdapter) PathSeparator() rune {
	return []rune(file.DirSeparator)[0]
}

// Stat returns a FileInfo describing the named file.
func (a *osAdapter) Stat(name string) (os.FileInfo, error) {
	fn, err := a.filetree.node(file.Path(name), linkResolutionStrategy{
		FollowAncestorLinks:          true,
		FollowBasenameLinks:          true,
		DoNotFollowDeadBasenameLinks: a.doNotFollowDeadBasenameLinks,
	})
	if err != nil {
		return &fileinfoAdapter{}, err
	}
	if fn == nil {
		return &fileinfoAdapter{}, os.ErrNotExist
	}
	return &fileinfoAdapter{
		VirtualPath: file.Path(name),
		Node:        *fn,
	}, nil
}

// fileinfoAdapter is meant to implement the os.FileInfo interface intended only for glob searching. This does NOT
// report correct metadata for all behavior.
type fileinfoAdapter struct {
	VirtualPath file.Path
	Node        filenode.FileNode
}

// Name base name of the file
func (a *fileinfoAdapter) Name() string {
	return a.VirtualPath.Basename()
}

// Size is a dummy return value (since it is not important for globbing). Traditionally this would be the length in
// bytes for regular files.
func (a *fileinfoAdapter) Size() int64 {
	panic("not implemented")
}

// Mode returns the file mode bits for the given file. Note that the only important bits in the bitset is the
// dir and symlink indicators; no other values can be used.
func (a *fileinfoAdapter) Mode() os.FileMode {
	// default to a typical mode value
	mode := os.FileMode(0o755)
	if a.IsDir() {
		mode |= os.ModeDir
	}
	// the underlying implementation for symlinks and hardlinks share the same semantics in the tree implementation
	// (meaning resolution is required) where as in a real file system this is taken care of by the driver
	// by making the file point to the same inode as another --making the indirection transparent to applications.
	if a.Node.FileType == file.TypeSymlink || a.Node.FileType == file.TypeHardLink {
		mode |= os.ModeSymlink
	}
	return mode
}

// ModTime returns a dummy value. Traditionally would be the modification time for the given file.
func (a *fileinfoAdapter) ModTime() time.Time {
	panic("not implemented")
}

// IsDir is an abbreviation for Mode().IsDir().
func (a *fileinfoAdapter) IsDir() bool {
	return a.Node.FileType == file.TypeDir
}

// Sys contains underlying data source (nothing in this case).
func (a *fileinfoAdapter) Sys() interface{} {
	panic("not implemented")
}
