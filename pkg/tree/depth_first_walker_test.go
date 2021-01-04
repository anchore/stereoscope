package tree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

func dfsTestTree() *Tree {
	root := newTestNode("/")
	home := newTestNode("/home")
	wagoodman := newTestNode("/home/wagoodman")
	some := newTestNode("/home/wagoodman/some")
	stuff1 := newTestNode("/home/wagoodman/some/stuff-1.txt")
	stuff2 := newTestNode("/home/wagoodman/some/stuff-2.txt")
	more := newTestNode("/home/wagoodman/more")
	file := newTestNode("/home/wagoodman/more/file.txt")

	tr := NewTree()
	tr.AddRoot(root)
	tr.AddChild(root, home)
	tr.AddChild(home, wagoodman)
	tr.AddChild(wagoodman, some)
	tr.AddChild(some, stuff1)
	tr.AddChild(some, stuff2)
	tr.AddChild(wagoodman, more)
	tr.AddChild(more, file)

	return tr
}

func TestDFS_WalkAll(t *testing.T) {
	tr := dfsTestTree()

	expected := []node.ID{
		node.ID("/"),
		node.ID("/home"),
		node.ID("/home/wagoodman"),
		node.ID("/home/wagoodman/more"),
		node.ID("/home/wagoodman/more/file.txt"),
		node.ID("/home/wagoodman/some"),
		node.ID("/home/wagoodman/some/stuff-1.txt"),
		node.ID("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]node.ID, 0)
	visitor := func(node node.Node) error {
		actual = append(actual, node.ID())
		return nil
	}

	walker := NewDepthFirstWalker(tr, visitor)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := node.ID("/home/wagoodman")
	walkFromNode := tr.Node(walkFrom)

	expected := []node.ID{
		walkFrom,
		node.ID("/home/wagoodman/more"),
		node.ID("/home/wagoodman/more/file.txt"),
		node.ID("/home/wagoodman/some"),
		node.ID("/home/wagoodman/some/stuff-1.txt"),
		node.ID("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]node.ID, 0)
	visitor := func(node node.Node) error {
		actual = append(actual, node.ID())
		return nil
	}

	walker := NewDepthFirstWalker(tr, visitor)
	walker.Walk(walkFromNode)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldTerminate(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := node.ID("/home/wagoodman")
	walkFromNode := tr.Node(walkFrom)
	terminatePath := node.ID("/home/wagoodman/some")
	expected := []node.ID{
		walkFrom,
		node.ID("/home/wagoodman/more"),
		node.ID("/home/wagoodman/more/file.txt"),
	}

	actual := make([]node.ID, 0)
	visitor := func(node node.Node) error {
		actual = append(actual, node.ID())
		return nil
	}

	h := WalkConditions{
		ShouldTerminate: func(n node.Node) bool {
			if n.ID() == terminatePath {
				return true
			}
			return false
		},
		ShouldVisit:          nil,
		ShouldContinueBranch: nil,
	}
	walker := NewDepthFirstWalkerWithConditions(tr, visitor, h)
	walker.Walk(walkFromNode)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldVisit(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := node.ID("/home/wagoodman")
	walkFromNode := tr.Node(walkFrom)
	skipPath := node.ID("/home/wagoodman/some")
	expected := []node.ID{
		walkFrom,
		node.ID("/home/wagoodman/more"),
		node.ID("/home/wagoodman/more/file.txt"),
		node.ID("/home/wagoodman/some/stuff-1.txt"),
		node.ID("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]node.ID, 0)
	visitor := func(node node.Node) error {
		actual = append(actual, node.ID())
		return nil
	}

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit: func(n node.Node) bool {
			if n.ID() == skipPath {
				return false
			}
			return true
		},
		ShouldContinueBranch: nil,
	}
	walker := NewDepthFirstWalkerWithConditions(tr, visitor, h)
	walker.Walk(walkFromNode)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldPruneBranch(t *testing.T) {
	tr := dfsTestTree()

	prunePath := node.ID("/home/wagoodman")
	expected := []node.ID{
		node.ID("/"),
		node.ID("/home"),
		prunePath,
	}

	actual := make([]node.ID, 0)
	visitor := func(node node.Node) error {
		actual = append(actual, node.ID())
		return nil
	}

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit:     nil,
		ShouldContinueBranch: func(n node.Node) bool {
			if n.ID() == prunePath {
				return false
			}
			return true
		},
	}
	walker := NewDepthFirstWalkerWithConditions(tr, visitor, h)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func assertExpectedTraversal(t *testing.T, expected []node.ID, actual []node.ID) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Did not traverse all nodes (expected %d, got %d)", len(expected), len(actual))
	}

	for idx, a := range actual {
		if expected[idx] != a {
			t.Errorf("expected visit ID @%v = '%v', got %v", idx, expected[idx], a)
		}
	}
}
