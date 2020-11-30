package docker

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/go-test/deep"
)

func TestNewManifest(t *testing.T) {
	tests := []struct {
		fixture           string
		expectedToBeValid bool
	}{
		{
			fixture:           "test-fixtures/valid-multi-manifest-with-tags.json",
			expectedToBeValid: true,
		},
		{
			fixture:           "test-fixtures/empty-file",
			expectedToBeValid: false,
		},
		{
			fixture:           "test-fixtures/no-descriptors.json",
			expectedToBeValid: false,
		},
		{
			// unexpected, but we are not expecting to validate this case further
			fixture:           "test-fixtures/single-blank-manifest.json",
			expectedToBeValid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			fh, err := os.Open(test.fixture)
			if err != nil {
				t.Fatalf("could not open fixture: %+v", err)
			}

			contents, err := ioutil.ReadAll(fh)
			if err != nil {
				t.Fatalf("could not read fixture: %+v", err)
			}

			m, err := newManifest(contents)
			if err != nil && test.expectedToBeValid {
				t.Fatalf("expected a valid manifest, but got error: %+v", err)
			} else if err == nil && !test.expectedToBeValid {
				t.Fatalf("expected to be an invalid manifest but got no error")
			}

			if !test.expectedToBeValid {
				return
			}

			if !bytes.Equal(m.raw, contents) {
				t.Error("expected raw contents to not be altered but were")
			}

			if m.parsed == nil {
				t.Error("failed to parse contents meaningfully and return an error")
			}

		})
	}
}

func TestManifestTags(t *testing.T) {
	tests := []struct {
		fixture string
		tags    []string
	}{
		{
			fixture: "test-fixtures/valid-multi-manifest-with-tags.json",
			tags: []string{
				"anchore/anchore-engine:latest",
				"anchore/anchore-engine:v0.8.2",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			fh, err := os.Open(test.fixture)
			if err != nil {
				t.Fatalf("could not open fixture: %+v", err)
			}

			contents, err := ioutil.ReadAll(fh)
			if err != nil {
				t.Fatalf("could not read fixture: %+v", err)
			}

			m, err := newManifest(contents)
			if err != nil {
				t.Fatalf("invalid manifest: %+v", err)
			}

			for _, d := range deep.Equal(m.tags(), test.tags) {
				t.Errorf("diff: %s", d)
			}
		})
	}
}
