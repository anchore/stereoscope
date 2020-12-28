package integration

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/imagetest"
	"testing"
)

func TestImage_SquashedTree_OpaqueDirectoryExistsInFileCatalog(t *testing.T) {
	image, cleanup := imagetest.GetFixtureImage(t, "docker-archive", "image-opaque-directory")
	defer cleanup()

	tree := image.SquashedTree()
	path := "/usr/lib/jvm"
	_, _, ref, err := tree.File(file.Path(path), true)
	if err != nil {
		t.Fatalf("unable to get file=%q : %+v", path, err)
	}

	_, err = image.FileCatalog.Get(*ref)
	if err != nil {
		t.Fatal(err)
	}
}
