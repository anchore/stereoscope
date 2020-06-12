package tree

import (
	"testing"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/pkg/file"
)

func TestFileTree_AddPath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	fileNode, err := tr.AddPath(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	if len(tr.pathToFileRef) != 5 {
		t.Fatal("unexpected file count", len(tr.pathToFileRef))
	}

	if *tr.File(path) != fileNode {
		t.Fatal("expected pointed to the newly created fileNode")
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
	}{
		{
			g: "/home/wagoodman/**/file.txt",
			expected: []string{
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/wagoodman/awesome/file.txt",
			},
		},
		{
			g: "/home/wagoodman/**",
			expected: []string{
				"/home/wagoodman/awesome",
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some",
				"/home/wagoodman/some/deeply",
				"/home/wagoodman/some/deeply/nested",
				"/home/wagoodman/some/deeply/nested/spot",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			g:        "file.txt",
			expected: []string{},
		},
		{
			g: "*file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/a-file.txt",
			},
		},
		{
			g: "*/file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			g: "*/?-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			g: "*/*-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			g: "**/?-file.txt",
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
			g: "*.txt",
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
			if err != nil {
				t.Fatal("failed to search by glob:", err)
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

func TestFileTree_Merge_Overwite(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	new, _ := tr2.AddPath("/home/wagoodman/awesome/file.txt")

	tr1.Merge(tr2)

	if tr1.File("/home/wagoodman/awesome/file.txt").ID() != new.ID() {
		t.Fatalf("did not overwrite paths on merge")
	}

}

func TestFileTree_Merge_OpaqueWhiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddPath("/home/wagoodman/.wh..wh..opq")

	tr1.Merge(tr2)

	for _, p := range []file.Path{"/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("missing expected path to be deleted: %s", p)
		}
	}

}

func TestFileTree_Merge_Whiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddPath("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddPath("/home/wagoodman/awesome/.wh.file.txt")

	tr1.Merge(tr2)

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("missing expected path to be deleted: %s", p)
		}
	}

}
