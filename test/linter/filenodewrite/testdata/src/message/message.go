package message

import "github.com/anchore/stereoscope/pkg/filetree/filenode"

// the finding should name the form that passes: a by-value copy, changed, then swapped in
func write(fn *filenode.FileNode) {
	fn.RealPath = "x" // want `copy it by value \(c := \*fn\)`
}
