package tree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

func dfsTestTree() *FileTree {
	tr := NewFileTree()
	tr.AddPath("/home/wagoodman/some/stuff-1.txt")
	tr.AddPath("/home/wagoodman/some/stuff-2.txt")
	tr.AddPath("/home/wagoodman/more/file.txt")
	return tr
}

func TestDFS_WalkAll(t *testing.T) {
	tr := dfsTestTree()

	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		file.Path("/home/wagoodman"),
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]file.Path, 0)
	visitor := tr.VisitorFn(func(path file.Path, f *file.Reference) {
		actual = append(actual, path)
	})

	walker := NewDepthFirstWalker(tr.Reader(), visitor)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := file.Path("/home/wagoodman")
	expected := []file.Path{
		walkFrom,
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]file.Path, 0)
	visitor := tr.VisitorFn(func(path file.Path, f *file.Reference) {
		actual = append(actual, path)
	})

	walker := NewDepthFirstWalker(tr.Reader(), visitor)
	walker.Walk(walkFrom)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldTerminate(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := file.Path("/home/wagoodman")
	terminatePath := file.Path("/home/wagoodman/some")
	expected := []file.Path{
		walkFrom,
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/more/file.txt"),
	}

	actual := make([]file.Path, 0)
	visitor := tr.VisitorFn(func(path file.Path, f *file.Reference) {
		actual = append(actual, path)
	})

	h := WalkConditions{
		ShouldTerminate: func(path node.Node) bool {
			if file.Path(path.ID()) == terminatePath {
				return true
			}
			return false
		},
		ShouldVisit:          nil,
		ShouldContinueBranch: nil,
	}
	walker := NewDepthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.Walk(walkFrom)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldVisit(t *testing.T) {
	tr := dfsTestTree()

	walkFrom := file.Path("/home/wagoodman")
	skipPath := file.Path("/home/wagoodman/some")
	expected := []file.Path{
		walkFrom,
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
	}

	actual := make([]file.Path, 0)
	visitor := tr.VisitorFn(func(path file.Path, f *file.Reference) {
		actual = append(actual, path)
	})

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit: func(path node.Node) bool {
			if file.Path(path.ID()) == skipPath {
				return false
			}
			return true
		},
		ShouldContinueBranch: nil,
	}
	walker := NewDepthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.Walk(walkFrom)

	assertExpectedTraversal(t, expected, actual)
}

func TestDFS_Walk_ShouldPruneBranch(t *testing.T) {
	tr := dfsTestTree()

	prunePath := file.Path("/home/wagoodman")
	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		prunePath,
	}

	actual := make([]file.Path, 0)
	visitor := tr.VisitorFn(func(path file.Path, f *file.Reference) {
		actual = append(actual, path)
	})

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit:     nil,
		ShouldContinueBranch: func(path node.Node) bool {
			if file.Path(path.ID()) == prunePath {
				return false
			}
			return true
		},
	}
	walker := NewDepthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func assertExpectedTraversal(t *testing.T, expected []file.Path, actual []file.Path) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("Did not traverse all nodes (expected %d, got %d)", len(expected), len(actual))
	}

	for idx, a := range actual {
		if expected[idx].ID() != a.ID() {
			t.Errorf("expected visit ID @%v = '%v', got %v", idx, expected[idx], a.ID())
		}
	}
}
