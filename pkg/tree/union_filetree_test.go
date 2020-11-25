package tree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
)

func TestUnionFileTree_Squash(t *testing.T) {
	ut := NewUnionTree()
	base := NewFileTree()

	base.AddPathAndMissingAncestors("/home/wagoodman/some/stuff-1.txt")
	originalNode, _ := base.AddPathAndMissingAncestors("/home/wagoodman/some/stuff-2-overlap.txt")
	base.AddPathAndMissingAncestors("/home/wagoodman/more")

	top := NewFileTree()
	top.AddPathAndMissingAncestors("/etc/redhat-release")
	top.AddPathAndMissingAncestors("/home/wagoodman/more/things.txt")
	newNode, _ := top.AddPathAndMissingAncestors("/home/wagoodman/some/stuff-2-overlap.txt")
	top.AddPathAndMissingAncestors("/home/wagoodman/some/stuff-3.txt")
	top.AddPathAndMissingAncestors("/home/wagoodman/another/other-1.txt")

	ut.PushTree(base)
	ut.PushTree(top)

	if originalNode.ID() == newNode.ID() {
		t.Fatal("original and new nodes are the same (should always be different)")
	}

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("cloud not squash trees", err)
	}

	nodes := squashed.AllFiles()
	if len(nodes) != 13 {
		t.Fatal("unexpected squashed tree number of nodes", len(nodes))
	}

	if originalNode.ID() == newNode.ID() {
		t.Fatal("original and new node ids changed after squash")
	}

	if squashed.File(newNode.Path).ID() != newNode.ID() {
		t.Fatal("failed to overwrite a path in the squash tree")
	}

	if squashed.File("/home/wagoodman/more").ID() != top.File("/home/wagoodman/more").ID() {
		t.Fatal("implicit file if did not track to squash")
	}

}

func TestUnionFileTree_Squash_whiteout(t *testing.T) {
	ut := NewUnionTree()
	base := NewFileTree()

	base.AddPathAndMissingAncestors("/some/stuff-1.txt")
	base.AddPathAndMissingAncestors("/some/stuff-2.txt")
	base.AddPathAndMissingAncestors("/other/things-1.txt")

	top := NewFileTree()
	top.AddPathAndMissingAncestors("/some/" + file.OpaqueWhiteout)
	top.AddPathAndMissingAncestors("/other/" + file.WhiteoutPrefix + "things-1.txt")

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("cloud not squash trees", err)
	}

	nodes := squashed.AllFiles()
	if len(nodes) != 3 {
		t.Fatal("unexpected squashed tree number of nodes", len(nodes))
	}

	expectedPaths := []string{
		"/",
		"/some",
		"/other",
	}

	for _, path := range expectedPaths {
		if !squashed.HasPath(file.Path(path)) {
			t.Errorf("expected '%v' but not found", path)
		}
	}

}
