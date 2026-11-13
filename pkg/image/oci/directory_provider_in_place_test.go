package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// tarBlob is a minimal uncompressed layer tar.
func tarBlob(t testing.TB) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "etc/hello.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 5}))
	_, err := tw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// writeLayout writes an OCI layout holding one layer whose blob is the given bytes stored as-is,
// which is the shape `skopeo copy --dest-decompress` leaves behind.
func writeLayout(t testing.TB, mt types.MediaType, blob []byte) (string, v1.Hash) {
	t.Helper()

	layer := static.NewLayer(blob, mt)
	digest, err := layer.Digest()
	require.NoError(t, err)

	img, err := mutate.AppendLayers(empty.Image, layer)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "layout")
	lp, err := layout.Write(path, empty.Index)
	require.NoError(t, err)
	require.NoError(t, lp.AppendImage(img))

	return path, digest
}

func layoutBlobPath(layoutPath string, h v1.Hash) string {
	return filepath.Join(layoutPath, "blobs", h.Algorithm, h.Hex)
}

// firstImageInLayout pulls the one image the test layouts hold out of the root index.
func firstImageInLayout(layoutPath string) (v1.Image, error) {
	idx, err := layout.ImageIndexFromPath(layoutPath)
	if err != nil {
		return nil, err
	}
	im, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(im.Manifests) == 0 {
		return nil, fmt.Errorf("layout has no manifests")
	}
	return idx.Image(im.Manifests[0].Digest)
}

func Test_layoutBlobLocator(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "blobs", "sha256", "aa"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256", "bb"), 0o755))

	locator := layoutBlobLocator(root)

	tests := []struct {
		name string
		hash v1.Hash
		want string
	}{
		{name: "blob present", hash: v1.Hash{Algorithm: "sha256", Hex: "aa"}, want: filepath.Join(root, "blobs", "sha256", "aa")},
		{name: "a directory is not a blob", hash: v1.Hash{Algorithm: "sha256", Hex: "bb"}},
		{name: "missing blob", hash: v1.Hash{Algorithm: "sha256", Hex: "cc"}},
		{name: "unknown algorithm", hash: v1.Hash{Algorithm: "sha512", Hex: "aa"}},
		{name: "path traversal in hex", hash: v1.Hash{Algorithm: "sha256", Hex: "../sha256/aa"}},
		{name: "path traversal in algorithm", hash: v1.Hash{Algorithm: "../..", Hex: "aa"}},
		{name: "empty hex", hash: v1.Hash{Algorithm: "sha256"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := locator(tt.hash)
			assert.Equal(t, tt.want != "", ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// An uncompressed layout is indexed out of blobs/ directly: the read consults the locator, the cache
// dir stays empty, the blob is byte-identical afterwards, and Cleanup does not remove it.
func Test_Image_read_uncompressedLayoutIsIndexedInPlace(t *testing.T) {
	blob := tarBlob(t)
	path, layerDigest := writeLayout(t, types.OCIUncompressedLayer, blob)

	consulted := []v1.Hash{}
	locator := func(h v1.Hash) (string, bool) {
		consulted = append(consulted, h)
		return layoutBlobLocator(path)(h)
	}

	cacheDir := t.TempDir()
	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	v1Img, err := firstImageInLayout(path)
	require.NoError(t, err)

	subject := image.New(v1Img, tmpDirGen, cacheDir, image.WithLayerBlobLocator(locator))
	require.NoError(t, subject.Read(context.Background()))

	require.Len(t, subject.Layers, 1)
	assert.Equal(t, []v1.Hash{layerDigest}, consulted, "the read should ask the layout for the layer blob")

	rc, err := subject.Layers[0].OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got := new(bytes.Buffer)
	_, err = got.ReadFrom(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", got.String())

	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "an in-place layer must not be copied into the cache dir")

	require.NoError(t, subject.Cleanup())

	after, err := os.ReadFile(layoutBlobPath(path, layerDigest))
	require.NoError(t, err)
	assert.Equal(t, blob, after, "Cleanup must leave the source blob alone")
}

// A compressed layer is not a candidate, so the layout is never consulted and the copy is made.
func Test_Image_read_compressedLayerIsCopied(t *testing.T) {
	path, _ := writeLayout(t, types.OCILayer, gzipBytes(t, tarBlob(t)))

	consulted := false
	locator := func(v1.Hash) (string, bool) {
		consulted = true
		return "", false
	}

	cacheDir := t.TempDir()
	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	v1Img, err := firstImageInLayout(path)
	require.NoError(t, err)

	subject := image.New(v1Img, tmpDirGen, cacheDir, image.WithLayerBlobLocator(locator))
	require.NoError(t, subject.Read(context.Background()))
	defer func() { require.NoError(t, subject.Cleanup()) }()

	require.Len(t, subject.Layers, 1)
	assert.False(t, consulted, "a compressed layer must not be handed to the locator")

	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the uncompressed copy is still made")
}

func Test_Directory_Provider_uncompressedLayoutReads(t *testing.T) {
	blob := tarBlob(t)
	path, layerDigest := writeLayout(t, types.OCIUncompressedLayer, blob)

	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	subject, err := NewDirectoryProvider(tmpDirGen, path).Provide(context.Background())
	require.NoError(t, err)
	require.Len(t, subject.Layers, 1)

	rc, err := subject.Layers[0].OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got := new(bytes.Buffer)
	_, err = got.ReadFrom(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", got.String())

	require.NoError(t, subject.Cleanup())

	after, err := os.ReadFile(layoutBlobPath(path, layerDigest))
	require.NoError(t, err)
	assert.Equal(t, blob, after, "reading and cleaning up must not touch the layout")
}

// The archive provider untars to a temp dir and reuses the directory provider, so the blobs it finds
// are in that temp dir; the source tarball is never a candidate and must be untouched.
func Test_Archive_Provider_uncompressedLayoutReads(t *testing.T) {
	path, _ := writeLayout(t, types.OCIUncompressedLayer, tarBlob(t))

	tarPath := filepath.Join(t.TempDir(), "image.tar")
	tarballFromDir(t, path, tarPath)
	before, err := os.ReadFile(tarPath)
	require.NoError(t, err)

	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	subject, err := NewArchiveProvider(tmpDirGen, tarPath).Provide(context.Background())
	require.NoError(t, err)
	defer func() { require.NoError(t, subject.Cleanup()) }()

	require.Len(t, subject.Layers, 1)
	rc, err := subject.Layers[0].OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got := new(bytes.Buffer)
	_, err = got.ReadFrom(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", got.String())

	after, err := os.ReadFile(tarPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func tarballFromDir(t testing.TB, dir, dest string) {
	t.Helper()
	out, err := os.Create(dest)
	require.NoError(t, err)
	tw := tar.NewWriter(out)
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		fh, err := os.Open(p)
		if err != nil {
			return err
		}
		defer fh.Close()
		_, err = io.Copy(tw, fh)
		return err
	}))
	require.NoError(t, tw.Close())
	require.NoError(t, out.Close())
}

func gzipBytes(t testing.TB, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(b)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
