package filetree

import (
	"errors"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/go-test/deep"
	"strings"
	"testing"
)

func dfsTestTree(t *testing.T) (*FileTree, map[string]*file.Reference) {
	tr := New()

	possiblePaths := make(map[string]*file.Reference)

	files := []string{
		"/hard-linked-dest/something/b-.gif",
		"/home/a-file.txt",
		"/home/nothing.txt",
		"/home/wagoodman/awesome/file.txt",
		"/home/wagoodman/b-file.txt",
		"/home/wagoodman/file.txt",
		"/home/wagoodman/some/deeply/nested/spot/file.txt",
		"/place/example.gif",
		"/sym-linked-dest/another/a-.gif",
	}

	dirs := []string{
		"/home/dir",
	}

	// all leaves that are files
	for _, p := range files {
		ref, err := tr.AddFile(file.Path(p))
		if err != nil {
			t.Fatalf("failed to add path ('%s'): %+v", p, err)
		}
		possiblePaths[p] = ref
	}

	// all leaves that are directories
	for _, p := range dirs {
		ref, err := tr.AddDir(file.Path(p))
		if err != nil {
			t.Fatalf("failed to add path ('%s'): %+v", p, err)
		}
		possiblePaths[p] = ref
	}

	// absolute symlink
	_, err := tr.AddSymLink("/home/elsewhere/symlink", "/sym-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	// since we are following base links on walk, we should NOT expect the symlink ref at the link destination
	possiblePaths["/home/elsewhere/symlink"] = nil
	possiblePaths["/home/elsewhere/symlink/another/a-.gif"] = possiblePaths["/sym-linked-dest/another/a-.gif"]

	// relative symlink
	_, err = tr.AddSymLink("/home/again/symlink", "../../../sym-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	// since we are following base links on walk, we should NOT expect the symlink ref at the link destination
	possiblePaths["/home/again/symlink"] = nil
	possiblePaths["/home/again/symlink/another/a-.gif"] = possiblePaths["/sym-linked-dest/another/a-.gif"]

	// dead symlink (dir)
	ref, err := tr.AddSymLink("/home/again/deadsymlink", "../ijustdontexist")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	possiblePaths["/home/again/deadsymlink"] = ref

	// dead symlink (to txt)
	ref, err = tr.AddSymLink("/home/again/dead.jpg", "../ialsojustdontexist")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	possiblePaths["/home/again/dead.jpg"] = ref

	// hardlink
	ref, err = tr.AddHardLink("/home/elsewhere/hardlink", "/hard-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	// since we are following base links on walk, we should NOT expect the symlink ref at the link destination
	possiblePaths["/home/elsewhere/hardlink"] = nil
	possiblePaths["/home/elsewhere/hardlink/something/b-.gif"] = possiblePaths["/hard-linked-dest/something/b-.gif"]

	// add all paths which should not have references
	for p := range possiblePaths {
		for _, c := range file.Path(p).ConstituentPaths() {
			if _, exists := possiblePaths[string(c)]; !exists {
				possiblePaths[string(c)] = nil
			}
		}
	}

	return tr, possiblePaths
}

func TestDFS_WalkAll(t *testing.T) {
	tr, possiblePaths := dfsTestTree(t)

	actualPaths := make(map[string]*file.Reference, 0)
	visitor := func(path file.Path, node filenode.FileNode) error {
		actualPaths[string(path)] = node.Reference
		return nil
	}

	walker := NewDepthFirstPathWalker(tr, visitor, nil)
	if err := walker.WalkAll(); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}

	assertExpectedTraversal(t, possiblePaths, actualPaths)
}

func TestDFS_WalkAll_EarlyTermination(t *testing.T) {
	tr, possiblePaths := dfsTestTree(t)

	// delete paths we aren't expecting
	tailPaths := []string{
		"/place/example.gif",
		"/sym-linked-dest/another/a-.gif",
	}
	for _, p := range tailPaths {
		for _, c := range file.Path(p).ConstituentPaths() {
			if c == "/" {
				continue
			}
			delete(possiblePaths, string(c))
		}
		delete(possiblePaths, p)
	}

	// start the test

	actualPaths := make(map[string]*file.Reference, 0)
	visitor := func(path file.Path, node filenode.FileNode) error {
		actualPaths[string(path)] = node.Reference
		return nil
	}

	conditions := WalkConditions{
		ShouldTerminate: func(p file.Path, fn filenode.FileNode) bool {
			// the first Node after /home/wagoodman
			if p == "/place" {
				return true
			}
			return false
		},
	}

	walker := NewDepthFirstPathWalker(tr, visitor, &conditions)
	if err := walker.WalkAll(); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}

	assertExpectedTraversal(t, possiblePaths, actualPaths)
}

func TestDFS_WalkAll_ConditionalVisit(t *testing.T) {
	tr, possiblePaths := dfsTestTree(t)

	// delete paths we aren't expecting
	for p := range possiblePaths {
		if !strings.Contains(p, "/home/wagoodman") {
			delete(possiblePaths, p)
		}
	}

	// start the test

	actualPaths := make(map[string]*file.Reference, 0)
	visitor := func(path file.Path, node filenode.FileNode) error {
		actualPaths[string(path)] = node.Reference
		return nil
	}

	conditions := WalkConditions{
		ShouldVisit: func(p file.Path, fn filenode.FileNode) bool {
			if strings.Contains(string(p), "/home/wagoodman") {
				return true
			}
			return false
		},
	}

	walker := NewDepthFirstPathWalker(tr, visitor, &conditions)
	if err := walker.WalkAll(); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}

	assertExpectedTraversal(t, possiblePaths, actualPaths)
}

func TestDFS_WalkAll_ConditionalBranchPruning(t *testing.T) {
	tr, possiblePaths := dfsTestTree(t)

	// delete paths we aren't expecting
	for p := range possiblePaths {
		if !strings.Contains(p, "/home") && p != "/" && p != "/place" && p != "/sym-linked-dest" && p != "/hard-linked-dest" {
			delete(possiblePaths, p)
		}
	}

	// start the test

	actualPaths := make(map[string]*file.Reference, 0)
	visitor := func(path file.Path, node filenode.FileNode) error {
		actualPaths[string(path)] = node.Reference
		return nil
	}

	conditions := WalkConditions{
		ShouldContinueBranch: func(p file.Path, fn filenode.FileNode) bool {
			if p == "/" || strings.Contains(string(p), "/home") {
				return true
			}
			return false
		},
	}

	walker := NewDepthFirstPathWalker(tr, visitor, &conditions)
	if err := walker.WalkAll(); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}

	assertExpectedTraversal(t, possiblePaths, actualPaths)
}

func TestDFS_WalkAll_MaxDirDepthTerminatesTraversal(t *testing.T) {
	tr := New()

	possiblePaths := make(map[string]*file.Reference)

	// absolute symlink
	_, err := tr.AddSymLink("/home/wagoodman", "/home")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	// since we are following base links on walk, we should NOT expect the symlink ref at the link destination
	possiblePaths["/home/wagoodman"] = nil
	possiblePaths["/home"] = nil

	// start the test

	actualMaxDepth := -1
	shouldTerminate := func(path file.Path, node filenode.FileNode) bool {
		if actualMaxDepth > maxDirDepth*2 {
			// test stop gap
			t.Fatalf("did not prevent max dir depth traversal")
			return true
		}
		return false
	}

	visitor := func(path file.Path, node filenode.FileNode) error {
		actualMaxDepth = strings.Count(string(path.Normalize()), file.DirSeparator)
		return nil
	}

	walker := NewDepthFirstPathWalker(tr, visitor, &WalkConditions{
		ShouldTerminate: shouldTerminate,
	})
	if err = walker.WalkAll(); !errors.Is(err, ErrMaxTraversalDepth) {
		t.Fatalf("expected max traversal error, but got another error instead: %+v", err)
	} else if err == nil {
		t.Fatalf("expected max traversal error, but got none")
	}

	if actualMaxDepth == -1 || actualMaxDepth > maxDirDepth {
		t.Fatalf("never traversed or went above allowable threshold")
	}

}

func assertExpectedTraversal(t *testing.T, expected, actual map[string]*file.Reference) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("Did not traverse all nodes (expected %d, got %d)", len(expected), len(actual))
	}

	for _, d := range deep.Equal(expected, actual) {
		t.Errorf("   diff: %s", d)
	}
}
