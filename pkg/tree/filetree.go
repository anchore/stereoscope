package tree

import (
	"fmt"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

type FileTree struct {
	pathToFileNode map[node.ID]file.Reference
	tree           *tree
}

func NewFileTree() *FileTree {
	return &FileTree{
		tree:           newTree(),
		pathToFileNode: make(map[node.ID]file.Reference),
	}
}

func (t *FileTree) Copy() (*FileTree, error) {
	dest := NewFileTree()
	for _, fileNode := range t.pathToFileNode {
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
	_, ok := t.pathToFileNode[path.ID()]
	return ok
}

func (t *FileTree) NodeByPathId(id node.ID) file.Reference {
	return t.pathToFileNode[id]
}

// TODO: this can be a stand alone function
func (t *FileTree) VisitorFn(fn func(file.Reference)) func(node.Node) {
	return func(node node.Node) {
		fn(t.NodeByPathId(node.ID()))
	}
}

// TODO: this can be a stand alone function
func (t *FileTree) ConditionFn(fn func(file.Reference) bool) func(node.Node) bool {
	return func(node node.Node) bool {
		return fn(t.NodeByPathId(node.ID()))
	}
}

func (t *FileTree) Nodes() []file.Reference {
	files := make([]file.Reference, len(t.pathToFileNode))
	idx := 0
	for _, f := range t.pathToFileNode {
		files[idx] = f
		idx++
	}
	return files
}

func (t *FileTree) Node(path file.Path) *file.Reference {
	if value, ok := t.pathToFileNode[path.ID()]; ok {
		return &value
	}
	return nil
}

// TODO: put under test
func (t *FileTree) SetFile(f file.Reference) error {
	original, ok := t.pathToFileNode[f.Path.ID()]

	if !ok {
		return fmt.Errorf("file does not already exist in tree (cannot replace)")
	}
	delete(t.pathToFileNode, original.Path.ID())
	t.pathToFileNode[f.Path.ID()] = f

	return nil
}

func (t *FileTree) AddPath(path file.Path) (file.Reference, error) {
	if f, ok := t.pathToFileNode[path.ID()]; ok {
		return f, nil
	}

	parent, err := path.ParentPath()
	var parentNode *file.Reference
	if err == nil {
		if pNode, ok := t.pathToFileNode[parent.ID()]; !ok {
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
		t.pathToFileNode[f.Path.ID()] = f
	}

	return f, nil
}

func (t *FileTree) RemovePath(path file.Path) error {
	removedNodes, err := t.tree.RemoveNode(path)
	if err != nil {
		return err
	}
	for _, n := range removedNodes {
		delete(t.pathToFileNode, n.ID())
	}
	return nil
}

func (t *FileTree) Reader() Reader {
	return t.tree
}
