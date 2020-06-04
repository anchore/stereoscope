// +build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/tree"
)

const (
	testFixturesDirName = "test-fixtures"
	tarCacheDirName     = "tar-cache"
	imagePrefix         = "stereoscope-fixture"
)

func compareLayerTrees(t *testing.T, expected map[uint]*tree.FileTree, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	for idx, expected := range expected {
		actual := i.Layers[idx].Tree
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
				t.Logf("ignore paths: %+v", ignorePaths)
				t.Logf("path differences: extra=%+v missing=%+v", extra, missing)
				t.Errorf("mismatched trees (layer %d)", idx)
			}
		}
	}
}

func compareSquashTree(t *testing.T, expected *tree.FileTree, i *image.Image) {
	t.Helper()

	actual := i.SquashedTree
	if !expected.Equal(actual) {
		fmt.Println(expected.PathDiff(actual))
		t.Errorf("mismatched squashed trees")
	}

}
