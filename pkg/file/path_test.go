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
			path:     "/some/Path ",
			expected: "/some/Path",
		},
		{
			name:     "Trim Left Whitespace",
			path:     "   /some/Path ",
			expected: "/some/Path",
		},
		{
			name:     "Trim extra slashes",
			path:     "/some/Path////",
			expected: "/some/Path",
		},
		{
			name:     "Special case: /",
			path:     "/",
			expected: "/",
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
	path := Path("/some/Path/to/a/file.txt")
	expected := []Path{
		Path("/"),
		Path("/some"),
		Path("/some/Path"),
		Path("/some/Path/to"),
		Path("/some/Path/to/a"),
		Path("/some/Path/to/a/file.txt"),
	}

	paths := path.AllPaths()
	if len(paths) != len(expected) {
		t.Fatalf("unexpected number of parent paths (%+v!=%v): %+v", len(paths), len(expected), paths)
	}

	for idx := range paths {
		if paths[idx] != expected[idx] {
			t.Errorf("unexpected Path ('%v' != '%v')", paths[idx], expected[idx])
		}
	}
}

func TestPath_Sanitize_ID(t *testing.T) {
	patha := Path("/some/Path/to/a")
	pathb := Path("/some/Path/to/a/")

	if patha.ID() != pathb.ID() {
		t.Fatalf("paths not equal: '%+v'!='%+v'", patha, pathb)
	}
}

func TestPath_ParentPath(t *testing.T) {
	path := Path("/some/Path/to/a/file.txt")
	expected := Path("/some/Path/to/a")

	actual, err := path.ParentPath()
	if err != nil {
		t.Fatal("no parent Path", err)
	}
	if expected != actual {
		t.Fatalf("bad parent Path: expected '%+v', got '%+v'", expected, actual)
	}
}

func TestPath_ParentPath_Root(t *testing.T) {
	path := Path("/home")

	parent, err := path.ParentPath()
	if err != nil {
		t.Fatal("expected /home to have parent Path:", err)
	}
	if parent.ID() != Path("/").ID() {
		t.Fatalf("expected /home parent to be / , got '%v':", parent)
	}

	path = Path("/")

	parent, err = path.ParentPath()
	if err == nil {
		t.Fatalf("expected no parent Path, got one: '%+v'", parent)
	}
}

func TestPath_Whiteout(t *testing.T) {
	path := Path("/some/Path/to/.wh.afile")

	if !path.IsWhiteout() {
		t.Fatal("Path should be a whiteout")
	}

	path = Path("/some/Path/to/.wh..wh..opq")

	if !path.IsWhiteout() {
		t.Fatal("Path should be a whiteout")
	}
}

func TestPath_UnWhiteoutPath(t *testing.T) {
	path := Path("/some/Path/to/.wh..wh..opq")

	newPath, err := path.UnWhiteoutPath()
	if err != nil {
		t.Fatal("error while unwhiteing out", err)
	}
	if newPath != Path("/some/Path/to") {
		t.Fatal("Path should be a whiteout")
	}

	path = "/some/Path/to/.wh.somefile.txt"

	newPath, err = path.UnWhiteoutPath()
	if err != nil {
		t.Fatal("error while unwhiteing out", err)
	}
	if newPath != "/some/Path/to/somefile.txt" {
		t.Fatal("Path should be a whiteout")
	}
}
