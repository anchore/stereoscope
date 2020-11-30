package image

import (
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
					RawManifest: []byte("some bytes"),
				},
			},
		},
		{
			name: "with digest",
			options: []AdditionalMetadata{
				WithManifestDigest("the-digest"),
			},
			image: Image{
				Metadata: Metadata{
					ManifestDigest: "the-digest",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			img, err := NewImage(nil, test.options...)
			if err != nil {
				t.Fatalf("could not create image: %+v", err)
			}
			for _, d := range deep.Equal(img, &test.image) {
				t.Errorf("diff: %+v", d)
			}
		})
	}
}
