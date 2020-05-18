package tree

import "github.com/anchore/stereoscope/pkg/tree/node"

type WalkConditions struct {
	// Return true when the walker should stop traversing (before visiting current node)
	ShouldTerminate func(node.Node) bool

	// Whether we should visit the current node. Note: this will continue down the same traversal
	// path, only "skipping" over a single node (but still potentially visiting children later)
	// Return true to visit the current node.
	ShouldVisit func(node.Node) bool

	// Whether we should consider children of this node to be included in the traversal path.
	// Return true to traverse children of this node.
	ShouldContinueBranch func(node.Node) bool
}
