package image

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

func TestImageAdditionalMetadata(t *testing.T) {
	theTag, err := name.NewTag("a/tag:latest")
	if err != nil {
		t.Fatalf("could not create a tag: %+v", err)
	}

	tests := []struct {
		name    string
		options []AdditionalMetadata
		image   Image
	}{
		{
			name:    "no options",
			options: []AdditionalMetadata{},
			image:   Image{},
		},
		{
			name: "with tags",
			options: []AdditionalMetadata{
				WithTags(theTag.String()),
			},
			image: Image{
				Metadata: Metadata{
					Tags: []name.Tag{theTag},
				},
			},
		},
		{
			name: "with manifest",
			options: []AdditionalMetadata{
				WithManifest([]byte("some bytes")),
			},
			image: Image{
				Metadata: Metadata{
					RawManifest:    []byte("some bytes"),
					ManifestDigest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("some bytes"))),
				},
			},
		},
		{
			name: "with manifest digest",
			options: []AdditionalMetadata{
				WithManifestDigest("the-digest"),
			},
			image: Image{
				Metadata: Metadata{
					ManifestDigest: "the-digest",
				},
			},
		},
		{
			name: "with config",
			options: []AdditionalMetadata{
				WithConfig([]byte("some bytes")),
			},
			image: Image{
				Metadata: Metadata{
					RawConfig: []byte("some bytes"),
					ID:        fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("some bytes"))),
				},
			},
		},
		{
			name: "with platform",
			options: []AdditionalMetadata{
				WithPlatform("windows/arm64/v9"),
			},
			image: Image{
				Metadata: Metadata{
					OS:           "windows",
					Architecture: "arm64",
					Variant:      "v9",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempFile, err := os.CreateTemp("", "")
			if err != nil {
				t.Fatalf("could not create tempfile: %+v", err)
			}
			t.Cleanup(func() {
				os.Remove(tempFile.Name())
			})

			img := New(nil, nil, tempFile.Name(), test.options...)

			err = img.applyOverrideMetadata()
			if err != nil {
				t.Fatalf("could not create image: %+v", err)
			}
			if d := cmp.Diff(img, &test.image,
				cmpopts.IgnoreFields(Image{}, "FileCatalog"),
				cmpopts.IgnoreUnexported(Image{}),
				cmp.AllowUnexported(name.Tag{}, name.Repository{}, name.Registry{}),
			); d != "" {
				t.Errorf("diff: %+v", d)
			}
		})
	}
}

func TestImage_SquashedTree(t *testing.T) {
	t.Run("zero layers", func(t *testing.T) {
		i := Image{
			Layers: []*Layer{},
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panicked (and recovered) while computing squashed tree for image with zero layers: %v", r)
			}
		}()

		// Asserting that this call doesn't panic (regression: https://github.com/anchore/stereoscope/issues/56)
		result := i.SquashedTree()

		if result == nil {
			t.Error("expected an initialized, empty FileTree, but got a nil FileTree")
		}
	})
}

// a layer is a tar archive, so there are no inodes to share until the final filesystem is assembled: a hardlink
// reaches us as a header with typeflag '1' naming another header in the archive, carrying no data section of its own.
// Conventional writers emit the first name for a file as a regular header with the contents, and every later name as
// a link header with nothing after it:
//
//	header: name=a.txt  typeflag='0' size=20   [20 bytes of contents]
//	header: name=b.txt  typeflag='1' size=0    linkname=a.txt
//
// FileTree.AddHardLink binds the link to the file that already exists in the layer being built, so contents for both
// names come from the regular header. Nothing in the tar format enforces that ordering, so these cases also cover
// the header configurations we assume we will not see.
// note: the layers are written with archive/tar rather than generated with tar(1) (see testdata/generators) because
// some of these header configurations cannot be produced by a real tar writer.
const (
	installedExe = "opt/venv/lib/python3.13/site-packages/setuptools/cli-32.exe"
	cachedExe    = "root/.cache/uv/archive-v0/zdWisgG0ZvHnkidb/setuptools/cli-32.exe"
	sharedExe    = "usr/share/cli-32.exe"
	exeContents  = "cli-32.exe contents\n"
)

func TestImage_OpenPathFromSquash_hardLinkTarHeaderOrdering(t *testing.T) {
	tests := []struct {
		name string
		// entries are the tar headers of the top layer. The layer below always holds installedExe as a regular
		// file, which only matters for the case where the top layer has no regular header at all.
		entries   []tarEntry
		wantPaths []string
	}{
		{
			name: "regular header, then link header (what container builds emit)",
			entries: []tarEntry{
				{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
				{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
			},
			wantPaths: []string{installedExe, cachedExe},
		},
		{
			// the link header is read before the file it names, so there is nothing in the layer to bind to and
			// resolution falls back to following the link path
			name: "link header, then regular header",
			entries: []tarEntry{
				{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
				{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
			},
			wantPaths: []string{installedExe, cachedExe},
		},
		{
			// reading the link entry's own bytes would yield whatever follows it in the archive (padding, or the
			// next header), so the only safe contents for that path are the ones belonging to the file it names.
			// note: the malformed header is written last on purpose. A tar reader consumes the claimed data section
			// before looking for the next header, so this entry placed first would swallow the header that follows
			// it and that file would be absent from the layer entirely.
			name: "link header claiming a data section it does not have",
			entries: []tarEntry{
				{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
				{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe, claimedSize: int64(len(exeContents))},
			},
			wantPaths: []string{installedExe, cachedExe},
		},
		{
			// no regular header at all: neither name can bind within the layer, both fall back to following the
			// link path into the layer below
			name: "only link headers, naming a file from a lower layer",
			entries: []tarEntry{
				{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
				{path: sharedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
			},
			wantPaths: []string{installedExe, cachedExe, sharedExe},
		},
		{
			// conventional writers never do this (every name after the first links back to the first). Binding to
			// the intermediate link would name a tar entry that carries no contents.
			name: "link header naming another link header",
			entries: []tarEntry{
				{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
				{path: sharedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
				{path: cachedExe, typeFlag: tar.TypeLink, linkPath: sharedExe},
			},
			wantPaths: []string{installedExe, cachedExe, sharedExe},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := readImageFromLayers(t,
				layerFromTarEntries(t, tarEntry{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents}),
				layerFromTarEntries(t, tt.entries...),
			)

			for _, p := range tt.wantPaths {
				assert.Equal(t, exeContents, contentsFromSquash(t, img, "/"+p), "unexpected contents for %q", p)
			}
		})
	}
}

// TestImage_OpenPathFromSquash_hardLinkTargetReplacedBySymlink is the scenario from
// TestUnionFileTree_Squash_hardLinkTargetReplacedBySymlink read from real layer tars: uv installs a file and
// hardlinks it into its cache, and a later layer replaces the installed name with a symlink aimed at the cache name.
func TestImage_OpenPathFromSquash_hardLinkTargetReplacedBySymlink(t *testing.T) {
	img := readImageFromLayers(t,
		layerFromTarEntries(t,
			tarEntry{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
			tarEntry{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
		),
		layerFromTarEntries(t,
			tarEntry{path: installedExe, typeFlag: tar.TypeSymlink, linkPath: "/" + cachedExe},
		),
	)

	t.Run("contents are fetched through the hardlink name", func(t *testing.T) {
		assert.Equal(t, exeContents, contentsFromSquash(t, img, "/"+cachedExe))
	})

	t.Run("contents are fetched through the symlink that replaced the file", func(t *testing.T) {
		assert.Equal(t, exeContents, contentsFromSquash(t, img, "/"+installedExe))
	})

	t.Run("both names resolve to the reference for the original file", func(t *testing.T) {
		installed, err := img.SquashedSearchContext.SearchByPath("/" + installedExe)
		require.NoError(t, err)
		require.True(t, installed.HasReference())

		cached, err := img.SquashedSearchContext.SearchByPath("/" + cachedExe)
		require.NoError(t, err)
		require.True(t, cached.HasReference())

		assert.Equal(t, file.Path("/"+installedExe), installed.Reference.RealPath)
		assert.Equal(t, installed.Reference.ID(), cached.Reference.ID())
	})

	t.Run("catalog metadata for the hardlink name still describes the tar header", func(t *testing.T) {
		// reading the hardlink path yields the contents of the file it names, while the metadata for that path is
		// what the tar header said: a hardlink of size zero. That belongs to tar layers rather than to stereoscope,
		// which reports what the layer contained.
		entries, err := img.FileCatalog.GetByBasename("cli-32.exe")
		require.NoError(t, err)

		var found bool
		for _, entry := range entries {
			if string(entry.RealPath) != "/"+cachedExe {
				continue
			}
			found = true
			assert.Equal(t, file.TypeHardLink, entry.Metadata.Type)
			assert.Equal(t, int64(0), entry.Metadata.Size())
			// the link destination is the raw tar header value, which is archive-relative
			assert.Equal(t, installedExe, entry.Metadata.LinkDestination)
		}
		require.True(t, found, "no catalog entry for the hardlink path")
	})
}

// tarEntry is a single tar header and the data section that follows it, if any.
type tarEntry struct {
	path     string
	typeFlag byte
	linkPath string
	contents string
	// claimedSize overrides the size written into the header, describing a header that claims a data section larger
	// than the bytes that actually follow it.
	claimedSize int64
}

func layerFromTarEntries(t *testing.T, entries ...tarEntry) v1.Layer {
	t.Helper()

	tarPath := filepath.Join(t.TempDir(), "layer.tar")
	fh, err := os.Create(tarPath)
	require.NoError(t, err)

	tw := tar.NewWriter(fh)
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:     entry.path,
			Typeflag: entry.typeFlag,
			Linkname: entry.linkPath,
			Mode:     0o644,
			Size:     int64(len(entry.contents)),
		}
		if entry.claimedSize != 0 {
			hdr.Size = entry.claimedSize
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if entry.contents != "" {
			_, err := io.WriteString(tw, entry.contents)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, fh.Close())

	layer, err := tarball.LayerFromFile(tarPath)
	require.NoError(t, err)

	return layer
}

func readImageFromLayers(t *testing.T, layers ...v1.Layer) *Image {
	t.Helper()

	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("image-test"), t.TempDir())
	require.NoError(t, img.Read())
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})

	return img
}

func contentsFromSquash(t *testing.T, img *Image, path string) string {
	t.Helper()

	reader, err := img.OpenPathFromSquash(file.Path(path))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	contents, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(contents)
}
