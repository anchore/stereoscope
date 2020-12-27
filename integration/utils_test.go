package integration

import (
	"github.com/anchore/stereoscope/pkg/filetree"
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func compareLayerSquashTrees(t *testing.T, expected map[uint]*filetree.FileTree, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	var actual = make([]*filetree.FileTree, 0)
	for _, l := range i.Layers {
		actual = append(actual, l.SquashedTree)
	}

	compareTrees(t, expected, actual, ignorePaths)
}

func compareLayerTrees(t *testing.T, expected map[uint]*filetree.FileTree, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	var actual = make([]*filetree.FileTree, 0)
	for _, l := range i.Layers {
		actual = append(actual, l.Tree)
	}

	compareTrees(t, expected, actual, ignorePaths)
}

func compareTrees(t *testing.T, expected map[uint]*filetree.FileTree, actual []*filetree.FileTree, ignorePaths []file.Path) {
	t.Helper()

	for idx, expected := range expected {
		actual := actual[idx]
		if !expected.Equal(actual) {
			extra, missing := expected.PathDiff(actual)
			nonIgnoredPaths := 0

			for _, p := range extra {
				found := false
			inner1:
				for _, ignore := range ignorePaths {
					if ignore == p {
						found = true
						break inner1
					}
				}
				if !found {
					nonIgnoredPaths++
				}
			}

			for _, p := range missing {
				found := false
			inner2:
				for _, ignore := range ignorePaths {
					if ignore == p {
						found = true
						break inner2
					}
				}
				if !found {
					nonIgnoredPaths++
				}
			}
			if nonIgnoredPaths > 0 {
				t.Errorf("ignore paths: %+v", ignorePaths)
				t.Errorf("path differences: extra=%+v missing=%+v", extra, missing)
				t.Errorf("mismatched trees (layer %d)", idx)
			}
		}
	}
}

func compareSquashTree(t *testing.T, expected *filetree.FileTree, i *image.Image) {
	t.Helper()

	actual := i.SquashedTree()
	if !expected.Equal(actual) {
		t.Log("Walking expected squashed tree:")
		expected.Walk(func(p file.Path, f *file.Reference) {
			t.Log("   ", p)
		})

		t.Log("Walking actual squashed tree:")
		actual.Walk(func(p file.Path, f *file.Reference) {
			t.Log("   ", p)
		})

		extra, missing := expected.PathDiff(actual)
		t.Errorf("path differences: extra=%+v missing=%+v", extra, missing)
		t.Errorf("mismatched squashed trees")
	}
}
