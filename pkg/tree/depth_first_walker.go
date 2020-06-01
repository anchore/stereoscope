package tree

import (
	"sort"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

// DepthFirstWalker implements stateful depth-first tree traversal.
type DepthFirstWalker struct {
	visitor    NodeVisitor
	tree       Reader
	stack      node.Stack
	visited    node.Set
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

func (w *DepthFirstWalker) Walk(from node.Node) node.Node {
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
