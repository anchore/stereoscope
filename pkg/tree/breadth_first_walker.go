package tree

import (
	"sort"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

type BreadthFirstWalker struct {
	visitor    NodeVisitor
	tree       Reader
	queue      node.Queue
	visited    node.Set
	conditions WalkConditions
}

func NewBreadthFirstWalker(reader Reader, visitor NodeVisitor) *BreadthFirstWalker {
	return &BreadthFirstWalker{
		visitor: visitor,
		tree:    reader,
		visited: node.NewIDSet(),
	}
}

func NewBreadthFirstWalkerWithConditions(reader Reader, visitor NodeVisitor, conditions WalkConditions) *BreadthFirstWalker {
	return &BreadthFirstWalker{
		visitor:    visitor,
		tree:       reader,
		visited:    node.NewIDSet(),
		conditions: conditions,
	}
}

func (w *BreadthFirstWalker) Walk(from node.Node) node.Node {
	w.queue.Enqueue(from)

	for w.queue.Size() > 0 {
		current := w.queue.Dequeue()

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
		sort.Sort(children)
		for _, child := range children {
			w.queue.Enqueue(child)
		}
	}

	return nil
}

func (w *BreadthFirstWalker) WalkAll() {
	for _, from := range w.tree.Roots() {
		if w.Visited(from) {
			continue
		}
		w.Walk(from)
	}
}

func (w *BreadthFirstWalker) Visited(n node.Node) bool {
	return w.visited.Contains(n.ID())
}
