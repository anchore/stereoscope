//go:build !windows

package integration

import (
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

// These tests state what the container built by testdata/image-hardlinks/Dockerfile actually serves
// at each path, and assert that stereoscope agrees. They are the specification, not a snapshot of
// current behavior, so a shape stereoscope gets wrong fails here until the code is fixed.
//
// The rule the cases encode: a hardlink is a second name for an inode, fixed when the link is
// created. Replacing, overwriting or deleting the OTHER name of that inode in a later layer does not
// change what this name refers to.

type hardlinkPathCase struct {
	path string
	// contents is what `cat path` returns inside the running container.
	contents string
	// absent means the path does not exist in the running container.
	absent bool
	// note explains why this answer is what it is, where that is not obvious.
	note string
}

func hardlinkPathCases() []hardlinkPathCase {
	return []hardlinkPathCase{
		// c1: the other name was modified in place, so overlayfs copied it up to a new inode.
		{path: "/c1/hard.txt", contents: "CASE1-ORIGINAL", note: "keeps the original inode"},
		{path: "/c1/orig.txt", contents: "CASE3-MODIFIED-IN-PLACE"},

		// c2: a cross-layer `ln` is a full copy, so nothing hardlink-specific happens here.
		{path: "/c2/cross-hard.txt", contents: "CASE1-ORIGINAL"},

		// c4: the other name was replaced by a new regular file.
		{path: "/c4/hard.txt", contents: "CASE4-ORIGINAL"},
		{path: "/c4/orig.txt", contents: "CASE4-REPLACED"},

		// c5: the uv/pip shape. squashed, the installed name is a symlink to the cache name and the
		// cache name is a hardlink whose link path names the installed name. that is a cycle in the
		// path graph but not on disk, where the cache name still refers to the original inode.
		{path: "/c5/bin/tool", contents: "CASE5-PAYLOAD", note: "symlink into the cache"},
		{path: "/c5/cache/blob", contents: "CASE5-PAYLOAD", note: "the surviving name for the payload inode"},

		// c6: `ln` onto a symlink hardlinks the symlink inode, so both names are symlinks to the
		// same destination and reading either follows through to the real file.
		{path: "/c6/real.txt", contents: "CASE6-REAL"},
		{path: "/c6/sym.txt", contents: "CASE6-REAL", note: "a hardlink naming a symlink IS a symlink"},
		{path: "/c6/hard-to-sym.txt", contents: "CASE6-REAL"},

		// c7: the other name was deleted.
		{path: "/c7/hard.txt", contents: "CASE7-PAYLOAD"},
		{path: "/c7/orig.txt", absent: true},

		// c8: three names, one inode.
		{path: "/c8/a.txt", contents: "CASE8-PAYLOAD"},
		{path: "/c8/b.txt", contents: "CASE8-PAYLOAD"},
		{path: "/c8/c.txt", contents: "CASE8-PAYLOAD"},

		// c9: the data-carrying name was overwritten in a later layer, so the two names diverge.
		{path: "/c9/hard.txt", contents: "CASE9-HARDLINK-OVERWRITTEN"},
		{path: "/c9/orig.txt", contents: "CASE9-ORIGINAL", note: "still the layer's original inode"},

		// c10: the data-carrying name was whiteouted. deleting one name of an inode does not remove
		// the others, so z.txt is still readable.
		{path: "/c10/a.txt", absent: true},
		{path: "/c10/z.txt", contents: "CASE10-PAYLOAD"},

		// c11: the data-carrying name was replaced in a later layer.
		{path: "/c11/a.txt", contents: "CASE11-A-REPLACED-LATER"},
		{path: "/c11/z.txt", contents: "CASE11-PAYLOAD", note: "still the original inode"},
	}
}

func TestImageHardLinks(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{name: "FromTarball", source: "docker-archive"},
		{name: "FromDocker", source: "docker"},
		{name: "FromPodman", source: "podman"},
		{name: "FromContainerd", source: "containerd"},
		{name: "FromOciTarball", source: "oci-archive"},
		{name: "FromOciDirectory", source: "oci-dir"},
		// note: singularity is deliberately absent. it is backed by squashfs, which resolves
		// hardlinks when the filesystem is built, so there are no hardlink entries left to test.
	}

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

			img := imagetest.GetFixtureImage(t, c.source, "image-hardlinks")

			for _, tc := range hardlinkPathCases() {
				t.Run(tc.path, func(t *testing.T) {
					contents, err := hardlinkContents(img, tc.path)

					if tc.absent {
						require.Error(t, err, "path does not exist in the container: %s", tc.note)
						return
					}

					require.NoError(t, err, tc.note)
					assert.Equal(t, tc.contents, contents, tc.note)
				})
			}
		})
	}
}

func hardlinkContents(img *image.Image, path string) (string, error) {
	reader, err := img.OpenPathFromSquash(file.Path(path))
	if err != nil {
		return "", err
	}
	defer reader.Close()

	contents, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(contents), nil
}
