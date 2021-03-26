package integration

import (
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/imagetest"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestImageXattr(t *testing.T) {
	expected := map[string]string{
		"user.comment":        "very cool",
		"com.anchore.version": "3.0",
	}
	cases := []struct {
		name   string
		source string
	}{
		{
			name:   "FromTarball",
			source: "docker-archive",
		},
		{
			name:   "FromDocker",
			source: "docker",
		},
		{
			name:   "FromOciTarball",
			source: "oci-archive",
		},
		{
			name:   "FromOciDirectory",
			source: "oci-dir",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := imagetest.GetFixtureImage(t, c.source, "image-xattr")

			_, ref, err := i.SquashedTree().File("/file-1.txt")
			if err != nil {
				t.Fatalf("could not get file: %+v", err)
			}

			entry, err := i.FileCatalog.Get(*ref)
			if err != nil {
				t.Fatalf("could not get entry: %+v", err)
			}

			assert.Equal(t, expected, entry.Metadata.PAXRecords)
		})
	}

	if len(cases) < len(image.AllSources) {
		t.Fatalf("probably missed a source during testing, double check that all image.sources are covered")
	}

}
