package filetree

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-test/deep"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
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

func TestDFS_WalkAll_SkipsLinkCycles(t *testing.T) {
	tr := New()

	for _, p := range []file.Path{
		"/usr/bin/a-before",
		"/usr/bin/zz-after",
	} {
		_, err := tr.AddFile(p)
		if err != nil {
			t.Fatalf("failed to add path %q: %+v", p, err)
		}
	}

	_, err := tr.AddSymLink("/usr/bin/xz", "/usr/bin/xzcat")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	_, err = tr.AddSymLink("/usr/bin/xzcat", "/usr/bin/xz")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	visited := file.NewPathSet()
	walker := NewDepthFirstPathWalker(tr, func(path file.Path, _ filenode.FileNode) error {
		visited.Add(path)
		return nil
	}, nil)

	if err := walker.WalkAll(); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}

	for _, p := range []file.Path{
		"/usr/bin/a-before",
		"/usr/bin/zz-after",
	} {
		if !visited.Contains(p) {
			t.Errorf("did not visit path %q", p)
		}
	}
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

// linkCycleTree builds a tree with a two-node symlink cycle plus regular files on either side of it.
func linkCycleTree(t *testing.T, cycleA, cycleB file.Path, regular ...file.Path) *FileTree {
	t.Helper()
	tr := New()
	for _, p := range regular {
		if _, err := tr.AddFile(p); err != nil {
			t.Fatalf("failed to add path %q: %+v", p, err)
		}
	}
	if _, err := tr.AddSymLink(cycleA, cycleB); err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	if _, err := tr.AddSymLink(cycleB, cycleA); err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}
	return tr
}

func walkCollect(t *testing.T, tr *FileTree, from file.Path) (file.PathSet, error) {
	t.Helper()
	visited := file.NewPathSet()
	w := NewDepthFirstPathWalker(tr, func(p file.Path, _ filenode.FileNode) error {
		visited.Add(p)
		return nil
	}, nil)
	_, _, err := w.Walk(from)
	return visited, err
}

// a link cycle must not hide files that sort before or after it in traversal order, and the walk must
// return cleanly rather than dereferencing the unresolved cycle node.
func TestDFS_WalkAll_SkipsLinkCycleAsLastEntry(t *testing.T) {
	tr := linkCycleTree(t, "/usr/bin/zz-x", "/usr/bin/zz-y", "/usr/bin/a-before", "/usr/bin/zz-after")

	visited, err := walkCollect(t, tr, "/")
	if err != nil {
		t.Fatalf("could not walk: %+v", err)
	}
	for _, p := range []file.Path{"/usr/bin/a-before", "/usr/bin/zz-after"} {
		if !visited.Contains(p) {
			t.Errorf("did not visit path %q", p)
		}
	}
}

// walking from the cycle itself means the very first pop is unresolvable and the stack drains immediately,
// so Walk must return a nil FileNode alongside a nil error rather than dereferencing it.
func TestDFS_Walk_StartingAtLinkCycle(t *testing.T) {
	tr := linkCycleTree(t, "/usr/bin/xz", "/usr/bin/xzcat", "/usr/bin/a-before")

	w := NewDepthFirstPathWalker(tr, func(file.Path, filenode.FileNode) error {
		return nil
	}, nil)

	_, node, err := w.Walk("/usr/bin/xz")
	if err != nil {
		t.Fatalf("could not walk: %+v", err)
	}
	if node != nil {
		t.Errorf("expected a nil FileNode for an unresolved final path, got: %+v", node)
	}
}

func TestDFS_WalkAll_SkipsSelfReferentialLink(t *testing.T) {
	tr := New()
	if _, err := tr.AddSymLink("/a", "/a"); err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	if _, err := walkCollect(t, tr, "/"); err != nil {
		t.Fatalf("could not walk: %+v", err)
	}
}

// a link chain too deep to follow denies a scan exactly like a cycle does, so it must be skipped too.
func TestDFS_WalkAll_SkipsExcessiveLinkDepth(t *testing.T) {
	tr := New()
	for _, p := range []file.Path{"/usr/bin/a-before", "/usr/bin/zz-after"} {
		if _, err := tr.AddFile(p); err != nil {
			t.Fatalf("failed to add path %q: %+v", p, err)
		}
	}
	for i := 0; i < maxLinkResolutionDepth+20; i++ {
		from := file.Path(fmt.Sprintf("/usr/bin/l%04d", i))
		to := file.Path(fmt.Sprintf("/usr/bin/l%04d", i+1))
		if _, err := tr.AddSymLink(from, to); err != nil {
			t.Fatalf("could not setup link: %+v", err)
		}
	}

	visited, err := walkCollect(t, tr, "/")
	if err != nil {
		t.Fatalf("could not walk: %+v", err)
	}
	for _, p := range []file.Path{"/usr/bin/a-before", "/usr/bin/zz-after"} {
		if !visited.Contains(p) {
			t.Errorf("did not visit path %q", p)
		}
	}
}

// a visitor error must still abort the walk; the new "missing node" skip logic only swallows
// unresolved paths, not errors returned from the visitor itself.
func TestDFS_Walk_VisitorErrorsAreNotSwallowed(t *testing.T) {
	tr := linkCycleTree(t, "/usr/bin/xz", "/usr/bin/xzcat", "/usr/bin/a-before")

	expected := errors.New("visitor blew up")
	w := NewDepthFirstPathWalker(tr, func(p file.Path, _ filenode.FileNode) error {
		if p == "/usr/bin/a-before" {
			return expected
		}
		return nil
	}, nil)

	if _, _, err := w.Walk("/"); !errors.Is(err, expected) {
		t.Fatalf("expected the visitor error to propagate, got: %+v", err)
	}
}
