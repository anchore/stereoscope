package tree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"testing"
)

func TestFileTree_AddPath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	fileNode, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	if len(tr.pathToFileNode) != 5 {
		t.Fatal("unexpected file count", len(tr.pathToFileNode))
	}

	if *tr.Node(path) != fileNode {
		t.Fatal("expected pointed to the newly created fileNode")
	}

	parent := file.Path("/home/wagoodman")
	child := file.Path("/home/wagoodman/awesome")

	children := tr.tree.Children(parent)

	if len(children) != 1 {
		t.Fatal("unexpected child count", len(children))
	}

	if children[0].ID() != child.ID() {
		t.Fatal("unexpected child", children[0])
	}

}

func TestFileTree_RemovePath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	_, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	err = tr.RemovePath("/home/wagoodman/awesome")
	if err != nil {
		t.Fatal("could not remote path", err)
	}

	if len(tr.tree.Nodes()) != 3 {
		t.Fatal("unexpected node count", len(tr.tree.Nodes()), tr.tree.Nodes())
	}

	if len(tr.pathToFileNode) != 3 {
		t.Fatal("unexpected file count", len(tr.pathToFileNode))
	}

	if tr.Node(path) != nil {
		t.Fatal("expected file to be missing")
	}

}
