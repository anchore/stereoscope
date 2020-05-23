package tree

import (
	"fmt"
	"path"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

type FileTree struct {
	pathToFileRef map[node.ID]file.Reference
	tree          *tree
}

func NewFileTree() *FileTree {
	return &FileTree{
		tree:          newTree(),
		pathToFileRef: make(map[node.ID]file.Reference),
	}
}

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

func (t *FileTree) HasPath(path file.Path) bool {
	_, ok := t.pathToFileRef[path.ID()]
	return ok
}

func (t *FileTree) FileByPathID(id node.ID) file.Reference {
	return t.pathToFileRef[id]
}

func (t *FileTree) VisitorFn(fn func(file.Reference)) func(node.Node) {
	return func(node node.Node) {
		fn(t.FileByPathID(node.ID()))
	}
}

func (t *FileTree) ConditionFn(fn func(file.Reference) bool) func(node.Node) bool {
	return func(node node.Node) bool {
		return fn(t.FileByPathID(node.ID()))
	}
}

func (t *FileTree) AllFiles() []file.Reference {
	files := make([]file.Reference, len(t.pathToFileRef))
	idx := 0
	for _, f := range t.pathToFileRef {
		files[idx] = f
		idx++
	}
	return files
}

func (t *FileTree) Files(paths []file.Path) ([]file.Reference, error) {
	files := make([]file.Reference, len(paths))
	idx := 0
	for _, path := range paths {
		f := t.File(path)
		if f == nil {
			return nil, fmt.Errorf("could not find path: %+v", path)
		}
		files[idx] = *f
		idx++
	}
	return files, nil
}

func (t *FileTree) File(path file.Path) *file.Reference {
	if value, ok := t.pathToFileRef[path.ID()]; ok {
		return &value
	}
	return nil
}

// TODO: put under test
func (t *FileTree) SetFile(f file.Reference) error {
	original, ok := t.pathToFileRef[f.Path.ID()]

	if !ok {
		return fmt.Errorf("file does not already exist in tree (cannot replace)")
	}
	delete(t.pathToFileRef, original.Path.ID())
	t.pathToFileRef[f.Path.ID()] = f

	return nil
}

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
		var err error
		if parentNode == nil {
			err = t.tree.AddRoot(f.Path)
		} else {
			err = t.tree.AddChild(parentNode.Path, f.Path)
		}
		if err != nil {
			return file.Reference{}, err
		}
		t.pathToFileRef[f.Path.ID()] = f
	}

	return f, nil
}

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

func (t *FileTree) Reader() Reader {
	return t.tree
}

func (t *FileTree) Walk(fn func(f file.Reference)) {
	visitor := t.VisitorFn(fn)
	w := NewDepthFirstWalker(t.Reader(), visitor)
	w.WalkAll()
}

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

func (t *FileTree) Equal(other *FileTree) bool {
	if len(t.pathToFileRef) != len(other.pathToFileRef) {
		return false
	}

	extra, missing := t.PathDiff(other)

	return len(extra) == 0 && len(missing) == 0
}

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
				log.WithFields(
					map[string]interface{}{
						"path": f.Path,
					},
				).Errorf("filetree merge failed to remove child paths: %w", err)
			}

			return
		}

		if f.Path.IsWhiteout() {
			lowerPath, err := f.Path.UnWhiteoutPath()
			if err != nil {
				log.WithFields(
					map[string]interface{}{
						"path": f.Path,
					},
				).Errorf("filetree merge failed to find original path for whiteout: %w", err)
			}

			err = t.RemovePath(lowerPath)
			if err != nil {
				log.WithFields(
					map[string]interface{}{
						"path": lowerPath,
					},
				).Errorf("filetree merge failed to remove path: %w", err)
			}
		} else {
			if !t.HasPath(f.Path) {
				_, err := t.AddPath(f.Path)
				if err != nil {
					log.WithFields(
						map[string]interface{}{
							"path": f.Path,
						},
					).Errorf("filetree merge failed to add path: %w", err)
				}
			}
			err := t.SetFile(f)
			if err != nil {
				log.WithFields(
					map[string]interface{}{
						"file": f,
					},
				).Errorf("filetree merge failed to set file reference: %w", err)
			}
		}
	})

	w := NewDepthFirstWalkerWithConditions(other.Reader(), visitor, conditions)
	w.WalkAll()
}
