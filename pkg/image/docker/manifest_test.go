package docker

import (
	"bytes"
	"encoding/json"
	"flag"
	"github.com/sergi/go-diff/diffmatchpatch"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anchore/go-testutils"
	"github.com/go-test/deep"
)

var update = flag.Bool("update", false, "update the *.golden files for the oci manifest assembly test")

func TestNewManifest(t *testing.T) {
	tests := []struct {
		fixture           string
		expectedToBeValid bool
		expectedConfig    string
	}{
		{
			fixture:           "test-fixtures/valid-multi-manifest-with-tags.json",
			expectedToBeValid: true,
			expectedConfig:    "881a352c4517dbf5e561a08dd1c7cf65f6c4349d3ab9b13e95210800e12b14a8.json",
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

			if m.parsed == nil {
				t.Error("failed to parse contents meaningfully and return an error")
			}

			if len(m.parsed) == 0 {
				if test.expectedConfig == "" {
					return
				} else {
					t.Fatalf("did not parse the config, but expected a value")
				}
			}

			if m.parsed[0].Config != test.expectedConfig {
				t.Errorf("unpexpected config value (parsing probably failed)")
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

			for _, d := range deep.Equal(m.allTags(), test.tags) {
				t.Errorf("diff: %s", d)
			}
		})
	}
}

func TestAssembleOCIManifest(t *testing.T) {
	// note: this is a
	fh, err := os.Open("test-fixtures/engine-config.json")
	if err != nil {
		t.Fatalf("could not open config: %+v", err)
	}

	configBytes, err := ioutil.ReadAll(fh)
	if err != nil {
		t.Fatalf("could not read config: %+v", err)
	}

	sizes := []int64{
		210882560,
		20480,
		64316928,
		38535168,
		1536,
		56832,
		232243200,
	}

	manifest, err := assembleOCIManifest(configBytes, sizes)
	if err != nil {
		t.Fatalf("could not assemble manifest: %+v", err)
	}

	actualBytes, err := json.Marshal(&manifest)
	if err != nil {
		t.Fatalf("could not serialize manifest: %+v", err)
	}

	if *update {
		testutils.UpdateGoldenFileContents(t, actualBytes)
	}

	var expectedBytes = testutils.GetGoldenFileContents(t)

	if !bytes.Equal(expectedBytes, actualBytes) {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(string(expectedBytes), string(actualBytes), true)
		t.Errorf("mismatched output:\n%s", dmp.DiffPrettyText(diffs))
	}

}
