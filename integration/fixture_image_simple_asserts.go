// +build integration

package integration

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/tree"
	"testing"
)

func assertImageSimpleFixtureTrees(t *testing.T, i *image.Image) {
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

	// there is a difference in behavior between docker 18 and 19 regarding opaque whiteout
	// creation during docker build (which could lead to test inconsistencies depending where
	// this test is run. However, this opaque whiteout is not important to theses tests, only
	// the correctness of the layer representation and squashing ability.
	ignorePaths := []file.Path{"/really/.wh..wh..opq"}

	compareLayerTrees(t, expectedTrees, i, ignorePaths)

	squashed := tree.NewFileTree()
	squashed.AddPath("/somefile-1.txt")
	squashed.AddPath("/somefile-2.txt")
	squashed.AddPath("/really/nested/file-3.txt")

	compareSquashTree(t, squashed, i)
}

func assertImageSimpleFixtureContents(t *testing.T, i *image.Image) {
	actualContents, err := i.MultipleFileContentsFromSquash(
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