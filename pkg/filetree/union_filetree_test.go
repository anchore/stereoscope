package filetree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
)

func TestUnionFileTree_Squash(t *testing.T) {
	ut := NewUnionFileTree()
	base := NewFileTree()

	base.AddFile("/home/wagoodman/some/stuff-1.txt")
	originalNode, _ := base.AddFile("/home/wagoodman/some/stuff-2-overlap.txt")
	// note: this is a file that gets overridden as a directory
	originalMore, _ := base.AddFile("/home/wagoodman/more")
	originalMoreDir, _ := base.AddDir("/home/wagoodman/moredir")

	top := NewFileTree()
	top.AddFile("/etc/redhat-release")
	// note: override /home/wagoodman/more (a file) as a directory
	top.AddFile("/home/wagoodman/more/things.txt")
	// note: we are adding a file in the upper layer which has an existing empty directory in the lower layer
	top.AddFile("/home/wagoodman/moredir/things-2.txt")
	// note: override a file in a previous layer
	newNode, _ := top.AddFile("/home/wagoodman/some/stuff-2-overlap.txt")
	top.AddFile("/home/wagoodman/some/stuff-3.txt")
	top.AddFile("/home/wagoodman/another/other-1.txt")

	ut.PushTree(base)
	ut.PushTree(top)

	if originalNode.ID() == newNode.ID() {
		t.Fatal("original and new nodes are the same (should always be different)")
	}

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("cloud not squash trees", err)
	}

	paths := squashed.AllRealPaths()
	if len(paths) != 15 {
		for _, n := range paths {
			t.Logf("   found file: %+v", n)
		}
		t.Fatalf("unexpected squashed Tree number of paths: %d : %+v", len(paths), paths)
	}

	nodes := squashed.AllFiles()
	if len(nodes) != 7 {
		for _, n := range nodes {
			t.Logf("   found node: %+v", n)
		}
		t.Fatalf("unexpected squashed Tree number of nodes: %d", len(nodes))
	}

	if originalNode.ID() == newNode.ID() {
		t.Fatal("original and new node ids changed after squash")
	}

	_, f, _ := squashed.File(newNode.RealPath)
	if f.ID() != newNode.ID() {
		t.Fatal("failed to overwrite a path in the squash Tree")
	}

	_, f, _ = base.File("/home/wagoodman/more")
	if f == nil {
		t.Fatal("base was never created")
	}

	if originalMore.ID() != f.ID() {
		t.Fatal("base path ref ID changed!")
	}

	_, f, _ = top.File("/home/wagoodman/more")
	if f != nil {
		t.Fatal("top file should have been implicitly nil but wasn't")
	}

	_, f, _ = squashed.File("/home/wagoodman/more")
	if f != nil {
		t.Fatal("file override to a dir has original properties")
	}

	_, f, _ = squashed.File("/home/wagoodman/moredir")
	if f == nil {
		t.Fatal("dir override to a dir is missing original properties")
	}
	if originalMoreDir.ID() != f.ID() {
		t.Fatal("dir override to a dir has different properties")
	}

}

func TestUnionFileTree_Squash_whiteout(t *testing.T) {
	ut := NewUnionFileTree()
	base := NewFileTree()

	base.AddFile("/some/stuff-1.txt")
	base.AddFile("/some/stuff-2.txt")
	base.AddFile("/other/things-1.txt")

	top := NewFileTree()
	top.AddFile("/some/" + file.OpaqueWhiteout)
	top.AddFile("/other/" + file.WhiteoutPrefix + "things-1.txt")

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("cloud not squash trees", err)
	}

	nodes := squashed.AllRealPaths()
	if len(nodes) != 3 {
		for _, n := range nodes {
			t.Logf("   found node: %+v", n)
		}
		t.Fatal("unexpected squashed Tree number of paths", len(nodes))
	}

	files := squashed.AllFiles()
	if len(files) != 0 {
		for _, n := range files {
			t.Logf("   found file: %+v", n)
		}
		t.Fatal("unexpected squashed Tree number of files", len(files))
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
