package image

import (
	"crypto/sha256"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
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

			img := NewImage(nil, tempFile.Name(), test.options...)

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
