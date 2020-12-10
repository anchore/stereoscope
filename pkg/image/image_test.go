package image

import (
	"crypto/sha256"
	"fmt"
	"github.com/go-test/deep"
	"github.com/google/go-containerregistry/pkg/name"
	"testing"
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			img := NewImage(nil, test.options...)

			err = img.applyOverrideMetadata()
			if err != nil {
				t.Fatalf("could not create image: %+v", err)
			}
			for _, d := range deep.Equal(img, &test.image) {
				t.Errorf("diff: %+v", d)
			}
		})
	}
}
