//go:build !windows
// +build !windows

package integration

import (
	"fmt"
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

type linkFetchConfig struct {
	linkLayer        int
	linkPath         string
	resolveLayer     int
	expectedPath     string
	perspectiveLayer int
	contents         string
	linkOptions      []filetree.LinkResolutionOption
}

func TestImageSymlinks(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name:   "FromTarball",
			source: "docker-archive",
		},
		{
			name:   "FromDocker",
			source: "docker",
		},
		{
			name:   "FromPodman",
			source: "podman",
		},
		{
			name:   "FromContainerd",
			source: "containerd",
		},
		{
			name:   "FromOciTarball",
			source: "oci-archive",
		},
		{
			name:   "FromOciDirectory",
			source: "oci-dir",
		},
		{
			name:   "FromSingularity",
			source: "singularity",
		},
	}

	expectedSet := stereoscope.ImageProviders(stereoscope.ImageProviderConfig{}).
		Remove(image.OciRegistrySource).
		Collect()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if runtime.GOOS != "linux" {
				switch c.source {
				case "containerd":
					t.Skip("containerd is only supported on linux")
				case "podman":
					t.Skip("podman is only supported on linux")
				}
			}

			i := imagetest.GetFixtureImage(t, c.source, "image-symlinks")

			if c.source == "singularity" {
				assertSquashedSymlinkLinkResolution(t, i)
				return
			}

			assertImageSymlinkLinkResolution(t, i)
		})
	}

	if len(cases) < len(expectedSet) {
		t.Fatalf("probably missed a source during testing, double check that all image.sources are covered")
	}

}

func assertMatch(t *testing.T, i *image.Image, cfg linkFetchConfig, expectedResolve, actualResolve *file.Reference) {
	t.Helper()
	if expectedResolve == nil && actualResolve != nil || expectedResolve != nil && actualResolve == nil {
		t.Fatalf("one of the resolved file.References is nil: expected=%+v actual=%+v", expectedResolve, actualResolve)
	}
	if expectedResolve == nil && actualResolve == nil {
		return
	}
	if actualResolve.ID() != expectedResolve.ID() {
		var exLayer = -1
		var acLayer = -1
		var exType file.Type
		var acType file.Type

		eM, err := i.FileCatalog.Get(*expectedResolve)
		if err == nil {
			exLayer = int(i.FileCatalog.Layer(*expectedResolve).Metadata.Index)
			exType = eM.Metadata.Type
		}

		aM, err := i.FileCatalog.Get(*actualResolve)
		if err == nil {
			acLayer = int(i.FileCatalog.Layer(*actualResolve).Metadata.Index)
			acType = aM.Metadata.Type
		}

		t.Fatalf("mismatched link resolution link=%+v: <%+v (layer=%d type=%+v)> != <%+v (layer=%d type=%+v linkName=%s)>", cfg.linkPath, expectedResolve, exLayer, exType, actualResolve, acLayer, acType, aM.Metadata.LinkDestination)
	}
}

func fetchRefs(t *testing.T, i *image.Image, cfg linkFetchConfig) (*file.Reference, *file.Reference) {
	_, link, err := i.Layers[cfg.linkLayer].Tree.File(file.Path(cfg.linkPath), cfg.linkOptions...)
	require.NoError(t, err)
	require.NotNil(t, link)

	_, expectedResolve, err := i.Layers[cfg.resolveLayer].Tree.File(file.Path(cfg.expectedPath), cfg.linkOptions...)
	require.NoError(t, err)
	require.NotNil(t, expectedResolve)

	actualResolve, err := i.ResolveLinkByLayerSquash(*link.Reference, cfg.perspectiveLayer, cfg.linkOptions...)
	require.NoError(t, err)
	return expectedResolve.Reference, actualResolve.Reference
}

func fetchContents(t *testing.T, i *image.Image, cfg linkFetchConfig) string {
	contents, err := i.Layers[cfg.perspectiveLayer].OpenPathFromSquash(file.Path(cfg.linkPath))
	require.NoError(t, err)

	b, err := io.ReadAll(contents)
	require.NoError(t, err)
	return string(b)
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
			contents:         "file 1!",
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
			contents:         "file 2!",
		},

		// # link with current data
		// LAYER 5 > RUN echo "file 3" > file-3.txt && ln -s ./file-3.txt link-within
		{
			linkLayer:        5,
			linkPath:         "/link-within",
			resolveLayer:     5,
			expectedPath:     "/file-3.txt",
			perspectiveLayer: 5,
			// since echo was used a newline character will be present
			contents: "file 3\n",
		},

		// # multiple links (link-indirect > link-2 > file-2.txt)
		// LAYER 6 > RUN ln -s ./link-2 link-indirect
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     4,
			expectedPath:     "/file-2.txt",
			perspectiveLayer: 6,
			contents:         "file 2!",
		},

		// # override contents / resolution
		// LAYER 7 > ADD new-file-2.txt file-2.txt
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     7,
			expectedPath:     "/file-2.txt",
			perspectiveLayer: 7,
			contents:         "NEW file override!",
		},

		// # dead link (link-indirect > [non-existant file])
		// LAYER 8 > RUN unlink link-2
		{
			linkLayer:        6,
			linkPath:         "/link-indirect",
			resolveLayer:     6,
			expectedPath:     "/link-indirect",
			perspectiveLayer: 8,
			linkOptions:      []filetree.LinkResolutionOption{filetree.DoNotFollowDeadBasenameLinks},
		},
	}

	for _, cfg := range tests {
		name := fmt.Sprintf("[%d:%s]-->[%d:%s]@%d", cfg.linkLayer, cfg.linkPath, cfg.resolveLayer, cfg.expectedPath, cfg.perspectiveLayer)
		t.Run(name, func(t *testing.T) {
			expectedResolve, actualResolve := fetchRefs(t, i, cfg)
			assertMatch(t, i, cfg, expectedResolve, actualResolve)

			if cfg.contents == "" {
				return
			}

			actualContents := fetchContents(t, i, cfg)
			if actualContents != cfg.contents {
				t.Errorf("mismatched contents: '%+v'!='%+v'", cfg.contents, actualContents)
			}
		})
	}
}

// Check symlinks in image after it has been squashed to a single layer (SingularityID)
func assertSquashedSymlinkLinkResolution(t *testing.T, i *image.Image) {
	tests := []linkFetchConfig{
		// # link with previous data
		// LAYER 1 > ADD file-1.txt .
		// LAYER 2 > RUN ln -s ./file-1.txt link-1
		{
			linkLayer:        0,
			linkPath:         "/link-1",
			resolveLayer:     0,
			expectedPath:     "/file-1.txt",
			perspectiveLayer: 0,
			contents:         "file 1!",
		},

		// # link with current data
		// LAYER 5 > RUN echo "file 3" > file-3.txt && ln -s ./file-3.txt link-within
		{
			linkLayer:        0,
			linkPath:         "/link-within",
			resolveLayer:     0,
			expectedPath:     "/file-3.txt",
			perspectiveLayer: 0,
			// since echo was used a newline character will be present
			contents: "file 3\n",
		},

		// # dead link (link-indirect > [non-existant file])
		// LAYER 8 > RUN unlink link-2
		{
			linkLayer:        0,
			linkPath:         "/link-indirect",
			resolveLayer:     0,
			expectedPath:     "/link-indirect",
			perspectiveLayer: 0,
			linkOptions:      []filetree.LinkResolutionOption{filetree.DoNotFollowDeadBasenameLinks},
		},
	}

	for _, cfg := range tests {
		name := fmt.Sprintf("[%d:%s]-->[%d:%s]@%d", cfg.linkLayer, cfg.linkPath, cfg.resolveLayer, cfg.expectedPath, cfg.perspectiveLayer)
		t.Run(name, func(t *testing.T) {
			expectedResolve, actualResolve := fetchRefs(t, i, cfg)
			assertMatch(t, i, cfg, expectedResolve, actualResolve)

			if cfg.contents == "" {
				return
			}

			actualContents := fetchContents(t, i, cfg)
			if actualContents != cfg.contents {
				t.Errorf("mismatched contents: '%+v'!='%+v'", cfg.contents, actualContents)
			}
		})
	}
}
