package image

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// Parity tests for indexing an uncompressed layer blob in place (see WithLayerBlobLocator).
//
// The claim these protect: where the bytes are read from is not part of what the indexer reports. Each case
// reads the same layers twice, once in place and once with disableInPlaceLayers set (today's copy into the
// cache dir), and requires the two reads to agree on layer metadata, trees, catalog entries and contents.

// rawTar builds an uncompressed tar from the entries, the bytes an uncompressed layer blob holds.
func rawTar(t testing.TB, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     e.path,
			Typeflag: e.typeFlag,
			Linkname: e.linkPath,
			Mode:     mode,
			Size:     int64(len(e.contents)),
			ModTime:  time.Unix(1700000000, 0),
		}))
		if e.contents != "" {
			_, err := io.WriteString(tw, e.contents)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

// inPlaceFixture is a set of layers plus the blobs a locator can serve for them.
type inPlaceFixture struct {
	layers []v1.Layer
	blobs  map[v1.Hash][]byte
}

// addUncompressed adds a layer whose blob is the raw tar, served by the locator.
func (f *inPlaceFixture) addUncompressed(t testing.TB, raw []byte) {
	t.Helper()
	layer := static.NewLayer(raw, types.OCIUncompressedLayer)
	digest, err := layer.Digest()
	require.NoError(t, err)
	if f.blobs == nil {
		f.blobs = map[v1.Hash][]byte{}
	}
	f.blobs[digest] = raw
	f.layers = append(f.layers, layer)
}

// addCompressed adds a gzip layer; it is never a candidate, so nothing is registered with the locator.
func (f *inPlaceFixture) addCompressed(t testing.TB, entries ...tarEntry) {
	t.Helper()
	f.layers = append(f.layers, layerFromTarEntries(t, entries...))
}

func readInPlaceFixture(t *testing.T, ctx context.Context, f inPlaceFixture, locator LayerBlobLocator, disableInPlace bool) *Image {
	t.Helper()
	v1Img, err := mutate.AppendLayers(empty.Image, f.layers...)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("in-place-parity"), t.TempDir(), WithLayerBlobLocator(locator))
	img.disableInPlaceLayers = disableInPlace
	t.Cleanup(func() { require.NoError(t, img.Cleanup()) })
	require.NoError(t, img.Read(ctx))
	return img
}

// indexTuple is what the indexer reports about one file, minus the reference ID (unique per read).
type indexTuple struct {
	RealPath        file.Path
	Type            file.Type
	Size            int64
	Mode            os.FileMode
	ModTime         time.Time
	UserID, GroupID int
	LinkDestination string
	MIMEType        string
	LayerIndex      uint
	ContentSHA256   string
}

func indexTuples(t *testing.T, tree filetree.Reader, catalog FileCatalogReader) []indexTuple {
	t.Helper()
	var out []indexTuple
	for _, ref := range tree.AllFiles(file.AllTypes()...) {
		entry, err := catalog.Get(ref)
		require.NoErrorf(t, err, "no catalog entry for %s", ref.RealPath)
		layer := catalog.Layer(ref)
		require.NotNilf(t, layer, "no layer for %s", ref.RealPath)

		tup := indexTuple{
			RealPath:        ref.RealPath,
			Type:            entry.Type,
			UserID:          entry.UserID,
			GroupID:         entry.GroupID,
			LinkDestination: entry.LinkDestination,
			MIMEType:        entry.MIMEType,
			LayerIndex:      layer.Metadata.Index,
		}
		if entry.FileInfo != nil {
			tup.Size, tup.Mode, tup.ModTime = entry.Size(), entry.Mode(), entry.ModTime()
		}
		if entry.Type == file.TypeRegular {
			rc, err := catalog.Open(ref)
			require.NoErrorf(t, err, "open %s", ref.RealPath)
			h := sha256.New()
			_, err = io.Copy(h, rc)
			require.NoError(t, err)
			require.NoError(t, rc.Close())
			tup.ContentSHA256 = fmt.Sprintf("%x", h.Sum(nil))
		}
		out = append(out, tup)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RealPath < out[j].RealPath })
	return out
}

// requireSameIndex asserts the copied and in-place reads report the same thing.
func requireSameIndex(t *testing.T, copied, inPlace *Image) {
	t.Helper()
	require.Len(t, inPlace.Layers, len(copied.Layers))
	assert.Equal(t, copied.Metadata.Size, inPlace.Metadata.Size, "image size")
	for idx := range copied.Layers {
		c, p := copied.Layers[idx], inPlace.Layers[idx]
		require.Equalf(t, c.Metadata, p.Metadata, "layer %d metadata", idx)
		require.Equalf(t, indexTuples(t, c.Tree, copied.FileCatalog), indexTuples(t, p.Tree, inPlace.FileCatalog),
			"layer %d tree", idx)
		require.Equalf(t, indexTuples(t, c.SquashedTree, copied.FileCatalog), indexTuples(t, p.SquashedTree, inPlace.FileCatalog),
			"layer %d squash", idx)
	}
}

func inPlaceFlags(img *Image) []bool {
	var out []bool
	for _, l := range img.Layers {
		out = append(out, l.contentInPlace)
	}
	return out
}

func Test_Image_inPlace_parityWithCopy(t *testing.T) {
	reg, dir, lnk, sym := byte(tar.TypeReg), byte(tar.TypeDir), byte(tar.TypeLink), byte(tar.TypeSymlink)

	tests := []struct {
		name  string
		build func(t *testing.T) inPlaceFixture
		// wantInPlace is the per-layer contentInPlace of the in-place read
		wantInPlace []bool
	}{
		{
			name: "single layer with every entry type",
			build: func(t *testing.T) (f inPlaceFixture) {
				f.addUncompressed(t, rawTar(t,
					tarEntry{path: "etc/", typeFlag: dir, mode: 0o700},
					tarEntry{path: "etc/hello.txt", typeFlag: reg, contents: "hello"},
					tarEntry{path: "etc/empty", typeFlag: reg},
					tarEntry{path: "bin/run", typeFlag: reg, contents: "#!/bin/sh\n", mode: 0o4755},
					tarEntry{path: "etc/hard", typeFlag: lnk, linkPath: "etc/hello.txt"},
					tarEntry{path: "alias", typeFlag: sym, linkPath: "etc"},
					tarEntry{path: "a/b/c/d/e/f/deep.txt", typeFlag: reg, contents: "deep"},
				))
				return f
			},
			wantInPlace: []bool{true},
		},
		{
			name: "overlap, whiteout and opaque dir across layers",
			build: func(t *testing.T) (f inPlaceFixture) {
				f.addUncompressed(t, rawTar(t,
					tarEntry{path: "shared.txt", typeFlag: reg, contents: "v1"},
					tarEntry{path: "gone.txt", typeFlag: reg, contents: "doomed"},
					tarEntry{path: "opq/", typeFlag: dir},
					tarEntry{path: "opq/hidden.txt", typeFlag: reg, contents: "hidden"},
				))
				f.addUncompressed(t, rawTar(t,
					tarEntry{path: "shared.txt", typeFlag: reg, contents: "v2"},
					tarEntry{path: ".wh.gone.txt", typeFlag: reg},
					tarEntry{path: "opq/", typeFlag: dir},
					tarEntry{path: "opq/.wh..wh..opq", typeFlag: reg},
					tarEntry{path: "opq/kept.txt", typeFlag: reg, contents: "kept"},
					tarEntry{path: "lower-link", typeFlag: lnk, linkPath: "shared.txt"},
				))
				return f
			},
			wantInPlace: []bool{true, true},
		},
		{
			// D4: an empty tar is just its two zero end-of-archive blocks
			name: "empty tar layer",
			build: func(t *testing.T) (f inPlaceFixture) {
				f.addUncompressed(t, make([]byte, 1024))
				f.addUncompressed(t, rawTar(t, tarEntry{path: "after-empty.txt", typeFlag: reg, contents: "x"}))
				return f
			},
			wantInPlace: []bool{true, true},
		},
		{
			// D8: per-layer choice, the gzip layer in the middle is copied
			name: "mixed uncompressed and gzip layers",
			build: func(t *testing.T) (f inPlaceFixture) {
				f.addUncompressed(t, rawTar(t, tarEntry{path: "a.txt", typeFlag: reg, contents: "a0"}))
				f.addCompressed(t,
					tarEntry{path: "a.txt", typeFlag: reg, contents: "a1"},
					tarEntry{path: "b.txt", typeFlag: reg, contents: "b1"},
				)
				f.addUncompressed(t, rawTar(t,
					tarEntry{path: ".wh.b.txt", typeFlag: reg},
					tarEntry{path: "c.txt", typeFlag: reg, contents: "c2"},
				))
				return f
			},
			wantInPlace: []bool{true, false, true},
		},
		{
			// D9: the same blob used for two layers; both point at one read-only file, each with its own descriptor
			name: "repeated layer digest",
			build: func(t *testing.T) (f inPlaceFixture) {
				raw := rawTar(t, tarEntry{path: "same.txt", typeFlag: reg, contents: "same bytes"})
				f.addUncompressed(t, raw)
				f.addUncompressed(t, rawTar(t, tarEntry{path: "between.txt", typeFlag: reg, contents: "b"}))
				f.addUncompressed(t, raw)
				return f
			},
			wantInPlace: []bool{true, true, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.build(t)
			locator := blobDirLocator(t, f.blobs)

			copied := readInPlaceFixture(t, context.Background(), f, locator, true)
			inPlace := readInPlaceFixture(t, context.Background(), f, locator, false)

			assert.Equal(t, make([]bool, len(f.layers)), inPlaceFlags(copied), "the hook must force every layer to copy")
			require.Equal(t, tt.wantInPlace, inPlaceFlags(inPlace))
			requireSameIndex(t, copied, inPlace)

			// the squash reads back the same bytes too
			for _, p := range copied.SquashedTree().AllRealPaths() {
				want, wantErr := readAllFromSquash(copied, p)
				got, gotErr := readAllFromSquash(inPlace, p)
				assert.Equalf(t, wantErr != nil, gotErr != nil, "squash open error for %s", p)
				assert.Equalf(t, want, got, "squash contents of %s", p)
			}
		})
	}
}

func readAllFromSquash(img *Image, p file.Path) ([]byte, error) {
	rc, err := img.OpenPathFromSquash(p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// D11: the fetch and index stages overlap across layers; an in-place layer skips the copy and hands its
// index stage a path nobody in this process wrote. Run under -race in CI.
func Test_Image_inPlace_parityUnderConcurrency(t *testing.T) {
	const n = 6
	var f inPlaceFixture
	for i := 0; i < n; i++ {
		f.addUncompressed(t, rawTar(t,
			tarEntry{path: fmt.Sprintf("only-%d.txt", i), typeFlag: byte(tar.TypeReg), contents: fmt.Sprintf("u%d", i)},
			tarEntry{path: "shared.txt", typeFlag: byte(tar.TypeReg), contents: fmt.Sprintf("v%d", i)},
			tarEntry{path: fmt.Sprintf("link-%d", i), typeFlag: byte(tar.TypeSymlink), linkPath: fmt.Sprintf("only-%d.txt", i)},
		))
	}
	locator := blobDirLocator(t, f.blobs)

	for _, c := range []int{1, 4} {
		t.Run(fmt.Sprintf("concurrency-%d", c), func(t *testing.T) {
			copied := readInPlaceFixture(t, layerConcurrency(c, c), f, locator, true)
			inPlace := readInPlaceFixture(t, layerConcurrency(c, c), f, locator, false)

			want := make([]bool, n)
			for i := range want {
				want[i] = true
			}
			require.Equal(t, want, inPlaceFlags(inPlace))
			requireSameIndex(t, copied, inPlace)

			got, err := readAllFromSquash(inPlace, "/shared.txt")
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("v%d", n-1), string(got))
		})
	}
}

// C3: SquashFS layers are a different reader entirely and are never offered to the locator.
func Test_Layer_fetch_singularityNeverLocated(t *testing.T) {
	consulted := false
	locator := func(v1.Hash) (string, bool) {
		consulted = true
		return "", false
	}

	l := newLayer(fakeLayer(SingularitySquashFSLayer, nil), "", locator)
	l.Metadata = LayerMetadata{MediaType: SingularitySquashFSLayer}

	_, ok := l.inPlacePath()
	assert.False(t, ok)
	assert.False(t, consulted, "a SquashFS layer must not be offered to the locator")
}

// C4: a locator miss falls back to the copy and the full read still succeeds.
func Test_Image_inPlace_locatorMissFallsBack(t *testing.T) {
	var f inPlaceFixture
	f.addUncompressed(t, rawTar(t, tarEntry{path: "x.txt", typeFlag: byte(tar.TypeReg), contents: "x"}))
	miss := func(v1.Hash) (string, bool) { return "", false }

	img := readInPlaceFixture(t, context.Background(), f, miss, false)
	require.Equal(t, []bool{false}, inPlaceFlags(img))
	got, err := readAllFromSquash(img, "/x.txt")
	require.NoError(t, err)
	assert.Equal(t, "x", string(got))
}

// A: a path that cannot be opened is reported as an error, which inPlacePath turns into the copy.
func Test_looksLikeUncompressedTar_unreadable(t *testing.T) {
	ok, err := looksLikeUncompressedTar(t.TempDir() + "/does-not-exist")
	assert.False(t, ok)
	assert.Error(t, err)
}

// D3: a blob labelled uncompressed that is really gzip is rejected by the magic check, so the in-place read
// takes the copy path and ends exactly as a read with in-place disabled does, success or failure alike.
func Test_Image_inPlace_mislabelledBlobMatchesCopy(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, err := zw.Write(rawTar(t, tarEntry{path: "x.txt", typeFlag: byte(tar.TypeReg), contents: "x"}))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	layer := static.NewLayer(gz.Bytes(), types.OCIUncompressedLayer)
	digest, err := layer.Digest()
	require.NoError(t, err)
	locator := blobDirLocator(t, map[v1.Hash][]byte{digest: gz.Bytes()})

	read := func(disable bool) (*Image, error) {
		v1Img, err := mutate.AppendLayers(empty.Image, layer)
		require.NoError(t, err)
		img := New(v1Img, file.NewTempDirGenerator("mislabelled"), t.TempDir(), WithLayerBlobLocator(locator))
		img.disableInPlaceLayers = disable
		t.Cleanup(func() { _ = img.Cleanup() })
		return img, img.Read(context.Background())
	}

	copied, copyErr := read(true)
	inPlace, inPlaceErr := read(false)

	require.Equal(t, copyErr == nil, inPlaceErr == nil, "copy err=%v, in-place err=%v", copyErr, inPlaceErr)
	for _, l := range inPlace.Layers {
		assert.False(t, l.contentInPlace, "a gzip blob must never be indexed in place")
	}
	if copyErr == nil {
		requireSameIndex(t, copied, inPlace)
	}
}
