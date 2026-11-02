package image

import (
	"archive/tar"
	"context"
	"crypto"
	"crypto/md5"  //nolint:gosec
	"crypto/sha1" //nolint:gosec
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// digestsOf is the expected Metadata.Digests for the given content when MD5, SHA-1 and SHA-256
// were requested, in that order.
func digestsOf(contents string) []file.Digest {
	return []file.Digest{
		{Algorithm: "md5", Value: fmt.Sprintf("%x", md5.Sum([]byte(contents)))},   //nolint:gosec
		{Algorithm: "sha1", Value: fmt.Sprintf("%x", sha1.Sum([]byte(contents)))}, //nolint:gosec
		{Algorithm: "sha256", Value: fmt.Sprintf("%x", sha256.Sum256([]byte(contents)))},
	}
}

// readImageFromLayersWithDigests is readImageFromLayers with file digest computation requested.
func readImageFromLayersWithDigests(t *testing.T, layers ...tarEntry) *Image {
	t.Helper()

	v1Img, err := mutate.AppendLayers(empty.Image, layerFromTarEntries(t, layers...))
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("image-test"), t.TempDir(),
		WithFileDigestAlgorithms(crypto.MD5, crypto.SHA1, crypto.SHA256))
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})
	require.NoError(t, img.Read(context.Background()))

	return img
}

func TestFileDigests_ComputedDuringIndexing(t *testing.T) {
	reg := byte(tar.TypeReg)
	lnk := byte(tar.TypeLink)
	sym := byte(tar.TypeSymlink)
	dir := byte(tar.TypeDir)

	img := readImageFromLayersWithDigests(t,
		tarEntry{path: "d/", typeFlag: dir},
		tarEntry{path: "d/a.txt", typeFlag: reg, contents: "first file"},
		tarEntry{path: "d/b.txt", typeFlag: lnk, linkPath: "d/a.txt"},
		tarEntry{path: "d/empty.txt", typeFlag: reg},
		tarEntry{path: "d/sym.txt", typeFlag: sym, linkPath: "/d/a.txt"},
	)

	// regular files carry the digests of their own contents, in the requested algorithm order
	assert.Equal(t, digestsOf("first file"), catalogEntryForTest(t, img, "/d/a.txt").Metadata.Digests)

	// empty regular files carry the digests of empty input, they are not skipped
	assert.Equal(t, digestsOf(""), catalogEntryForTest(t, img, "/d/empty.txt").Metadata.Digests)

	// a hardlink is a second name for its target's inode, so it carries the target's digests
	assert.Equal(t, digestsOf("first file"), catalogEntryForTest(t, img, "/d/b.txt").Metadata.Digests)

	// non-regular entries carry none
	assert.Nil(t, catalogEntryForTest(t, img, "/d").Metadata.Digests)
	assert.Nil(t, catalogEntryForTest(t, img, "/d/sym.txt").Metadata.Digests)
}

func TestFileDigests_UnavailableAlgorithmDoesNotFailIndexing(t *testing.T) {
	reg := byte(tar.TypeReg)

	// crypto.MD4 is never registered, so digest computation fails for every file; indexing must
	// carry on and leave the digests unset for consumers to fall back on
	v1Img, err := mutate.AppendLayers(empty.Image, layerFromTarEntries(t,
		tarEntry{path: "d/a.txt", typeFlag: reg, contents: "first file"},
	))
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("image-test"), t.TempDir(),
		WithFileDigestAlgorithms(crypto.MD4))
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})
	require.NoError(t, img.Read(context.Background()))

	entry := catalogEntryForTest(t, img, "/d/a.txt")
	assert.Nil(t, entry.Metadata.Digests)
	assert.Equal(t, file.TypeRegular, entry.Metadata.Type)
}

func TestFileDigests_OffByDefault(t *testing.T) {
	reg := byte(tar.TypeReg)

	img := readImageFromLayers(t,
		layerFromTarEntries(t,
			tarEntry{path: "d/a.txt", typeFlag: reg, contents: "first file"},
		),
	)

	assert.Nil(t, catalogEntryForTest(t, img, "/d/a.txt").Metadata.Digests)
}
