package filetree

import (
	"fmt"
	"sort"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
)

type FileNodeVisitor func(file.Path, filenode.FileNode) error

type WalkConditions struct {
	// Return true when the walker should stop traversing (before visiting current node)
	ShouldTerminate func(file.Path, filenode.FileNode) bool

	// Whether we should visit the current node. Note: this will continue down the same traversal
	// path, only "skipping" over a single node (but still potentially visiting children later)
	// Return true to visit the current node.
	ShouldVisit func(file.Path, filenode.FileNode) bool

	// Whether we should consider children of this node to be included in the traversal path.
	// Return true to traverse children of this node.
	ShouldContinueBranch func(file.Path, filenode.FileNode) bool
}

// DepthFirstPathWalker implements stateful depth-first Tree traversal.
type DepthFirstPathWalker struct {
	visitor      FileNodeVisitor
	tree         *FileTree
	pathStack    file.PathStack
	visitedPaths file.PathSet
	conditions   WalkConditions
}

func NewDepthFirstWalker(tree *FileTree, visitor FileNodeVisitor, conditions *WalkConditions) *DepthFirstPathWalker {
	w := &DepthFirstPathWalker{
		visitor:      visitor,
		tree:         tree,
		visitedPaths: file.NewPathSet(),
	}
	if conditions != nil {
		w.conditions = *conditions
	}
	return w
}

// nolint:gocognit
func (w *DepthFirstPathWalker) Walk(from file.Path) (file.Path, *filenode.FileNode, error) {
	w.pathStack.Push(from)

	var currentPath file.Path
	var currentNode *filenode.FileNode
	var err error

	for w.pathStack.Size() > 0 {
		currentPath = w.pathStack.Pop()
		_, currentNode, err = w.tree.node(currentPath, linkResolutionStrategy{
			FollowAncestorLinks: true,
			FollowBasenameLinks: true,
		})
		if err != nil {
			return "", nil, err
		}
		if currentNode == nil {
			return "", nil, fmt.Errorf("nil node at path=%q", currentPath)
		}

		if w.conditions.ShouldTerminate != nil && w.conditions.ShouldTerminate(currentPath, *currentNode) {
			return currentPath, currentNode, nil
		}
		currentPath = currentPath.Normalize()

		// visit
		if w.visitor != nil && !w.visitedPaths.Contains(currentPath) {
			if w.conditions.ShouldVisit == nil || w.conditions.ShouldVisit != nil && w.conditions.ShouldVisit(currentPath, *currentNode) {
				err := w.visitor(currentPath, *currentNode)
				if err != nil {
					return currentPath, currentNode, err
				}
				w.visitedPaths.Add(currentPath)
			}
		}

		if w.conditions.ShouldContinueBranch != nil && !w.conditions.ShouldContinueBranch(currentPath, *currentNode) {
			continue
		}

		// enqueue child paths
		childPaths, err := w.tree.ListPaths(currentPath)
		if err != nil {
			return "", nil, err
		}
		sort.Sort(sort.Reverse(file.Paths(childPaths)))
		for _, childPath := range childPaths {
			w.pathStack.Push(childPath)
		}
	}

	return currentPath, currentNode, nil
}

func (w *DepthFirstPathWalker) WalkAll() error {
	_, _, err := w.Walk("/")
	return err
}

func (w *DepthFirstPathWalker) Visited(p file.Path) bool {
	return w.visitedPaths.Contains(p)
}
