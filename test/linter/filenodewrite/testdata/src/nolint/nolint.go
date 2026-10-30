package nolint

import "github.com/anchore/stereoscope/pkg/filetree/filenode"

// a node built here is not in any tree yet, so writing to it is safe; the directive says so
func build() *filenode.FileNode {
	n := &filenode.FileNode{}
	n.RealPath = "x" //nolint:filenodewrite // not in a tree yet
	//nolint:filenodewrite
	n.FileType = 1
	return n
}
