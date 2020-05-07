package file

import "testing"

func TestPath_Normalize(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Trim Right Whitespace",
			path:     "/some/path ",
			expected: "/some/path",
		},
		{
			name:     "Trim Left Whitespace",
			path:     "   /some/path ",
			expected: "/some/path",
		},
		{
			name:     "Trim extra slashes",
			path:     "/some/path////",
			expected: "/some/path",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Path(c.path).Normalize()
			expected := Path(c.expected)
			if got != expected {
				t.Errorf("Didn't normalize correctly ('%v' != '%v')", got, expected)
			}
		})
	}
}

func TestPath_AllPaths(t *testing.T) {
	path := Path("/some/path/to/a/file.txt")
	expected := []Path{
		Path("/"),
		Path("/some"),
		Path("/some/path"),
		Path("/some/path/to"),
		Path("/some/path/to/a"),
		Path("/some/path/to/a/file.txt"),
	}

	paths := path.AllPaths()
	if len(paths) != len(expected) {
		t.Fatalf("unexpected number of parent paths (%+v!=%v): %+v", len(paths), len(expected), paths)
	}

	for idx := range paths {
		if paths[idx] != expected[idx] {
			t.Errorf("unexpected path ('%v' != '%v')", paths[idx], expected[idx])
		}
	}
}

func TestPath_Sanitize_ID(t *testing.T) {
	patha := Path("/some/path/to/a")
	pathb := Path("/some/path/to/a/")

	if patha.ID() != pathb.ID() {
		t.Fatalf("paths not equal: '%+v'!='%+v'", patha, pathb)
	}
}

func TestPath_ParentPath(t *testing.T) {
	path := Path("/some/path/to/a/file.txt")
	expected := Path("/some/path/to/a")

	actual, err := path.ParentPath()
	if err != nil {
		t.Fatal("no parent path", err)
	}
	if expected != actual {
		t.Fatalf("bad parent path: expected '%+v', got '%+v'", expected, actual)
	}
}

func TestPath_ParentPath_Root(t *testing.T) {
	path := Path("/home")

	parent, err := path.ParentPath()
	if err != nil {
		t.Fatal("expected /home to have parent path:", err)
	}
	if parent.ID() != Path("/").ID() {
		t.Fatalf("expected /home parent to be / , got '%v':", parent)
	}

	path = Path("/")

	parent, err = path.ParentPath()
	if err == nil {
		t.Fatalf("expected no parent path, got one: '%+v'", parent)
	}
}

func TestPath_Whiteout(t *testing.T) {
	path := Path("/some/path/to/.wh.afile")

	if !path.IsWhiteout() {
		t.Fatal("path should be a whiteout")
	}

	path = Path("/some/path/to/.wh..wh..opq")

	if !path.IsWhiteout() {
		t.Fatal("path should be a whiteout")
	}
}

func TestPath_UnWhiteoutPath(t *testing.T) {
	path := Path("/some/path/to/.wh..wh..opq")

	newPath, err := path.UnWhiteoutPath()
	if err != nil {
		t.Fatal("error while unwhiteing out", err)
	}
	if newPath != Path("/some/path/to") {
		t.Fatal("path should be a whiteout")
	}

	path = "/some/path/to/.wh.somefile.txt"

	newPath, err = path.UnWhiteoutPath()
	if err != nil {
		t.Fatal("error while unwhiteing out", err)
	}
	if newPath != "/some/path/to/somefile.txt" {
		t.Fatal("path should be a whiteout")
	}
}
