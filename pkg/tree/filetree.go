package tree

import (
	"fmt"
	"path"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

// FileTree represents a file/directory tree
type FileTree struct {
	pathToFileRef map[node.ID]file.Reference
	tree          *tree
}

// NewFileTree creates a new FileTree instance.
func NewFileTree() *FileTree {
	return &FileTree{
		tree:          newTree(),
		pathToFileRef: make(map[node.ID]file.Reference),
	}
}

// Copy returns a copy of the current FileTree.
func (t *FileTree) Copy() (*FileTree, error) {
	dest := NewFileTree()
	for _, fileNode := range t.pathToFileRef {
		_, err := dest.AddPath(fileNode.Path)
		if err != nil {
			return nil, err
		}
		err = dest.SetFile(fileNode)
		if err != nil {
			return nil, err
		}
	}
	return dest, nil
}

// HasPath indicates is the given path is in the file tree.
func (t *FileTree) HasPath(path file.Path) bool {
	_, ok := t.pathToFileRef[path.ID()]
	return ok
}

// fileByPathID indicates if the given node ID is in the FileTree (useful for tree -> FileTree node resolution).
func (t *FileTree) fileByPathID(id node.ID) file.Reference {
	return t.pathToFileRef[id]
}

// VisitorFn, used for traversal, wraps the given user function (meant to walk file.References) with a function that
// can resolve an underlying tree Node to a file.Reference.
func (t *FileTree) VisitorFn(fn func(file.Reference)) func(node.Node) {
	return func(node node.Node) {
		fn(t.fileByPathID(node.ID()))
	}
}

// ConditionFn, used for conditioning traversal, wraps the given user function (meant to walk file.References) with a
// function that can resolve an underlying tree Node to a file.Reference.
func (t *FileTree) ConditionFn(fn func(file.Reference) bool) func(node.Node) bool {
	return func(node node.Node) bool {
		return fn(t.fileByPathID(node.ID()))
	}
}

// AllFiles returns all files and directories within the FileTree.
func (t *FileTree) AllFiles() []file.Reference {
	files := make([]file.Reference, len(t.pathToFileRef))
	idx := 0
	for _, f := range t.pathToFileRef {
		files[idx] = f
		idx++
	}
	return files
}

// File fetches a file.Reference for the given path. Returns nil if the path does not exist in the FileTree.
func (t *FileTree) File(path file.Path) *file.Reference {
	if value, ok := t.pathToFileRef[path.ID()]; ok {
		return &value
	}
	return nil
}

// File fetches zero to many file.References for the given glob pattern.
func (t *FileTree) FilesByGlob(query string) ([]file.Reference, error) {
	result := make([]file.Reference, 0)

	for _, f := range t.pathToFileRef {
		if internal.GlobMatch(query, string(f.Path)) {
			result = append(result, f)
		}
	}

	return result, nil
}

// TODO: put under test
// SetFile replaces any file already in the FileTree with the given file.Reference.
func (t *FileTree) SetFile(f file.Reference) error {
	original, ok := t.pathToFileRef[f.Path.ID()]

	if !ok {
		return fmt.Errorf("file does not already exist in tree (cannot replace)")
	}
	delete(t.pathToFileRef, original.Path.ID())
	t.pathToFileRef[f.Path.ID()] = f

	return nil
}

// AddPath adds a new path (and all containing paths) to the tree. The resulting file.Reference of the new
// (leaf) addition is returned.
func (t *FileTree) AddPath(path file.Path) (file.Reference, error) {
	if f, ok := t.pathToFileRef[path.ID()]; ok {
		return f, nil
	}

	parent, err := path.ParentPath()
	var parentNode *file.Reference
	if err == nil {
		if pNode, ok := t.pathToFileRef[parent.ID()]; !ok {
			pNode, err = t.AddPath(parent)
			if err != nil {
				return file.Reference{}, err
			}
			parentNode = &pNode
		} else {
			parentNode = &pNode
		}
	}

	f := file.NewFileReference(path)
	if !t.tree.HasNode(path.ID()) {
		// add the node to the tree
		var err error
		if parentNode == nil {
			err = t.tree.AddRoot(f.Path)
		} else {
			err = t.tree.AddChild(parentNode.Path, f.Path)
		}
		if err != nil {
			return file.Reference{}, err
		}

		// track the path for fast lookup
		t.pathToFileRef[f.Path.ID()] = f
	}

	return f, nil
}

// RemovePath deletes the file.Reference from the FileTree by the given path.
func (t *FileTree) RemovePath(path file.Path) error {
	removedNodes, err := t.tree.RemoveNode(path)
	if err != nil {
		return err
	}
	for _, n := range removedNodes {
		delete(t.pathToFileRef, n.ID())
	}
	return nil
}

// RemoveChildPaths deletes all children of the given path (not including the given path).
func (t *FileTree) RemoveChildPaths(path file.Path) error {
	removedNodes := make(node.Nodes, 0)
	for _, child := range t.tree.Children(path) {
		nodes, err := t.tree.RemoveNode(child)
		if err != nil {
			return err
		}
		removedNodes = append(removedNodes, nodes...)
	}
	for _, n := range removedNodes {
		delete(t.pathToFileRef, n.ID())
	}
	return nil
}

// Reader returns a tree.Reader useful for tree traversal.
func (t *FileTree) Reader() Reader {
	return t.tree
}

// Walk takes a visitor function and invokes it for all file.References within the FileTree in depth-first ordering.
func (t *FileTree) Walk(fn func(f file.Reference)) {
	visitor := t.VisitorFn(fn)
	w := NewDepthFirstWalker(t.Reader(), visitor)
	w.WalkAll()
}

// PathDiff shows the path differences between two trees (useful for testing)
func (t *FileTree) PathDiff(other *FileTree) (extra, missing []file.Path) {
	extra = make([]file.Path, 0)
	missing = make([]file.Path, 0)

	ourPaths := internal.NewStringSet()
	for _, f := range t.pathToFileRef {
		ourPaths.Add(string(f.Path))
	}

	theirPaths := internal.NewStringSet()
	for _, f := range other.pathToFileRef {
		theirPaths.Add(string(f.Path))
	}

	for _, f := range other.pathToFileRef {
		if !ourPaths.Contains(string(f.Path)) {
			extra = append(extra, f.Path)
		}
	}

	for _, f := range t.pathToFileRef {
		if !theirPaths.Contains(string(f.Path)) {
			missing = append(missing, f.Path)
		}
	}

	return
}

// Equal indicates if the two trees have the same paths or not.
func (t *FileTree) Equal(other *FileTree) bool {
	if len(t.pathToFileRef) != len(other.pathToFileRef) {
		return false
	}

	extra, missing := t.PathDiff(other)

	return len(extra) == 0 && len(missing) == 0
}

// Merge takes the given tree and combines it with the current tree, preferring files in the other tree if there
// are path conflicts. This is the basis function for squashing (where the current tree is the bottom tree and the
// given tree is the top tree).
func (t *FileTree) Merge(other *FileTree) {
	conditions := WalkConditions{
		ShouldContinueBranch: other.ConditionFn(func(f file.Reference) bool {
			return !f.Path.IsWhiteout()
		}),
		ShouldVisit: other.ConditionFn(func(f file.Reference) bool {
			return !f.Path.IsDirWhiteout()
		}),
	}

	visitor := other.VisitorFn(func(f file.Reference) {
		// opaque whiteouts must be processed first
		opaqueWhiteoutChild := file.Path(path.Join(string(f.Path), file.OpaqueWhiteout))
		if other.HasPath(opaqueWhiteoutChild) {
			err := t.RemoveChildPaths(f.Path)
			if err != nil {
				log.Errorf("filetree merge failed to remove child paths (path=%s): %w", f.Path, err)
			}

			return
		}

		if f.Path.IsWhiteout() {
			lowerPath, err := f.Path.UnWhiteoutPath()
			if err != nil {
				log.Errorf("filetree merge failed to find original path for whiteout (path=%s): %w", f.Path, err)
			}

			err = t.RemovePath(lowerPath)
			if err != nil {
				log.Errorf("filetree merge failed to remove path (path=%s): %w", lowerPath, err)
			}
		} else {
			if !t.HasPath(f.Path) {
				_, err := t.AddPath(f.Path)
				if err != nil {
					log.Errorf("filetree merge failed to add path (path=%s): %w", f.Path, err)
				}
			}
			err := t.SetFile(f)
			if err != nil {
				log.Errorf("filetree merge failed to set file reference (ref=%+v): %w", f, err)
			}
		}
	})

	w := NewDepthFirstWalkerWithConditions(other.Reader(), visitor, conditions)
	w.WalkAll()
}
