package embedded

import "github.com/anchore/stereoscope/pkg/filetree/filenode"

type wrapper struct {
	*filenode.FileNode
}

func write(w wrapper) {
	w.RealPath = "x" // want `FileNodes are shared between tree copies`
}
