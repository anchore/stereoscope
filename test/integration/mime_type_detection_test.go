package integration

import (
	"context"
	"testing"

	"github.com/scylladb/go-set/strset"
	"github.com/stretchr/testify/assert"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

func TestContentMIMETypeDetection(t *testing.T) {
	request := imagetest.PrepareFixtureImage(t, "docker-archive", "image-simple")

	img, err := stereoscope.GetImage(context.TODO(), request, "")

	assert.NoError(t, err)
	t.Cleanup(stereoscope.Cleanup)

	pathsByMIMEType := map[string]*strset.Set{
		"text/plain": strset.New("/somefile-1.txt", "/somefile-2.txt", "/really", "/really/nested", "/really/nested/file-3.txt"),
	}

	for mimeType, paths := range pathsByMIMEType {
		refs, err := img.SquashedSearchContext.SearchByMIMEType(mimeType)
		assert.NoError(t, err)
		assert.NotZero(t, len(refs), "found no refs for type=%q", mimeType)
		for _, ref := range refs {
			if !paths.Has(string(ref.RealPath)) {
				t.Errorf("unable to find %q", ref.RealPath)
			}
		}
	}

}
