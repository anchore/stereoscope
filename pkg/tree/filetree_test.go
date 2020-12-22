package tree

import (
	"errors"
	"testing"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/pkg/file"
)

func TestFileTree_AddPath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home")
	fileNode, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	if len(tr.pathToFileRef) != 2 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	if tr.File(path) != fileNode {
		t.Fatal("expected pointer to the newly created fileNode")
	}
}

func TestFileTree_AddPathAndMissingAncestors(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	fileNode, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	if len(tr.pathToFileRef) != 5 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	if tr.File(path) != fileNode {
		t.Fatal("expected pointer to the newly created fileNode")
	}

	parent := file.Path("/home/wagoodman")
	child := file.Path("/home/wagoodman/awesome")

	children := tr.tree.Children(parent)

	if len(children) != 1 {
		t.Fatal("unexpected child count", len(children))
	}

	if children[0].ID() != child.ID() {
		t.Fatal("unexpected child", children[0])
	}
}

func TestFileTree_RemovePath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	_, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	err = tr.RemovePath("/home/wagoodman/awesome")
	if err != nil {
		t.Fatal("could not remote path", err)
	}

	if len(tr.tree.Nodes()) != 3 {
		t.Fatal("unexpected node count", len(tr.tree.Nodes()), tr.tree.Nodes())
	}

	if len(tr.pathToFileRef) != 3 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	if tr.File(path) != nil {
		t.Fatal("expected file to be missing")
	}

	err = tr.RemovePath("/")
	if !errors.Is(err, ErrRemovingRoot) {
		t.Fatalf("should not be able to remove root path, but the call returned err: %v", err)
	}
}

func TestFileTree_FilesByRegex(t *testing.T) {
	tr := NewFileTree()

	paths := []string{
		"/home/wagoodman/awesome/file.txt",
		"/home/wagoodman/file.txt",
		"/home/wagoodman/b-file.txt",
		"/home/wagoodman/some/deeply/nested/spot/file.txt",
		"/home/a-file.txt",
		"/home/nothing.txt",
		"/home/dir",
		"/place/example.gif",
	}

	for _, p := range paths {
		_, err := tr.AddPath(file.Path(p))
		if err != nil {
			t.Fatalf("failed to add path ('%s'): %+v", p, err)
		}
	}

	tests := []struct {
		g        string
		expected []string
		err      bool
	}{
		{
			g: "/home/wagoodman/**/file.txt",
			expected: []string{
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
			},
		},
		{
			g: "/home/wagoodman/**",
			expected: []string{
				// note: this will only find files, not dirs
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			g:        "file.txt",
			expected: []string{},
		},
		{
			g:        "*file.txt",
			expected: []string{},
		},
		{
			g: "**/*file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/a-file.txt",
			},
		},
		{
			g: "*/example.gif",
			expected: []string{
				"/place/example.gif",
			},
		},
		{
			g: "/**/file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			g: "/**/?-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			g: "/**/*-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			g: "/**/?-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			g: "**/a-file.txt",
			expected: []string{
				"/home/a-file.txt",
			},
		},
		{
			g: "/**/*.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/a-file.txt",
				"/home/nothing.txt",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.g, func(t *testing.T) {
			t.Log("PATTERN: ", test.g)
			actual, err := tr.FilesByGlob(test.g)
			if err != nil && !test.err {
				t.Fatal("failed to search by glob:", err)
			} else if err == nil && test.err {
				t.Fatalf("expected an error but did not get one")
			} else if err != nil && test.err {
				// we expected an error, nothing else matters
				return
			}

			actualSet := internal.NewStringSet()
			expectedSet := internal.NewStringSet()

			for _, f := range actual {
				actualSet.Add(string(f.Path))
			}

			for _, e := range test.expected {
				expectedSet.Add(e)
				if !actualSet.Contains(e) {
					t.Errorf("missing search hit: %s", e)
				}
			}

			for _, f := range actual {
				if !expectedSet.Contains(string(f.Path)) {
					t.Errorf("extra search hit: %+v", f)
				}
			}

		})
	}

}

func TestFileTree_Merge(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file-1.txt")

	tr2 := NewFileTree()
	tr2.AddPath("/home/wagoodman/awesome/file-2.txt")

	tr1.merge(tr2)

	for _, p := range []file.Path{"/home/wagoodman/awesome/file-1.txt", "/home/wagoodman/awesome/file-2.txt"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}
}

func TestFileTree_Merge_Overwrite(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	new, _ := tr2.AddPath("/home/wagoodman/awesome/file.txt")

	tr1.merge(tr2)

	if tr1.File("/home/wagoodman/awesome/file.txt").ID() != new.ID() {
		t.Fatalf("did not overwrite paths on merge")
	}

}

func TestFileTree_Merge_OpaqueWhiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddPath("/home/wagoodman/.wh..wh..opq")

	tr1.merge(tr2)

	for _, p := range []file.Path{"/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("expected path to be deleted: %s", p)
		}
	}

}

func TestFileTree_Merge_OpaqueWhiteout_NoLowerDirectory(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home")

	tr2 := NewFileTree()
	tr2.AddPath("/home/luhring/.wh..wh..opq")

	tr1.merge(tr2)

	for _, p := range []file.Path{"/home/luhring", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}
}

func TestFileTree_Merge_Whiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddPath("/home/wagoodman/awesome/.wh.file.txt")

	tr1.merge(tr2)

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("expected path to be deleted: %s", p)
		}
	}

}

func TestFileTree_Symlink(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home", "/another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	myHome, _ := tr.AddPath("/another/place")

	if len(tr.pathToFileRef) != 4 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	ref := tr.File("/home")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	// at this point we are NOT expecting these references to be the same... one is a link
	// and the other is a file, so the tree should be keeping track of both
	if ref.ID() == myHome.ID() {
		t.Errorf("failed to resolve to home symlink ref: %+v != %+v", ref.ID(), myHome.ID())
	}
}

func TestFileTree_Symlink_AbsoluteTarget(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home/wagoodman", "/another/place/wagoodman")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}
	myHome, _ := tr.AddPath("/another/place/wagoodman")

	ref := tr.File("/home/wagoodman")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	// at this point we are NOT expecting these references to be the same... one is a link
	// and the other is a file, so the tree should be keeping track of both
	if ref.ID() == myHome.ID() {
		t.Errorf("failed to resolve to home symlink ref: %+v != %+v", ref.ID(), myHome.ID())
	}
}

func TestFileTree_Symlink_RelativeTarget(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home/wagoodman", "../../another/place/wagoodman")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}
	myHome, _ := tr.AddPath("/another/place/wagoodman")

	ref := tr.File("/home/wagoodman")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	// at this point we are NOT expecting these references to be the same... one is a link
	// and the other is a file, so the tree should be keeping track of both
	if ref.ID() == myHome.ID() {
		t.Errorf("failed to resolve to home symlink ref: %+v != %+v", ref.ID(), myHome.ID())
	}
}

func TestFileTree_Symlink_Deadlink(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home", "/another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	if len(tr.pathToFileRef) != 2 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	ref := tr.File("/home")
	if ref == nil {
		t.Errorf("expected a ref but got none")
	}
}

func TestFileTree_Symlink_AbsoluteParent(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home", "/another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	myHome, _ := tr.AddPath("/another/place/wagoodman")

	ref := tr.File("/home/wagoodman")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	if ref.ID() != myHome.ID() {
		t.Errorf("failed to resolve to home: %+v != %+v", ref.ID(), myHome.ID())
	}
}

func TestFileTree_Symlink_RelativeParent(t *testing.T) {
	tr := NewFileTree()
	_, err := tr.AddLink("/home", "../another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}
	myHome, _ := tr.AddPath("/another/place/wagoodman")

	ref := tr.File("/home/wagoodman")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	if ref.ID() != myHome.ID() {
		t.Errorf("failed to resolve to home: %+v != %+v", ref.ID(), myHome.ID())
	}
}

func TestFileTree_Symlink_RelativeParent_AboveRoot(t *testing.T) {
	tr := NewFileTree()
	// a user cannot pop above root (will resolve to / per https://9p.io/sys/doc/lexnames.html)
	_, err := tr.AddLink("/home", "../../../../another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}
	myHome, _ := tr.AddPath("/another/place/wagoodman")

	ref := tr.File("/home/wagoodman")
	if ref == nil {
		t.Fatalf("expected a ref but got none")
	}

	if ref.ID() != myHome.ID() {
		t.Errorf("failed to resolve to home: %+v != %+v", ref.ID(), myHome.ID())
	}
}
