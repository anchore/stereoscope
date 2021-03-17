package integration

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/imagetest"
	"testing"
)

func TestImage_SquashedTree_OpaqueDirectoryExistsInFileCatalog(t *testing.T) {
	image := imagetest.GetFixtureImage(t, "docker-archive", "image-opaque-directory")

	tree := image.SquashedTree()
	path := "/usr/lib/jvm"
	_, ref, err := tree.File(file.Path(path), filetree.FollowBasenameLinks)
	if err != nil {
		t.Fatalf("unable to get file=%q : %+v", path, err)
	}

	_, err = image.FileCatalog.Get(*ref)
	if err != nil {
		t.Fatal(err)
	}
}
