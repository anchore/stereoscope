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
	"github.com/scylladb/go-set/strset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
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

// uv and pip can write a file and then hardlink it into a content-addressed cache
// a later layer can replace the installed name with a symlink aimed at the cache name
// See TestUnionFileTree_Squash_hardLinkTargetReplacedBySymlink.
//
//	header: name=opt/venv/.../cli-32.exe     typeflag='0' size=20   [20 bytes of contents]
//	header: name=root/.cache/.../cli-32.exe  typeflag='1' size=0    linkname=opt/venv/.../cli-32.exe
//
// Binding at FileTree.AddHardLink time is what turns them back into two names for one file, and these tests exercise
// that from real layer tars through each public content and link resolution API.
const (
	installedExe = "opt/venv/lib/python3.13/site-packages/setuptools/cli-32.exe"
	cachedExe    = "root/.cache/uv/archive-v0/zdWisgG0ZvHnkidb/setuptools/cli-32.exe"
	sharedExe    = "usr/share/cli-32.exe"
	exeContents  = "cli-32.exe contents\n"
)

func TestImage_OpenPathFromSquash_hardLinkTargetReplacedBySymlink(t *testing.T) {
	img := readImageWithHardLinkReplacedBySymlink(t)

	t.Run("contents are fetched through the hardlink name", func(t *testing.T) {
		assert.Equal(t, exeContents, contentsFromPath(t, img.OpenPathFromSquash, "/"+cachedExe))
	})

	t.Run("contents are fetched through the symlink that replaced the file", func(t *testing.T) {
		assert.Equal(t, exeContents, contentsFromPath(t, img.OpenPathFromSquash, "/"+installedExe))
	})
}

func TestLayer_OpenPathFromSquash_hardLinkTargetReplacedBySymlink(t *testing.T) {
	img := readImageWithHardLinkReplacedBySymlink(t)
	require.Len(t, img.Layers, 2)

	// the squashed tree of the layer that replaced the file is where the two names would point at each other
	for _, p := range []string{installedExe, cachedExe} {
		assert.Equal(t, exeContents, contentsFromPath(t, img.Layers[1].OpenPathFromSquash, "/"+p), "unexpected contents for %q", p)
	}
}

// TestImage_SquashedSearchContext_hardLinkTargetReplacedBySymlink covers the index-backed search paths, which resolve
// each candidate path found in the catalog against the squashed tree.
func TestImage_SquashedSearchContext_hardLinkTargetReplacedBySymlink(t *testing.T) {
	img := readImageWithHardLinkReplacedBySymlink(t)

	t.Run("by path", func(t *testing.T) {
		installed, err := img.SquashedSearchContext.SearchByPath("/" + installedExe)
		require.NoError(t, err)
		require.True(t, installed.HasReference())

		cached, err := img.SquashedSearchContext.SearchByPath("/" + cachedExe)
		require.NoError(t, err)
		require.True(t, cached.HasReference())

		assert.Equal(t, file.Path("/"+installedExe), installed.RealPath)
		assert.Equal(t, installed.ID(), cached.ID())
	})

	t.Run("by glob", func(t *testing.T) {
		resolutions, err := img.SquashedSearchContext.SearchByGlob("**/*.exe")
		require.NoError(t, err)

		// note: the catalog holds an entry per tar header, so the installed name is searched twice (once for the
		// regular header, once for the symlink header that replaced it) and resolves to the same file both times
		requestPaths := strset.New()
		for _, resolution := range resolutions {
			requestPaths.Add(string(resolution.RequestPath))
			require.True(t, resolution.HasReference(), "no reference for %q", resolution.RequestPath)
			assert.Equal(t, exeContents, contentsFromReference(t, img, *resolution.Reference), "unexpected contents for %q", resolution.RequestPath)
		}
		assert.ElementsMatch(t, []string{"/" + installedExe, "/" + cachedExe}, requestPaths.List())
	})

	t.Run("by MIME type", func(t *testing.T) {
		// only the regular header carries contents, so it is the only entry with a MIME type: a link header has no
		// data section to sniff. The path recorded for that entry is the installed name, which the upper layer has
		// replaced with a symlink, so the search still has to resolve it.
		installedEntry := catalogEntryForPath(t, img, "/"+installedExe, file.TypeRegular)
		require.NotEmpty(t, installedEntry.MIMEType)
		assert.Empty(t, catalogEntryForPath(t, img, "/"+cachedExe, file.TypeHardLink).MIMEType)

		resolutions, err := img.SquashedSearchContext.SearchByMIMEType(installedEntry.MIMEType)
		require.NoError(t, err)
		require.Len(t, resolutions, 1)
		require.True(t, resolutions[0].HasReference())

		assert.Equal(t, exeContents, contentsFromReference(t, img, *resolutions[0].Reference))
	})
}

// TestImage_ResolveLink_hardLinkTargetReplacedBySymlink covers the public link resolution API for a reference to the
// hardlink name, relative to both the image squash and the squash of the layer that replaced the file it names.
func TestImage_ResolveLink_hardLinkTargetReplacedBySymlink(t *testing.T) {
	img := readImageWithHardLinkReplacedBySymlink(t)
	require.Len(t, img.Layers, 2)

	cachedRef := catalogEntryForPath(t, img, "/"+cachedExe, file.TypeHardLink).Reference
	installedRef := catalogEntryForPath(t, img, "/"+installedExe, file.TypeRegular).Reference

	t.Run("by image squash", func(t *testing.T) {
		resolution, err := img.ResolveLinkByImageSquash(cachedRef)
		require.NoError(t, err)
		require.True(t, resolution.HasReference())
		assert.Equal(t, installedRef.ID(), resolution.ID())
	})

	t.Run("by layer squash", func(t *testing.T) {
		resolution, err := img.ResolveLinkByLayerSquash(cachedRef, 1)
		require.NoError(t, err)
		require.True(t, resolution.HasReference())
		assert.Equal(t, installedRef.ID(), resolution.ID())
	})
}

// TestImage_OpenPathFromSquash_hardLinkNamingAnotherHardLink covers a link header naming another link header rather
// than the regular one. Conventional tar writers do not emit this (every name after the first links back to the
// first), but binding to the intermediate link would name a tar entry that carries no contents.
func TestImage_OpenPathFromSquash_hardLinkNamingAnotherHardLink(t *testing.T) {
	img := readImageFromLayers(t, layerFromTarEntries(t,
		tarEntry{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
		tarEntry{path: sharedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
		tarEntry{path: cachedExe, typeFlag: tar.TypeLink, linkPath: sharedExe},
	))

	for _, p := range []string{installedExe, sharedExe, cachedExe} {
		assert.Equal(t, exeContents, contentsFromPath(t, img.OpenPathFromSquash, "/"+p), "unexpected contents for %q", p)
	}
}

// readImageWithHardLinkReplacedBySymlink builds the two layers of the uv scenario: the file and the hardlink naming
// it, then a layer replacing the installed name with a symlink aimed at the hardlink name.
func readImageWithHardLinkReplacedBySymlink(t *testing.T) *Image {
	t.Helper()

	return readImageFromLayers(t,
		layerFromTarEntries(t,
			tarEntry{path: installedExe, typeFlag: tar.TypeReg, contents: exeContents},
			tarEntry{path: cachedExe, typeFlag: tar.TypeLink, linkPath: installedExe},
		),
		layerFromTarEntries(t,
			tarEntry{path: installedExe, typeFlag: tar.TypeSymlink, linkPath: "/" + cachedExe},
		),
	)
}

// tarEntry is a single tar header and the data section that follows it, if any.
type tarEntry struct {
	path     string
	typeFlag byte
	linkPath string
	contents string
}

func layerFromTarEntries(t *testing.T, entries ...tarEntry) v1.Layer {
	t.Helper()

	tarPath := filepath.Join(t.TempDir(), "layer.tar")
	fh, err := os.Create(tarPath)
	require.NoError(t, err)

	tw := tar.NewWriter(fh)
	for _, entry := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     entry.path,
			Typeflag: entry.typeFlag,
			Linkname: entry.linkPath,
			Mode:     0o644,
			Size:     int64(len(entry.contents)),
		}))
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

// contentsFromPath fetches contents with any of the path-based content APIs (Image.OpenPathFromSquash,
// Layer.OpenPath, Layer.OpenPathFromSquash).
func contentsFromPath(t *testing.T, open func(file.Path) (io.ReadCloser, error), path string) string {
	t.Helper()

	reader, err := open(file.Path(path))
	require.NoError(t, err)

	return contentsFromReader(t, reader)
}

func contentsFromReference(t *testing.T, img *Image, ref file.Reference) string {
	t.Helper()

	reader, err := img.OpenReference(ref)
	require.NoError(t, err)

	return contentsFromReader(t, reader)
}

func contentsFromReader(t *testing.T, reader io.ReadCloser) string {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, reader.Close())
	})

	contents, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(contents)
}

// catalogEntryForPath returns the catalog entry recorded for the given real path and file type (built from that
// path's own tar header, with no link resolution). The type is needed because a path has an entry per layer that
// wrote it, and the layers here write the installed name as both a regular file and a symlink.
func catalogEntryForPath(t *testing.T, img *Image, path string, fileType file.Type) filetree.IndexEntry {
	t.Helper()

	entries, err := img.FileCatalog.GetByBasename(file.Path(path).Basename())
	require.NoError(t, err)

	for _, entry := range entries {
		if string(entry.RealPath) == path && entry.Metadata.Type == fileType {
			return entry
		}
	}

	t.Fatalf("no catalog entry for path=%q type=%q", path, fileType)
	return filetree.IndexEntry{}
}
