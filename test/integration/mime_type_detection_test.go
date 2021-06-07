package integration

import (
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/imagetest"
	"github.com/scylladb/go-set/strset"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestContentMIMETypeDetection(t *testing.T) {
	request := imagetest.PrepareFixtureImage(t, "docker-archive", "image-simple")
	img, err := stereoscope.GetImage(request, nil)
	assert.NoError(t, err)
	t.Cleanup(stereoscope.Cleanup)

	pathsByMIMEType := map[string]*strset.Set{
		"text/plain": strset.New("/somefile-1.txt", "/somefile-2.txt", "/really/nested/file-3.txt"),
	}

	for mimeType, paths := range pathsByMIMEType {
		refs, err := img.FilesByMIMETypeFromSquash(mimeType)
		assert.NoError(t, err)
		assert.NotZero(t, len(refs), "found no refs for type=%q", mimeType)
		for _, ref := range refs {
			if !paths.Has(string(ref.RealPath)) {
				t.Errorf("unable to find %q", ref.RealPath)
			}
		}
	}

}
