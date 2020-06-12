// +build integration

package integration

import (
	"strings"
	"testing"

	"github.com/anchore/go-testutils"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/tree"
)

func TestSimpleImage(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct {
		name        string
		source      string
		fixtureName string
	}{
		{
			name:        "FromTarball",
			source:      "docker-archive",
			fixtureName: fixtureName,
		},
		{
			name:        "FromDocker",
			source:      "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i, cleanup := testutils.GetFixtureImage(t, c.source, c.fixtureName)
			defer cleanup()

			assertImageSimpleMetadata(t, i)
			assertImageSimpleTrees(t, i)
			assertImageSimpleSquashedTrees(t, i)
			assertImageSimpleContents(t, i)
		})
	}

	if len(cases) < len(image.AllSources) {
		t.Fatalf("probably missed a source during testing, double check that all image.sources are covered")
	}

}

func assertImageSimpleMetadata(t *testing.T, i *image.Image) {
	t.Log("Asserting metadata...")
	if i.Metadata.Size != 65 {
		t.Errorf("unexpected image size: %d", i.Metadata.Size)
	}
	if string(i.Metadata.MediaType) != "application/vnd.docker.distribution.manifest.v2+json" {
		t.Errorf("unexpected image media type: %+v", i.Metadata.MediaType)
	}
	if len(i.Metadata.Tags) != 1 {
		t.Errorf("unexpected number of tags: %d", len(i.Metadata.Tags))
	} else {
		if !strings.HasPrefix(i.Metadata.Tags[0].String(), "anchore-fixture-image-simple:") {
			t.Errorf("unexpected image tag: %+v", i.Metadata.Tags)
		}
	}

	expected := []image.LayerMetadata{
		{
			Index:     0,
			Size:      22,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Index:     1,
			Size:      16,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Index:     2,
			Size:      27,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
	}

	if len(expected) != len(i.Layers) {
		t.Fatal("unexpected number of layers:", len(i.Layers))
	}

	for idx, l := range i.Layers {
		if expected[idx].Size != l.Metadata.Size {
			t.Errorf("mismatched layer 'Size' (layer %d): %+v", idx, l.Metadata.Size)
		}
		if expected[idx].MediaType != l.Metadata.MediaType {
			t.Errorf("mismatched layer 'MediaType' (layer %d): %+v", idx, l.Metadata.MediaType)
		}
		if expected[idx].Index != l.Metadata.Index {
			t.Errorf("mismatched layer 'Index' (layer %d): %+v", idx, l.Metadata.Index)
		}
	}
}

func assertImageSimpleSquashedTrees(t *testing.T, i *image.Image) {
	t.Log("Asserting squashed trees...")
	one := tree.NewFileTree()
	one.AddPath("/somefile-1.txt")

	two := tree.NewFileTree()
	two.AddPath("/somefile-1.txt")
	two.AddPath("/somefile-2.txt")

	three := tree.NewFileTree()
	three.AddPath("/somefile-1.txt")
	three.AddPath("/somefile-2.txt")
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

	compareLayerSquashTrees(t, expectedTrees, i, ignorePaths)

	squashed := tree.NewFileTree()
	squashed.AddPath("/somefile-1.txt")
	squashed.AddPath("/somefile-2.txt")
	squashed.AddPath("/really/nested/file-3.txt")

	compareSquashTree(t, squashed, i)
}

func assertImageSimpleTrees(t *testing.T, i *image.Image) {
	t.Log("Asserting trees...")
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
}

func assertImageSimpleContents(t *testing.T, i *image.Image) {
	t.Log("Asserting contents...")
	actualContents, err := i.MultipleFileContentsFromSquash(
		"/somefile-1.txt",
		"/somefile-2.txt",
		"/really/nested/file-3.txt",
	)

	if err != nil {
		t.Fatal("unable to fetch multiple contents", err)
	}

	expectedContents := map[string]string{
		"/somefile-1.txt":           "this file has contents",
		"/somefile-2.txt":           "file-2 contents!",
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
