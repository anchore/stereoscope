package tree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
)

func TestNewCompactTree(t *testing.T) {
	tree := NewCompactTree()
	if tree == nil {
		t.Fatal("NewCompactTree returned nil")
	}
	if tree.Length() != 0 {
		t.Fatalf("expected empty tree, got %d nodes", tree.Length())
	}
}

func TestAddRoot(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/")

	id, err := tree.AddRoot("", file.TypeDirectory, ref)
	if err != nil {
		t.Fatalf("AddRoot failed: %v", err)
	}

	if id != 1 {
		t.Fatalf("expected root ID 1, got %d", id)
	}

	if tree.Length() != 1 {
		t.Fatalf("expected 1 node, got %d", tree.Length())
	}

	if tree.Root() == nil {
		t.Fatal("Root returned nil")
	}

	if tree.FileType(id) != file.TypeDirectory {
		t.Fatalf("expected directory, got %v", tree.FileType(id))
	}
}

func TestAddRootDuplicates(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/")

	_, err := tree.AddRoot("", file.TypeDirectory, ref)
	if err != nil {
		t.Fatalf("first AddRoot failed: %v", err)
	}

	_, err = tree.AddRoot("", file.TypeDirectory, ref)
	if err == nil {
		t.Error("expected error for duplicate root")
	}
}

func TestAddChild(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/")

	rootID, err := tree.AddRoot("", file.TypeDirectory, ref)
	if err != nil {
		t.Fatalf("AddRoot failed: %v", err)
	}

	childRef := file.NewFileReference("/test.txt")
	childID, err := tree.AddChild(rootID, "test.txt", file.TypeRegular, childRef, "")
	if err != nil {
		t.Fatalf("AddChild failed: %v", err)
	}

	if childID != 2 {
		t.Fatalf("expected child ID 2, got %d", childID)
	}

	if tree.Length() != 2 {
		t.Fatalf("expected 2 nodes, got %d", tree.Length())
	}

	// Check parent relationship
	if tree.GetParentID(childID) != rootID {
		t.Error("child parent is incorrect")
	}

	// Check children
	children := tree.GetChildIDs(rootID)
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}

	if children[0] != childID {
		t.Fatalf("expected child ID %d, got %d", childID, children[0])
	}

	// Check node retrieval
	child := tree.GetTreeNode(childID)
	if child == nil {
		t.Fatal("Node returned nil for child")
	}

	if tree.Name(childID) != "test.txt" {
		t.Fatalf("expected name \"test.txt\", got %q", tree.Name(childID))
	}

	if tree.FileType(childID) != file.TypeRegular {
		t.Fatalf("expected regular file, got %v", tree.FileType(childID))
	}
}

func TestAddChildInvalidParent(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/test.txt")

	_, err := tree.AddChild(999, "test.txt", file.TypeRegular, ref, "")
	if err == nil {
		t.Error("expected error for invalid parent ID")
	}
}

func TestAddDir(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/usr/local/bin")

	id, err := tree.AddDir("/usr/local/bin", ref)
	if err != nil {
		t.Fatalf("AddDir failed: %v", err)
	}

	if !tree.HasPath("/usr/local/bin") {
		t.Error("HasPath returned false for path")
	}

	if tree.ID("/usr/local/bin") != id {
		t.Error("ID returned wrong value for path")
	}

	// Verify intermediate directories were created
	if !tree.HasPath("/") {
		t.Error("root not created")
	}
	if !tree.HasPath("/usr") {
		t.Error("/usr not created")
	}
	if !tree.HasPath("/usr/local") {
		t.Error("/usr/local not created")
	}
}

func TestAddDirDuplicate(t *testing.T) {
	tree := NewCompactTree()

	ref1 := file.NewFileReference("/test")
	id1, err := tree.AddDir("/test", ref1)
	if err != nil {
		t.Fatalf("first AddDir failed: %v", err)
	}

	ref2 := file.NewFileReference("/test")
	id2, err := tree.AddDir("/test", ref2)
	if err != nil {
		t.Fatalf("second AddDir failed: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("expected same ID %d, got %d", id1, id2)
	}

	if tree.Length() != 2 {
		t.Fatalf("expected 2 nodes, got %d", tree.Length())
	}
}

func TestAddFile(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/test/file.txt")

	id, err := tree.AddFile("/test/file.txt", file.TypeRegular, ref, "")
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	if !tree.HasPath("/test/file.txt") {
		t.Error("HasPath returned false for file")
	}

	if tree.FileType(id) != file.TypeRegular {
		t.Fatalf("expected regular file, got %v", tree.FileType(id))
	}

	// Verify parent directory was created
	if !tree.HasPath("/test") {
		t.Error("parent directory not created")
	}

	if tree.FileType(tree.ID("/test")) != file.TypeDirectory {
		t.Error("parent is not a directory")
	}
}

func TestAddSymlink(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/test/link")

	id, err := tree.AddFile("/test/link", file.TypeSymLink, ref, "/target.txt")
	if err != nil {
		t.Fatalf("AddFile (symlink) failed: %v", err)
	}

	if tree.FileType(id) != file.TypeSymLink {
		t.Fatalf("expected symlink, got %v", tree.FileType(id))
	}

	if tree.LinkPath(id) != "/target.txt" {
		t.Fatalf("expected link path \"/target.txt\", got %q", tree.LinkPath(id))
	}
}

func TestDeepTree(t *testing.T) {
	tree := NewCompactTree()

	// Create a deep hierarchy: /a/b/c/d/e/f
	path := "/a/b/c/d/e/f"
	tree.AddDir(path, nil)

	for _, p := range []string{"/", "/a", "/a/b", "/a/b/c", "/a/b/c/d", "/a/b/c/d/e", "/a/b/c/d/e/f"} {
		if !tree.HasPath(p) {
			t.Errorf("expected path %q to exist", p)
		}
	}

	if tree.Length() != 7 {
		t.Fatalf("expected 7 nodes, got %d", tree.Length())
	}
}

func TestStringPool(t *testing.T) {
	tree := NewCompactTree()
	tree.AddRoot("", file.TypeDirectory, nil)
	tree.AddChild(1, "file1.txt", file.TypeRegular, nil, "")
	tree.AddChild(1, "file1.txt", file.TypeRegular, nil, "")

	// Same string should use same index
	node1 := tree.GetTreeNode(2)
	node2 := tree.GetTreeNode(3)

	if node1.nameIdx != node2.nameIdx {
		t.Error("same string should have same index in pool")
	}

	// Verify pool has only two entries ("" and "file1.txt")
	if tree.stringPool.Len() != 2 {
		t.Fatalf("expected 2 strings in pool, got %d", tree.stringPool.Len())
	}
}

func TestReferencePool(t *testing.T) {
	tree := NewCompactTree()
	ref := file.NewFileReference("/test")

	tree.AddRoot("", file.TypeDirectory, ref)
	tree.AddChild(1, "file1", file.TypeRegular, ref, "")
	tree.AddChild(1, "file2", file.TypeRegular, ref, "")

	// Same reference should use same index
	node1 := tree.GetTreeNode(2)
	node2 := tree.GetTreeNode(3)

	if node1.refIdx != node2.refIdx {
		t.Error("same reference should have same index in pool")
	}

	// Verify pool has only one entry
	if tree.refPool.Len() != 1 {
		t.Fatalf("expected 1 reference in pool, got %d", tree.refPool.Len())
	}
}

func BenchmarkCompactTreeAdd(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tree := NewCompactTree()
		for j := 0; j < 10000; j++ {
			tree.AddDir("/nested/deep/path", nil)
		}
	}
}
