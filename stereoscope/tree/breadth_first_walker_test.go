package tree

import (
	"github.com/anchore/stereoscope/stereoscope/file"
	"github.com/anchore/stereoscope/stereoscope/tree/node"
	"testing"
)

func bfsTestTree() *FileTree {
	tr := NewFileTree()
	tr.AddPath("/home/wagoodman/some/stuff-1.txt")
	tr.AddPath("/home/wagoodman/some/stuff-2.txt")
	tr.AddPath("/home/wagoodman/more/file.txt")
	return tr
}

func assertExpectedTraversal(t *testing.T, expected []file.Path, actual []*file.File) {
	if len(actual) != len(expected) {
		t.Errorf("Did not traverse all nodes (expected %d, got %d)", len(expected), len(actual))
	}

	for idx, a := range actual {
		if expected[idx].ID() != a.Path.ID() {
			t.Errorf("eEpected visit ID @%v = '%v', got %v", idx, expected[idx], a)
		}
	}
}

func TestBFS_WalkAll(t *testing.T) {
	tr := bfsTestTree()

	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		file.Path("/home/wagoodman"),
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/some"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
	}

	actual := make([]*file.File, 0)
	visitor := tr.VisitorFn(func(f *file.File) {
		actual = append(actual, f)
	})

	walker := NewBreadthFirstWalker(tr.Reader(), visitor)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func TestBFS_Walk(t *testing.T) {
	tr := bfsTestTree()

	fromPath := file.Path("/home/wagoodman")
	expected := []file.Path{
		fromPath,
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/some"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
	}

	actual := make([]*file.File, 0)
	visitor := tr.VisitorFn(func(f *file.File) {
		actual = append(actual, f)
	})

	walker := NewBreadthFirstWalker(tr.Reader(), visitor)
	walker.Walk(fromPath)

	assertExpectedTraversal(t, expected, actual)
}

func TestBFS_Walk_ShouldTerminate(t *testing.T) {
	tr := bfsTestTree()

	terminatePath := file.Path("/home/wagoodman/some")
	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		file.Path("/home/wagoodman"),
		file.Path("/home/wagoodman/more"),
	}

	actual := make([]*file.File, 0)
	visitor := tr.VisitorFn(func(f *file.File) {
		actual = append(actual, f)
	})

	h := WalkConditions{
		ShouldTerminate: func(path node.Node) bool {
			if tr.NodeByPathId(path.ID()).Path == terminatePath {
				return true
			}
			return false
		},
		ShouldVisit:          nil,
		ShouldContinueBranch: nil,
	}
	walker := NewBreadthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func TestBFS_Walk_ShouldVisit(t *testing.T) {
	tr := bfsTestTree()

	skipPath := file.Path("/home/wagoodman/some")
	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		file.Path("/home/wagoodman"),
		file.Path("/home/wagoodman/more"),
		file.Path("/home/wagoodman/more/file.txt"),
		file.Path("/home/wagoodman/some/stuff-2.txt"),
		file.Path("/home/wagoodman/some/stuff-1.txt"),
	}

	actual := make([]*file.File, 0)
	visitor := tr.VisitorFn(func(f *file.File) {
		actual = append(actual, f)
	})

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit: func(path node.Node) bool {
			if tr.NodeByPathId(path.ID()).Path == skipPath {
				return false
			}
			return true
		},
		ShouldContinueBranch: nil,
	}
	walker := NewBreadthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}

func TestBFS_Walk_ShouldContinueBranch(t *testing.T) {
	tr := bfsTestTree()

	prunePath := file.Path("/home/wagoodman/some")
	expected := []file.Path{
		file.Path("/"),
		file.Path("/home"),
		file.Path("/home/wagoodman"),
		file.Path("/home/wagoodman/more"),
		prunePath,
		file.Path("/home/wagoodman/more/file.txt"),
	}

	actual := make([]*file.File, 0)
	visitor := tr.VisitorFn(func(f *file.File) {
		actual = append(actual, f)
	})

	h := WalkConditions{
		ShouldTerminate: nil,
		ShouldVisit:     nil,
		ShouldContinueBranch: func(path node.Node) bool {
			if tr.NodeByPathId(path.ID()).Path == prunePath {
				return false
			}
			return true
		},
	}
	walker := NewBreadthFirstWalkerWithConditions(tr.Reader(), visitor, h)
	walker.WalkAll()

	assertExpectedTraversal(t, expected, actual)
}
