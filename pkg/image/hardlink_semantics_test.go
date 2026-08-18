package image

import (
	"archive/tar"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// These tests state what a container actually serves for each hardlink shape, and assert that
// stereoscope agrees. They are the specification, not a snapshot of current behavior, so any case
// stereoscope gets wrong fails here until the code is fixed.
//
// The layer layouts are not invented. They were captured from the image built by
// test/integration/testdata/image-hardlinks/Dockerfile and read back with `docker save`, so the tar
// header type, ordering and linknames match what a container runtime actually produces. The
// integration test over that fixture (TestImageHardLinks) is the ground-truth anchor; this file is
// the docker-free version of the same cases, and can also express layouts docker will not emit.
//
// Two properties of that output are load-bearing, and both are easy to guess wrong:
//
//   - The data section goes to the ALPHABETICALLY FIRST name, not to the name that was created
//     first. `printf > orig.txt && ln orig.txt hard.txt` yields a REG header for hard.txt and a
//     HARDLINK header for orig.txt, because the tar writer walks the directory sorted.
//   - A hardlink header's target is ALWAYS in the same layer, and always precedes it. `ln` across
//     layers forces an overlayfs copy-up, so it emits a full REG copy rather than a hardlink header.
//
// The rule the cases below encode: a hardlink is a second name for an inode, fixed when the link is
// created. Replacing, overwriting or deleting the OTHER name of that inode in a later layer does not
// change what this name refers to.

const (
	c1Original  = "CASE1-ORIGINAL"
	c3Modified  = "CASE3-MODIFIED-IN-PLACE"
	c4Original  = "CASE4-ORIGINAL"
	c4Replaced  = "CASE4-REPLACED"
	c5Payload   = "CASE5-PAYLOAD"
	c6Real      = "CASE6-REAL"
	c7Payload   = "CASE7-PAYLOAD"
	c8Payload   = "CASE8-PAYLOAD"
	c9Original  = "CASE9-ORIGINAL"
	c9Overwrite = "CASE9-HARDLINK-OVERWRITTEN"
	c10Payload  = "CASE10-PAYLOAD"
	c11Payload  = "CASE11-PAYLOAD"
	c11Replaced = "CASE11-A-REPLACED-LATER"
)

// hardlinkCase is one path and what the container serves at it.
type hardlinkCase struct {
	path string
	// contents is what `cat path` returns inside the running container.
	contents string
	// absent means the path does not exist in the running container.
	absent bool
	// note explains why this answer is what it is, where that is not obvious.
	note string
}

func hardlinkCases() []hardlinkCase {
	return []hardlinkCase{
		// c1: the other name was modified in place, so overlayfs copied it up to a new inode.
		{path: "/c1/hard.txt", contents: c1Original, note: "keeps the original inode"},
		{path: "/c1/orig.txt", contents: c3Modified},

		// c2: a cross-layer `ln` is a full copy, so nothing hardlink-specific happens here.
		{path: "/c2/cross-hard.txt", contents: c1Original},

		// c4: the other name was replaced by a new regular file.
		{path: "/c4/hard.txt", contents: c4Original},
		{path: "/c4/orig.txt", contents: c4Replaced},

		// c5: the uv/pip shape. squashed, the installed name is a symlink to the cache name and the
		// cache name is a hardlink whose link path names the installed name. that is a cycle in the
		// path graph but not on disk, where the cache name still refers to the original inode.
		{path: "/c5/bin/tool", contents: c5Payload, note: "symlink into the cache"},
		{path: "/c5/cache/blob", contents: c5Payload, note: "the surviving name for the payload inode"},

		// c6: `ln` onto a symlink hardlinks the symlink inode, so both names are symlinks to the
		// same destination and reading either follows through to the real file.
		{path: "/c6/real.txt", contents: c6Real},
		{path: "/c6/sym.txt", contents: c6Real, note: "a hardlink naming a symlink IS a symlink"},
		{path: "/c6/hard-to-sym.txt", contents: c6Real},

		// c7: the other name was deleted.
		{path: "/c7/hard.txt", contents: c7Payload},
		{path: "/c7/orig.txt", absent: true},

		// c8: three names, one inode.
		{path: "/c8/a.txt", contents: c8Payload},
		{path: "/c8/b.txt", contents: c8Payload},
		{path: "/c8/c.txt", contents: c8Payload},

		// c9: the data-carrying name was overwritten in a later layer, so the two names diverge.
		{path: "/c9/hard.txt", contents: c9Overwrite},
		{path: "/c9/orig.txt", contents: c9Original, note: "still the layer's original inode"},

		// c10: the data-carrying name was whiteouted. deleting one name of an inode does not remove
		// the others, so z.txt is still readable.
		{path: "/c10/a.txt", absent: true},
		{path: "/c10/z.txt", contents: c10Payload},

		// c11: the data-carrying name was replaced in a later layer.
		{path: "/c11/a.txt", contents: c11Replaced},
		{path: "/c11/z.txt", contents: c11Payload, note: "still the original inode"},
	}
}

// TestHardLinkSemantics_Contents asserts stereoscope serves what the container serves.
func TestHardLinkSemantics_Contents(t *testing.T) {
	img := hardlinkReproImage(t)

	for _, tc := range hardlinkCases() {
		t.Run(tc.path, func(t *testing.T) {
			contents, err := hardlinkContentsForTest(img, tc.path)

			if tc.absent {
				require.Error(t, err, "path does not exist in the container: %s", tc.note)
				return
			}

			require.NoError(t, err, tc.note)
			assert.Equal(t, tc.contents, contents, tc.note)
		})
	}
}

// TestHardLinkSemantics_ResolvedPathExists asserts that whatever reference a path resolves to names
// a path that is actually present in the squashed tree. Resolving to a location the image does not
// contain is wrong regardless of which name we choose to report.
func TestHardLinkSemantics_ResolvedPathExists(t *testing.T) {
	img := hardlinkReproImage(t)
	tree := img.SquashedTree()

	for _, tc := range hardlinkCases() {
		if tc.absent {
			continue
		}

		t.Run(tc.path, func(t *testing.T) {
			_, resolution, err := tree.File(file.Path(tc.path), filetree.FollowBasenameLinks)
			require.NoError(t, err)
			require.NotNil(t, resolution)
			require.NotNil(t, resolution.Reference)

			exists, _, err := tree.File(resolution.RealPath)
			require.NoError(t, err)
			assert.True(t, exists,
				"%s resolves to %s, which is not in the squashed tree", tc.path, resolution.RealPath)
		})
	}
}

// TestHardLinkSemantics_Metadata asserts that a hardlinked name reports the size of the inode it
// names. A hardlink tar header carries no data section, so taking its size straight from the header
// describes the header rather than the file, and a consumer reading that size sees an empty file.
//
// Symlinks are excluded: a symlink inode's size is the length of its link text, not the size of
// whatever it points at, so size-matches-contents is not the right question for them.
func TestHardLinkSemantics_Metadata(t *testing.T) {
	img := hardlinkReproImage(t)
	tree := img.SquashedTree()

	for _, tc := range hardlinkCases() {
		if tc.absent {
			continue
		}

		t.Run(tc.path, func(t *testing.T) {
			// the path's OWN node in the squashed tree, with no link following, so this is the entry
			// a consumer sees for this name rather than the one for some other layer's version of it
			exists, resolution, err := tree.File(file.Path(tc.path))
			require.NoError(t, err)
			require.True(t, exists)
			require.NotNil(t, resolution.Reference)

			entry, err := img.FileCatalog.Get(*resolution.Reference)
			require.NoError(t, err)

			if entry.Metadata.Type == file.TypeSymLink {
				t.Skipf("%s is a symlink; its size is its link text", tc.path)
			}

			assert.Equal(t, int64(len(tc.contents)), entry.Metadata.Size(),
				"%s reports a size that does not match the contents the container serves", tc.path)
		})
	}
}

// TestHardLinkSemantics_MIMESearchFindsHardLinks asserts that a hardlinked name is reachable by
// MIME-driven search. Enumerating by MIME type is how consumers find files to catalog, so a
// hardlinked binary being invisible to it means the file is silently skipped.
func TestHardLinkSemantics_MIMESearchFindsHardLinks(t *testing.T) {
	img := hardlinkReproImage(t)

	refs, err := img.FilesByMIMETypeFromSquash("text/plain")
	require.NoError(t, err)

	found := make(map[string]bool)
	for _, ref := range refs {
		found[string(ref.RealPath)] = true
	}

	for _, path := range []string{"/c8/a.txt", "/c8/b.txt", "/c8/c.txt"} {
		assert.True(t, found[path], "%s is text/plain in the container but is not reachable by MIME search", path)
	}
}

// hardlinkReproImage builds one image whose layers mirror
// test/integration/testdata/image-hardlinks/Dockerfile, layer for layer, using the tar header
// layout captured from the real `docker build` output for that fixture.
func hardlinkReproImage(t *testing.T) *Image {
	t.Helper()

	reg := byte(tar.TypeReg)
	lnk := byte(tar.TypeLink)
	sym := byte(tar.TypeSymlink)

	return readImageFromLayers(t,
		// case 1: same-layer hardlink pair. note hard.txt carries the data (alphabetically first).
		layerFromTarEntries(t,
			tarEntry{path: "c1/hard.txt", typeFlag: reg, contents: c1Original},
			tarEntry{path: "c1/orig.txt", typeFlag: lnk, linkPath: "c1/hard.txt"},
		),
		// case 2: `ln` to a previous layer's file becomes a full copy, not a hardlink header.
		layerFromTarEntries(t,
			tarEntry{path: "c2/cross-hard.txt", typeFlag: reg, contents: c1Original},
		),
		// case 3: the data-carrying name's sibling is modified in place; overlayfs copies it up.
		layerFromTarEntries(t,
			tarEntry{path: "c1/orig.txt", typeFlag: reg, contents: c3Modified},
		),
		// case 4: same-layer pair, then the hardlink name is replaced by a new regular file.
		layerFromTarEntries(t,
			tarEntry{path: "c4/hard.txt", typeFlag: reg, contents: c4Original},
			tarEntry{path: "c4/orig.txt", typeFlag: lnk, linkPath: "c4/hard.txt"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c4/orig.txt", typeFlag: reg, contents: c4Replaced},
		),
		// case 5: the uv/pip shape that motivated #665. installed name hardlinked into a cache, then
		// replaced by a symlink aimed at the cache name. squashed, the two names name each other.
		layerFromTarEntries(t,
			tarEntry{path: "c5/bin/tool", typeFlag: reg, contents: c5Payload},
			tarEntry{path: "c5/cache/blob", typeFlag: lnk, linkPath: "c5/bin/tool"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c5/bin/tool", typeFlag: sym, linkPath: "/c5/cache/blob"},
		),
		// case 6: `ln` onto a symlink. the hardlink header names a SYMLINK member.
		layerFromTarEntries(t,
			tarEntry{path: "c6/hard-to-sym.txt", typeFlag: sym, linkPath: "/c6/real.txt"},
			tarEntry{path: "c6/real.txt", typeFlag: reg, contents: c6Real},
			tarEntry{path: "c6/sym.txt", typeFlag: lnk, linkPath: "c6/hard-to-sym.txt"},
		),
		// case 7: the hardlink-header name is deleted; the data-carrying name survives.
		layerFromTarEntries(t,
			tarEntry{path: "c7/hard.txt", typeFlag: reg, contents: c7Payload},
			tarEntry{path: "c7/orig.txt", typeFlag: lnk, linkPath: "c7/hard.txt"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c7/.wh.orig.txt", typeFlag: reg},
		),
		// case 8: three names, one inode: one REG header and two HARDLINK headers.
		layerFromTarEntries(t,
			tarEntry{path: "c8/a.txt", typeFlag: reg, contents: c8Payload},
			tarEntry{path: "c8/b.txt", typeFlag: lnk, linkPath: "c8/a.txt"},
			tarEntry{path: "c8/c.txt", typeFlag: lnk, linkPath: "c8/a.txt"},
		),
		// case 9: the data-carrying name is overwritten in a later layer. on disk the other name
		// keeps the original inode, so the two names diverge.
		layerFromTarEntries(t,
			tarEntry{path: "c9/hard.txt", typeFlag: reg, contents: c9Original},
			tarEntry{path: "c9/orig.txt", typeFlag: lnk, linkPath: "c9/hard.txt"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c9/hard.txt", typeFlag: reg, contents: c9Overwrite},
		),
		// case 10: the data-carrying name is whiteouted, leaving a hardlink header whose linkname is
		// gone from the squashed tree. the container still serves the file under the other name.
		layerFromTarEntries(t,
			tarEntry{path: "c10/a.txt", typeFlag: reg, contents: c10Payload},
			tarEntry{path: "c10/z.txt", typeFlag: lnk, linkPath: "c10/a.txt"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c10/.wh.a.txt", typeFlag: reg},
		),
		// case 11: the data-carrying name is replaced in a later layer.
		layerFromTarEntries(t,
			tarEntry{path: "c11/a.txt", typeFlag: reg, contents: c11Payload},
			tarEntry{path: "c11/z.txt", typeFlag: lnk, linkPath: "c11/a.txt"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "c11/a.txt", typeFlag: reg, contents: c11Replaced},
		),
	)
}

func hardlinkContentsForTest(img *Image, path string) (string, error) {
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

// TestHardLinkSemantics_LayerSizeExcludesHardLinkHeaders asserts that a hardlinked name adds nothing
// to the reported layer size. A hardlink header carries no data section, so counting the size of the
// inode it names counts those bytes once per name: busybox:latest has 410 hardlink headers over 16
// regular ones, which turns a 4 MB layer into a reported 431 MB.
func TestHardLinkSemantics_LayerSizeExcludesHardLinkHeaders(t *testing.T) {
	const payload = "0123456789"

	img := readImageFromLayers(t,
		layerFromTarEntries(t,
			tarEntry{path: "a.txt", typeFlag: tar.TypeReg, contents: payload},
			tarEntry{path: "b.txt", typeFlag: tar.TypeLink, linkPath: "a.txt"},
			tarEntry{path: "c.txt", typeFlag: tar.TypeLink, linkPath: "a.txt"},
		),
	)

	require.Len(t, img.Layers, 1)
	assert.Equal(t, int64(len(payload)), img.Layers[0].Metadata.Size,
		"layer size must count tar data sections, and only the regular header has one")
	assert.Equal(t, int64(len(payload)), img.Metadata.Size, "image size is the sum of its layer sizes")

	// ...while the entry for a hardlinked name still describes the inode it names, so this cannot be
	// "fixed" by dropping adoption
	entry := catalogEntryForTest(t, img, "/b.txt")
	assert.Equal(t, int64(len(payload)), entry.Metadata.Size(),
		"a hardlinked name still reports the size of the file it names")
}

// catalogEntryForTest returns the catalog entry for a path's OWN node in the squashed tree, with no
// link following.
func catalogEntryForTest(t *testing.T, img *Image, path string) filetree.IndexEntry {
	t.Helper()

	exists, resolution, err := img.SquashedTree().File(file.Path(path))
	require.NoError(t, err)
	require.True(t, exists, "%s is not in the squashed tree", path)
	require.NotNil(t, resolution.Reference)

	entry, err := img.FileCatalog.Get(*resolution.Reference)
	require.NoError(t, err)

	return entry
}
