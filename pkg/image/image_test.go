package image

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/testutil"
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

// failingLayer is a valid layer right up until its content is needed, so Image.Read fails partway
// through the layer loop rather than in the up-front media type validation.
type failingLayer struct{ mockLayer }

func (failingLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, errors.New("layer content unavailable")
}

func (failingLayer) MediaType() (v1Types.MediaType, error) {
	return v1Types.DockerLayer, nil
}

func TestImage_CleanupReleasesLayerTars(t *testing.T) {
	img := readImageFromLayers(t, layerFromTarEntries(t, tarEntry{path: "a.txt", contents: "alpha"}))

	rc, err := img.OpenPathFromSquash("/a.txt")
	require.NoError(t, err)
	contents, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "alpha", string(contents))
	require.NoError(t, rc.Close())

	require.NoError(t, img.Cleanup())

	rc, err = img.OpenPathFromSquash("/a.txt")
	require.NoError(t, err)
	_, err = io.ReadAll(rc)
	require.ErrorIs(t, err, os.ErrClosed, "cleanup did not release the layer tar")

	// readImageFromLayers registers a second Cleanup via t.Cleanup, so this must stay idempotent
	require.NoError(t, img.Cleanup())
}

func TestImage_SecondReadReleasesTheFirstReadsTars(t *testing.T) {
	img := readImageFromLayers(t, layerFromTarEntries(t, tarEntry{path: "a.txt", contents: "alpha"}))

	// Read() looks idempotent and is called twice in the wild: pkg/image/sif/archive_provider_test.go
	// and test/integration/oci_registry_source_test.go both re-read an already-read image
	firstIndex := img.Layers[0].indexedContent
	require.NoError(t, img.Read())
	require.NotSame(t, firstIndex, img.Layers[0].indexedContent, "second Read builds a fresh index")

	// the second Read replaced the first one's layers, so it had to release them on the way past:
	// nothing else can still reach that index
	entries, err := firstIndex.EntriesByName("a.txt")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	_, err = io.ReadAll(entries[0].Reader)
	require.ErrorIs(t, err, os.ErrClosed, "the first Read's layer tar was orphaned")
}

func TestImage_PartialReadReleasesEarlierLayers(t *testing.T) {
	v1Img, err := mutate.AppendLayers(empty.Image,
		layerFromTarEntries(t, tarEntry{path: "a.txt", contents: "alpha"}),
		failingLayer{},
	)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("partial-read-test"), t.TempDir())
	t.Cleanup(func() { require.NoError(t, img.Cleanup()) })

	// layer one reads fine and opens its tar, layer two fails. Counting descriptors rather than
	// reaching for the index, because a failed Read must not leave the half-built layer set behind
	before := testutil.OpenDescriptorCount(t)
	require.Error(t, img.Read(), "expected the second layer to fail the read")
	require.Equal(t, before, testutil.OpenDescriptorCount(t), "a partial read leaked the first layer's tar")

	// and the image must stay safe to touch: accessors read the last layer, which on a partial read
	// is one that was never squashed
	require.Empty(t, img.Layers)
	require.NotPanics(t, func() {
		_, _, err := img.SquashedTree().File("/a.txt")
		require.NoError(t, err)
	})
}
