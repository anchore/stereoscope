package filetree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
	"sort"
)

type NodeVisitor func(*FileNode) error

type WalkConditions struct {
	// Return true when the walker should stop traversing (before visiting current node)
	ShouldTerminate func(*FileNode) bool

	// Whether we should visit the current node. Note: this will continue down the same traversal
	// path, only "skipping" over a single node (but still potentially visiting children later)
	// Return true to visit the current node.
	ShouldVisit func(*FileNode) bool

	// Whether we should consider children of this node to be included in the traversal path.
	// Return true to traverse children of this node.
	ShouldContinueBranch func(*FileNode) bool
}

// DepthFirstWalker implements stateful depth-first Tree traversal.
type DepthFirstWalker struct {
	visitor    NodeVisitor
	tree       Reader
	stack      file.Path
	visited    file.Path
	conditions WalkConditions
}

func NewDepthFirstWalker(reader Reader, visitor NodeVisitor) *DepthFirstWalker {
	return &DepthFirstWalker{
		visitor: visitor,
		tree:    reader,
		visited: node.NewIDSet(),
	}
}

func NewDepthFirstWalkerWithConditions(reader Reader, visitor NodeVisitor, conditions WalkConditions) *DepthFirstWalker {
	return &DepthFirstWalker{
		visitor:    visitor,
		tree:       reader,
		visited:    node.NewIDSet(),
		conditions: conditions,
	}
}

func (w *DepthFirstWalker) Walk(from *FileNode) *FileNode {
	w.stack.Push(from)

	for w.stack.Size() > 0 {
		current := w.stack.Pop()
		if w.conditions.ShouldTerminate != nil && w.conditions.ShouldTerminate(current) {
			return current
		}
		cid := current.ID()

		// visit
		if w.visitor != nil && !w.visited.Contains(cid) {
			if w.conditions.ShouldVisit == nil || w.conditions.ShouldVisit != nil && w.conditions.ShouldVisit(current) {
				w.visitor(current)
				w.visited.Add(cid)
			}
		}

		if w.conditions.ShouldContinueBranch != nil && !w.conditions.ShouldContinueBranch(current) {
			continue
		}

		// enqueue children
		children := w.tree.Children(current)
		sort.Sort(sort.Reverse(children))
		for _, child := range children {
			w.stack.Push(child)
		}
	}

	return nil
}

func (w *DepthFirstWalker) WalkAll() {
	for _, from := range w.tree.Roots() {
		w.Walk(from)
	}
}

func (w *DepthFirstWalker) Visited(n node.Node) bool {
	return w.visited.Contains(n.ID())
}
