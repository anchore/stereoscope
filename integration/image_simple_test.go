// +build integration

package integration

import (
	"github.com/anchore/stereoscope/pkg/tree"
	"testing"
)

func TestSimpleImageFiletrees(t *testing.T) {
	i := getSquashedImage(t, "image-simple")

	one := tree.NewFileTree()
	one.AddPath("/somefile-1.txt")

	two := tree.NewFileTree()
	two.AddPath("/somefile-2.txt")

	three := tree.NewFileTree()
	three.AddPath("/really/.wh..wh..opq")
	three.AddPath("/really/nested/file-3.txt")

	expectedTrees := map[uint]*tree.FileTree{
		0: one,
		1: two,
		2: three,
	}

	compareLayerTrees(t, expectedTrees, i)

	squashed := tree.NewFileTree()
	squashed.AddPath("/somefile-1.txt")
	squashed.AddPath("/somefile-2.txt")
	squashed.AddPath("/really/nested/file-3.txt")

	compareSquashTree(t, squashed, i)
}

func TestSimpleImageMultipleFileContents(t *testing.T) {
	i := getSquashedImage(t, "image-simple")
	actualContents, err := i.MultipleFileContents(
		"/somefile-1.txt",
		"/somefile-2.txt",
		"/really/nested/file-3.txt",
	)

	if err != nil {
		t.Fatal("unable to fetch multiple contents", err)
	}

	expectedContents := map[string]string{
		"/somefile-1.txt": "this file has contents",
		"/somefile-2.txt": "file-2 contents!",
		"/really/nested/file-3.txt": "another file!\nwith lines...",
	}

	if len(expectedContents) != len(actualContents) {
		t.Fatalf("mismatched number of contents: %d!=%d", len(expectedContents), len(actualContents))
	}

	for fileRef, actual := range actualContents {
		expected, ok := expectedContents[string(fileRef.Path)]
		if !ok {
			t.Errorf("extra path found: %+v", fileRef.Path)
		}
		if expected != actual {
			t.Errorf("mismatched contents (%s)", fileRef.Path)
		}
	}
}
