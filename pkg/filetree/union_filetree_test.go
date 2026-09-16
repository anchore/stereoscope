package filetree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
)

func TestUnionFileTree_Squash(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/home/wagoodman/some/stuff-1.txt")
	originalNode, _ := base.AddFile("/home/wagoodman/some/stuff-2-overlap.txt")
	// note: this is a file that gets overridden as a directory
	originalMore, _ := base.AddFile("/home/wagoodman/more")
	originalMoreDir, _ := base.AddDir("/home/wagoodman/moredir")

	top := New()
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
			t.Logf("   found Node: %+v", n)
		}
		t.Fatalf("unexpected squashed Tree number of nodes: %d", len(nodes))
	}

	if originalNode.ID() == newNode.ID() {
		t.Fatal("original and new Node ids changed after squash")
	}

	_, f, _ := squashed.File(newNode.RealPath)
	if f.ID() != newNode.ID() {
		t.Fatal("failed to overwrite a path in the squash Tree")
	}

	_, f, _ = base.File("/home/wagoodman/more")
	if f == nil || f.Reference == nil {
		t.Fatal("base was never created")
	}

	if originalMore.ID() != f.ID() {
		t.Fatal("base path ref ID changed!")
	}

	_, f, _ = top.File("/home/wagoodman/more")
	if f.Reference != nil {
		t.Fatal("top file should have been implicitly nil but wasn't")
	}

	_, f, _ = squashed.File("/home/wagoodman/more")
	if f.Reference != nil {
		t.Fatal("file override to a dir has original properties")
	}

	_, f, _ = squashed.File("/home/wagoodman/moredir")
	if f == nil || f.Reference == nil {
		t.Fatal("dir override to a dir is missing original properties")
	}
	if originalMoreDir.ID() != f.ID() {
		t.Fatal("dir override to a dir has different properties")
	}

}

func TestUnionFileTree_Squash_bareWhiteout(t *testing.T) {
	// a bare .wh. (the whiteout prefix with no name after it) is not a valid whiteout
	// marker. If it is treated as one, UnWhiteoutPath resolves to the parent directory
	// itself and squashing deletes the whole parent instead of a single named sibling.
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/etc/passwd")
	base.AddFile("/etc/hostname")

	top := New()
	top.AddFile("/etc/" + file.WhiteoutPrefix)

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("could not squash trees", err)
	}

	if !squashed.HasPath(file.Path("/etc/passwd")) {
		t.Error("expected /etc/passwd to survive a bare .wh. but it was deleted")
	}
	if !squashed.HasPath(file.Path("/etc/hostname")) {
		t.Error("expected /etc/hostname to survive a bare .wh. but it was deleted")
	}
}

func TestUnionFileTree_Squash_whiteout(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/some/stuff-1.txt")
	base.AddFile("/some/stuff-2.txt")
	base.AddFile("/other/things-1.txt")

	top := New()
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
			t.Logf("   found Node: %+v", n)
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

// an opaque directory in an upper layer sitting over a malformed link in a lower layer must not cost the whole
// image. Merge runs RemoveChildPaths against the lower tree before grafting the upper node, and that resolves
// through the link. It must still delete the link's own children.
func TestUnionFileTree_Squash_opaqueDirectoryOverLinkCycle(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	if _, err := base.AddFile("/usr/bin/keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.AddSymLink("/x", "/y"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.AddSymLink("/y", "/x"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.AddFile("/x/gone.txt"); err != nil {
		t.Fatal(err)
	}

	top := New()
	if _, err := top.AddFile("/x/" + file.OpaqueWhiteout); err != nil {
		t.Fatal(err)
	}

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatalf("could not squash trees: %+v", err)
	}

	// the unrelated file must survive the malformed link entirely
	if !squashed.HasPath("/usr/bin/keep") {
		t.Errorf("expected '/usr/bin/keep' to survive the squash but it was not found")
	}

	// ...and the opaque directory must still delete what it covers
	if squashed.HasPath("/x/gone.txt") {
		t.Errorf("opaque directory did not clear the lower tree; real paths are %v", squashed.AllRealPaths())
	}
}

// the opaque marker is the file named exactly ".wh..wh..opq". An entry that merely starts with it is an
// ordinary whiteout of the sibling that follows the ".wh." prefix; treating it as opaque resolves to the
// parent directory and deletes the entire parent subtree.
func TestUnionFileTree_Squash_opaquePrefixIsNotOpaqueMarker(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/etc/passwd")
	base.AddFile("/etc/hostname")

	top := New()
	top.AddFile("/etc/" + file.OpaqueWhiteout + "X")

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("could not squash trees", err)
	}

	if !squashed.HasPath(file.Path("/etc/passwd")) {
		t.Error("expected /etc/passwd to survive a non-opaque marker but it was deleted")
	}
	if !squashed.HasPath(file.Path("/etc/hostname")) {
		t.Error("expected /etc/hostname to survive a non-opaque marker but it was deleted")
	}
}

// the same mechanism at the image root resolves to "/", which RemovePath refuses, aborting the whole squash.
func TestUnionFileTree_Squash_opaquePrefixAtRoot(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/etc/passwd")

	top := New()
	top.AddFile("/" + file.OpaqueWhiteout + "X")

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("could not squash trees", err)
	}

	if !squashed.HasPath(file.Path("/etc/passwd")) {
		t.Error("expected /etc/passwd to survive a root level non-opaque marker but it was deleted")
	}
}

// whiteout entries are changeset metadata and never materialize as files in an applied rootfs, but the
// lowest tree is copied rather than merged, so nothing else would drop them.
func TestUnionFileTree_Squash_whiteoutInLowestLayer(t *testing.T) {
	ut := NewUnionFileTree()
	base := New()

	base.AddFile("/etc/passwd")
	base.AddFile("/etc/" + file.WhiteoutPrefix + "shadow")
	base.AddFile("/etc/" + file.OpaqueWhiteout)

	top := New()
	top.AddFile("/etc/hostname")

	ut.PushTree(base)
	ut.PushTree(top)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("could not squash trees", err)
	}

	if squashed.HasPath(file.Path("/etc/" + file.WhiteoutPrefix + "shadow")) {
		t.Error("expected the whiteout marker from the lowest layer to be stripped")
	}
	if squashed.HasPath(file.Path("/etc/" + file.OpaqueWhiteout)) {
		t.Error("expected the opaque marker from the lowest layer to be stripped")
	}
	if !squashed.HasPath(file.Path("/etc/passwd")) {
		t.Error("expected /etc/passwd to survive")
	}
}

// a single tree is squashed by copy, which must still drop the whiteout markers it carries.
func TestUnionFileTree_Squash_whiteoutInOnlyLayer(t *testing.T) {
	ut := NewUnionFileTree()
	only := New()

	only.AddFile("/etc/passwd")
	only.AddFile("/etc/" + file.WhiteoutPrefix + "shadow")

	ut.PushTree(only)

	squashed, err := ut.Squash()
	if err != nil {
		t.Fatal("could not squash trees", err)
	}

	if squashed.HasPath(file.Path("/etc/" + file.WhiteoutPrefix + "shadow")) {
		t.Error("expected the whiteout marker from the only layer to be stripped")
	}
	if !squashed.HasPath(file.Path("/etc/passwd")) {
		t.Error("expected /etc/passwd to survive")
	}
}
