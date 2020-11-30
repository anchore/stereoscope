// +build integration

package integration

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

func TestImage_SquashedTree_OpaqueDirectoryExistsInFileCatalog(t *testing.T) {
	image, cleanup := imagetest.GetFixtureImage(t, "docker", "image-opaque-directory")
	defer cleanup()

	tree := image.SquashedTree()
	path := "/usr/lib/jvm"
	ref := tree.File(file.Path(path))

	_, err := image.FileCatalog.Get(*ref)
	if err != nil {
		t.Fatal(err)
	}
}
