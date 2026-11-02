package parenderef

import "github.com/anchore/stereoscope/pkg/filetree/filenode"

func write(fn *filenode.FileNode) {
	(*fn).RealPath = "x" // want `FileNodes are shared between tree copies`
}
