package integration

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/image"
)

func compareLayerSquashTrees(t *testing.T, expected map[uint]filetree.Reader, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	var actual = make([]filetree.Reader, 0)
	for _, l := range i.Layers {
		actual = append(actual, l.SquashedTree)
	}

	compareTrees(t, expected, actual, ignorePaths)
}

func compareLayerTrees(t *testing.T, expected map[uint]filetree.Reader, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	var actual = make([]filetree.Reader, 0)
	for _, l := range i.Layers {
		actual = append(actual, l.Tree)
	}

	compareTrees(t, expected, actual, ignorePaths)
}

func compareTrees(t *testing.T, expected map[uint]filetree.Reader, actual []filetree.Reader, ignorePaths []file.Path) {
	t.Helper()

	for idx, e := range expected {
		a := actual[idx]
		if !e.(*filetree.FileTree).Equal(a.(*filetree.FileTree)) {
			extra, missing := e.(*filetree.FileTree).PathDiff(a.(*filetree.FileTree))
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

func compareSquashTree(t *testing.T, expected filetree.Reader, i *image.Image) {
	t.Helper()

	actual := i.SquashedTree()
	if !expected.(*filetree.FileTree).Equal(actual.(*filetree.FileTree)) {
		t.Log("Walking expected squashed tree:")
		err := expected.Walk(func(p file.Path, _ filenode.FileNode) error {
			t.Log("   ", p)
			return nil
		}, nil)
		if err != nil {
			t.Fatalf("failed to walk tree: %+v", err)
		}

		t.Log("Walking actual squashed tree:")
		err = actual.Walk(func(p file.Path, _ filenode.FileNode) error {
			t.Log("   ", p)
			return nil
		}, nil)
		if err != nil {
			t.Fatalf("failed to walk tree: %+v", err)
		}

		extra, missing := expected.(*filetree.FileTree).PathDiff(actual.(*filetree.FileTree))
		t.Errorf("path differences: extra=%+v missing=%+v", extra, missing)
		t.Errorf("mismatched squashed trees")
	}
}
