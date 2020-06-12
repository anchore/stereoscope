// +build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/anchore/go-testutils"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

type linkFetchConfig struct {
	linkLayer        int
	linkPath         string
	resolveLayer     int
	expectedPath     string
	perspectiveLayer int
}

func TestImageSymlinks(t *testing.T) {
	fixtureName := "image-symlinks"
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
			assertImageSymlinkLinkResolution(t, i)
		})
	}

	if len(cases) < len(image.AllSources) {
		t.Fatalf("probably missed a source during testing, double check that all image.sources are covered")
	}

}

func assertMatch(t *testing.T, i *image.Image, cfg linkFetchConfig, expectedResolve, actualResolve *file.Reference) {
	t.Helper()
	if actualResolve.ID() != expectedResolve.ID() {
		var exLayer int = -1
		var acLayer int = -1
		var exType byte = 0x0
		var acType byte = 0x0

		eM, err := i.FileCatalog.Get(*expectedResolve)
		if err == nil {
			exLayer = int(eM.Source.Metadata.Index)
			exType = eM.Metadata.TypeFlag
		}

		aM, err := i.FileCatalog.Get(*actualResolve)
		if err == nil {
			acLayer = int(aM.Source.Metadata.Index)
			acType = aM.Metadata.TypeFlag
		}

		t.Fatalf("mismatched link resolution link=%+v: '%+v (layer=%d type=%+v)'!='%+v (layer=%d type=%+v linkName=%s)'", cfg.linkPath, expectedResolve, exLayer, exType, actualResolve, acLayer, acType, aM.Metadata.Linkname)
	}
}

func fetchRefs(t *testing.T, i *image.Image, cfg linkFetchConfig) (*file.Reference, *file.Reference) {
	link := i.Layers[cfg.linkLayer].Tree.File(file.Path(cfg.linkPath))
	if link == nil {
		t.Fatalf("missing expected link: %s", cfg.linkPath)
	}

	expectedResolve := i.Layers[cfg.resolveLayer].Tree.File(file.Path(cfg.expectedPath))
	if expectedResolve == nil {
		t.Fatalf("missing expected path: %s", expectedResolve)
	}

	actualResolve, err := i.ResolveLinkByLayerSquash(*link, cfg.perspectiveLayer)
	if err != nil {
		t.Fatalf("failed to resolve link=%+v: %+v", link, err)
	}
	return expectedResolve, actualResolve
}

func assertImageSymlinkLinkResolution(t *testing.T, i *image.Image) {

	tests := []linkFetchConfig{
		// LAYER 0 > FROM busybox:latest (hardlink test)
		{
			linkLayer:        0,
			linkPath:         "/bin/busybox",
			resolveLayer:     0,
			expectedPath:     "/bin/[",
			perspectiveLayer: 0,
		},
		// # link with previous data
		// LAYER 1 > ADD file-1.txt .
		// LAYER 2 > RUN ln -s ./file-1.txt link-1
		{
			linkLayer:        2,
			linkPath:         "/link-1",
			resolveLayer:     1,
			expectedPath:     "/file-1.txt",
			perspectiveLayer: 2,
		},

		// # link with future data
		// LAYER 3 > RUN ln -s ./file-2.txt link-2
		// LAYER 4 > ADD file-2.txt .
		{
			linkLayer:        3,
			linkPath:         "/link-2",
			resolveLayer:     4,
			expectedPath:     "/file-2.txt",
			perspectiveLayer: 4,
		},

		// # link with current data
		// LAYER 5 > RUN echo "file 3" > file-3.txt && ln -s ./file-3.txt link-within
		{
			linkLayer:        5,
			linkPath:         "/link-within",
			resolveLayer:     5,
			expectedPath:     "/file-3.txt",
			perspectiveLayer: 5,
		},

		// # multiple links (link-indirect > link-2 > file-2.txt)
		// LAYER 6 > RUN ln -s ./link-2 link-indirect
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     4,
			expectedPath:     "/file-2.txt",
			perspectiveLayer: 6,
		},

		// # override contents / resolution
		// LAYER 7 > ADD new-file-2.txt file-2.txt
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     7,
			expectedPath:     "/file-2.txt",
			perspectiveLayer: 7,
		},

		// # dead link (link-indirect > [non-existant file])
		// LAYER 8 > RUN unlink link-2
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     6,
			expectedPath:     "/link-indirect",
			perspectiveLayer: 8,
		},
	}

	for _, cfg := range tests {
		name := fmt.Sprintf("[%d:%s]-->[%d:%s]@%d", cfg.linkLayer, cfg.linkPath, cfg.resolveLayer, cfg.expectedPath, cfg.perspectiveLayer)
		t.Run(name, func(t *testing.T) {
			expectedResolve, actualResolve := fetchRefs(t, i, cfg)
			assertMatch(t, i, cfg, expectedResolve, actualResolve)
		})
	}
}
