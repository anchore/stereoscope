package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tarBytes(t testing.TB, format tar.Format) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "etc/hello.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 5, Format: format}
	if format == tar.FormatPAX {
		hdr.PAXRecords = map[string]string{"comment": "pax"}
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return buf.Bytes()
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

// v7Tar is a pre-POSIX header: name, mode and size set, no "ustar" magic at 257
func v7Tar() []byte {
	b := make([]byte, 1024)
	copy(b, "etc/hello.txt")
	copy(b[100:], "0000644\x00")
	copy(b[124:], "00000000000\x00")
	return b
}

func Test_looksLikeUncompressedTar(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{name: "ustar", content: tarBytes(t, tar.FormatUSTAR), want: true},
		{name: "GNU", content: tarBytes(t, tar.FormatGNU), want: true},
		{name: "PAX", content: tarBytes(t, tar.FormatPAX), want: true},
		{name: "empty file", content: nil, want: true},
		{name: "empty tar (two zero blocks)", content: make([]byte, 1024), want: true},
		{name: "short and not zero", content: []byte("not a tar at all"), want: false},
		{name: "gzip", content: gzipBytes(t, tarBytes(t, tar.FormatUSTAR)), want: false},
		{name: "zstd magic", content: append([]byte{0x28, 0xb5, 0x2f, 0xfd}, make([]byte, 600)...), want: false},
		{name: "bzip2 magic", content: append([]byte("BZh9"), make([]byte, 600)...), want: false},
		{name: "xz magic", content: append([]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, make([]byte, 600)...), want: false},
		{name: "v7 tar has no magic", content: v7Tar(), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "blob")
			require.NoError(t, os.WriteFile(p, tt.content, 0o600))
			got, err := looksLikeUncompressedTar(p)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("unreadable path", func(t *testing.T) {
		got, err := looksLikeUncompressedTar(filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
		assert.False(t, got)
	})
}

// blobDirLocator serves blobs written into a directory by digest hex, the way a layout would.
func blobDirLocator(t testing.TB, blobs map[v1.Hash][]byte) LayerBlobLocator {
	t.Helper()
	dir := t.TempDir()
	for h, b := range blobs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, h.Hex), b, 0o600))
	}
	return func(h v1.Hash) (string, bool) {
		p := filepath.Join(dir, h.Hex)
		if _, err := os.Stat(p); err != nil {
			return "", false
		}
		return p, true
	}
}

func Test_Layer_fetch_inPlace(t *testing.T) {
	raw := tarBytes(t, tar.FormatUSTAR)
	gz := gzipBytes(t, raw)

	tests := []struct {
		name        string
		layer       v1.Layer
		stored      []byte // what the locator serves for the layer's digest; nil means not found
		noLocator   bool
		wantInPlace bool
	}{
		{name: "uncompressed OCI blob", layer: static.NewLayer(raw, types.OCIUncompressedLayer), stored: raw, wantInPlace: true},
		{name: "uncompressed restricted OCI blob", layer: static.NewLayer(raw, types.OCIUncompressedRestrictedLayer), stored: raw, wantInPlace: true},
		{name: "uncompressed docker blob", layer: static.NewLayer(raw, types.DockerUncompressedLayer), stored: raw, wantInPlace: true},
		{name: "no locator", layer: static.NewLayer(raw, types.OCIUncompressedLayer), stored: raw, noLocator: true},
		{name: "compressed media type", layer: static.NewLayer(gz, types.OCILayer), stored: gz},
		{name: "compressed docker media type", layer: static.NewLayer(gz, types.DockerLayer), stored: gz},
		{name: "blob not found", layer: static.NewLayer(raw, types.OCIUncompressedLayer)},
		{name: "labelled uncompressed but gzip", layer: static.NewLayer(gz, types.OCIUncompressedLayer), stored: gz},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest, err := tt.layer.Digest()
			require.NoError(t, err)
			blobs := map[v1.Hash][]byte{}
			if tt.stored != nil {
				blobs[digest] = tt.stored
			}
			var locator LayerBlobLocator
			if !tt.noLocator {
				locator = blobDirLocator(t, blobs)
			}

			cacheDir := t.TempDir()
			l := newLayer(tt.layer, "", locator)
			require.NoError(t, l.fetch(0, cacheDir))

			assert.Equal(t, tt.wantInPlace, l.contentInPlace)
			cached, err := os.ReadDir(cacheDir)
			require.NoError(t, err)
			if tt.wantInPlace {
				wantPath, _ := locator(digest)
				assert.Equal(t, wantPath, l.contentPath)
				assert.Empty(t, cached, "an in-place layer must not be copied into the cache")
			} else {
				assert.Equal(t, cacheDir, filepath.Dir(l.contentPath))
				assert.Len(t, cached, 1)
			}
		})
	}
}

func Test_Layer_inPlace_indexesAndReads(t *testing.T) {
	raw := tarBytes(t, tar.FormatUSTAR)
	layer := static.NewLayer(raw, types.OCIUncompressedLayer)
	digest, err := layer.Digest()
	require.NoError(t, err)
	locator := blobDirLocator(t, map[v1.Hash][]byte{digest: raw})
	blobPath, _ := locator(digest)

	l := newLayer(layer, "", locator)
	require.NoError(t, l.Read(NewFileCatalog(), 0, t.TempDir()))
	t.Cleanup(func() { require.NoError(t, l.close()) })

	require.True(t, l.contentInPlace)
	rc, err := l.OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got := new(bytes.Buffer)
	_, err = got.ReadFrom(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", got.String())

	after, err := os.ReadFile(blobPath)
	require.NoError(t, err)
	assert.Equal(t, raw, after, "the source blob must be untouched")
}
