package filetree

//
//import (
//	"testing"
//
//	"github.com/anchore/stereoscope/pkg/file"
//)
//
//func TestUnionFileTree_Squash(t *testing.T) {
//	ut := NewUnionFileTree()
//	base := NewFileTree()
//
//	base.AddFile("/home/wagoodman/some/stuff-1.txt")
//	originalNode, _ := base.AddFile("/home/wagoodman/some/stuff-2-overlap.txt")
//	originalMore, _ := base.AddFile("/home/wagoodman/more")
//
//	top := NewFileTree()
//	top.AddFile("/etc/redhat-release")
//	top.AddFile("/home/wagoodman/more/things.txt")
//	newNode, _ := top.AddFile("/home/wagoodman/some/stuff-2-overlap.txt")
//	top.AddFile("/home/wagoodman/some/stuff-3.txt")
//	top.AddFile("/home/wagoodman/another/other-1.txt")
//
//	ut.PushTree(base)
//	ut.PushTree(top)
//
//	if originalNode.ID() == newNode.ID() {
//		t.Fatal("original and new nodes are the same (should always be different)")
//	}
//
//	squashed, err := ut.Squash()
//	if err != nil {
//		t.Fatal("cloud not squash trees", err)
//	}
//
//	paths := squashed.AllPaths()
//	if len(paths) != 13 {
//		t.Fatalf("unexpected squashed Tree number of paths: %d : %+v", len(paths), paths)
//	}
//
//	// this does not include paths with nil file refs
//	nodes := squashed.AllFiles()
//	if len(nodes) != 8 { // data nodes + root /
//		t.Fatalf("unexpected squashed Tree number of nodes: %d : %+v", len(nodes), nodes)
//	}
//
//	if originalNode.ID() == newNode.ID() {
//		t.Fatal("original and new node ids changed after squash")
//	}
//
//	_, _, f, err := squashed.File(newNode.Path, false)
//	if f.ID() != newNode.ID() {
//		t.Fatal("failed to overwrite a path in the squash Tree")
//	}
//
//	_, _, f, err = base.File("/home/wagoodman/more", false)
//	if f == nil {
//		t.Fatal("base was never created")
//	}
//
//	if originalMore.ID() != f.ID() {
//		t.Fatal("base path ref ID changed!")
//	}
//
//	_, _, f, err = top.File("/home/wagoodman/more", false)
//	if f != nil {
//		t.Fatal("top file should have been implicitly nil but wasn't")
//	}
//
//	_, _, f, err = squashed.File("/home/wagoodman/more", false)
//	if f == nil {
//		t.Fatal("implicit file was copied to squash")
//	}
//
//}
//
//func TestUnionFileTree_Squash_whiteout(t *testing.T) {
//	ut := NewUnionFileTree()
//	base := NewFileTree()
//
//	base.AddFile("/some/stuff-1.txt")
//	base.AddFile("/some/stuff-2.txt")
//	base.AddFile("/other/things-1.txt")
//
//	top := NewFileTree()
//	top.AddFile("/some/" + file.OpaqueWhiteout)
//	top.AddFile("/other/" + file.WhiteoutPrefix + "things-1.txt")
//
//	ut.PushTree(base)
//	ut.PushTree(top)
//
//	squashed, err := ut.Squash()
//	if err != nil {
//		t.Fatal("cloud not squash trees", err)
//	}
//
//	nodes := squashed.AllPaths()
//	if len(nodes) != 3 {
//		t.Fatal("unexpected squashed Tree number of paths", len(nodes))
//	}
//
//	files := squashed.AllFiles()
//	if len(files) != 1 { // just the root node
//		t.Fatal("unexpected squashed Tree number of files", len(files))
//	}
//
//	expectedPaths := []string{
//		"/",
//		"/some",
//		"/other",
//	}
//
//	for _, path := range expectedPaths {
//		if !squashed.HasPath(file.Path(path)) {
//			t.Errorf("expected '%v' but not found", path)
//		}
//	}
//
//}
