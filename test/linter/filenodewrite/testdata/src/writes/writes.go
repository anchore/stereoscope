package writes

import "github.com/anchore/stereoscope/pkg/filetree/filenode"

func set(p *string) { *p = "x" }

func flagged(fn *filenode.FileNode, m map[string]*filenode.FileNode, s []*filenode.FileNode) {
	fn.RealPath = "x"         // want `FileNodes are shared between tree copies`
	fn.FileType += 1          // want `FileNodes are shared between tree copies`
	fn.FileType++             // want `FileNodes are shared between tree copies`
	*fn = filenode.FileNode{} // want `FileNodes are shared between tree copies`
	set(&fn.RealPath)         // want `FileNodes are shared between tree copies`
	m["a"].RealPath = "x"     // want `FileNodes are shared between tree copies`
	s[0].RealPath = "x"       // want `FileNodes are shared between tree copies`
}

func notFlagged(fn *filenode.FileNode) *filenode.FileNode {
	c := *fn
	c.Reference = &filenode.Reference{}
	fn = &c
	return fn
}
