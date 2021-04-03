package integration

import (
	"fmt"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/scylladb/go-set"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/imagetest"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
)

var bi *image.Image
var by []byte

var simpleImageTestCases = []testCase{
	{
		name:           "FromTarball",
		source:         "docker-archive",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		tagCount:       1,
	},
	{
		name:           "FromDocker",
		source:         "docker",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		// name:hash
		// name:latest
		tagCount: 2,
	},
	{
		name:           "FromOciTarball",
		source:         "oci-archive",
		imageMediaType: v1Types.OCIManifestSchema1,
		layerMediaType: v1Types.OCILayer,
		tagCount:       0,
	},
	{
		name:           "FromOciDirectory",
		source:         "oci-dir",
		imageMediaType: v1Types.OCIManifestSchema1,
		layerMediaType: v1Types.OCILayer,
		tagCount:       0,
	},
}

type testCase struct {
	name           string
	source         string
	imageMediaType v1Types.MediaType
	layerMediaType v1Types.MediaType
	tagCount       int
}

func TestSimpleImage(t *testing.T) {
	expectedSet := set.NewIntSet()
	for _, src := range image.AllSources {
		expectedSet.Add(int(src))
	}
	expectedSet.Remove(int(image.OciRegistrySource))

	for _, c := range simpleImageTestCases {
		t.Run(c.name, func(t *testing.T) {
			i := imagetest.GetFixtureImage(t, c.source, "image-simple")

			assertImageSimpleMetadata(t, i, c)
			assertImageSimpleTrees(t, i)
			assertImageSimpleSquashedTrees(t, i)
			assertImageSimpleContents(t, i)
		})
	}

	if len(simpleImageTestCases) < expectedSet.Size() {
		t.Fatalf("probably missed a source during testing, double check that all image.sources are covered")
	}

}

func BenchmarkSimpleImage_GetImage(b *testing.B) {
	var err error
	for _, c := range simpleImageTestCases {
		if c.source == "docker" {
			// skip benchmark testing against the docker daemon
			continue
		}
		request := imagetest.PrepareFixtureImage(b, c.source, "image-simple")
		b.Cleanup(stereoscope.Cleanup)
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				bi, err = stereoscope.GetImage(request, nil)
				if err != nil {
					b.Fatal("could not get fixture image:", err)
				}
			}
		})
	}
}

func BenchmarkSimpleImage_FetchSquashedContents(b *testing.B) {
	for _, c := range simpleImageTestCases {
		if c.source == "docker" {
			// skip benchmark testing against the docker daemon
			continue
		}

		img := imagetest.GetFixtureImage(b, c.source, "image-simple")
		paths := img.SquashedTree().AllFiles()
		if len(paths) == 0 {
			b.Fatalf("expected paths but found none")
		}
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for _, ref := range paths {
					f, err := img.FileCatalog.Get(ref)
					if err != nil {
						b.Fatalf("unable to read: %+v", err)
					}
					by, err = ioutil.ReadAll(f.Contents())
				}
			}
		})
	}
}

func assertImageSimpleMetadata(t *testing.T, i *image.Image, expectedValues testCase) {
	t.Helper()
	t.Log("Asserting metadata...")
	if i.Metadata.Size != 65 {
		t.Errorf("unexpected image size: %d", i.Metadata.Size)
	}
	if i.Metadata.MediaType != expectedValues.imageMediaType {
		t.Errorf("unexpected image media type: %+v", i.Metadata.MediaType)
	}
	if len(i.Metadata.Tags) != expectedValues.tagCount {
		t.Errorf("unexpected number of tags: %d != %d : %+v", len(i.Metadata.Tags), expectedValues.tagCount, i.Metadata.Tags)
	} else if expectedValues.tagCount > 0 {
		if !strings.HasPrefix(i.Metadata.Tags[0].String(), fmt.Sprintf("%s-image-simple:", imagetest.ImagePrefix)) {
			t.Errorf("unexpected image tag: %+v", i.Metadata.Tags)
		}
	}

	expected := []image.LayerMetadata{
		{
			Index:     0,
			Size:      22,
			MediaType: expectedValues.layerMediaType,
		},
		{
			Index:     1,
			Size:      16,
			MediaType: expectedValues.layerMediaType,
		},
		{
			Index:     2,
			Size:      27,
			MediaType: expectedValues.layerMediaType,
		},
	}

	if len(expected) != len(i.Layers) {
		t.Fatal("unexpected number of layers:", len(i.Layers))
	}

	for idx, l := range i.Layers {
		if expected[idx].Size != l.Metadata.Size {
			t.Errorf("mismatched layer 'Size' (layer %d): %+v", idx, l.Metadata.Size)
		}
		if expected[idx].MediaType != l.Metadata.MediaType {
			t.Errorf("mismatched layer 'MediaType' (layer %d): %+v", idx, l.Metadata.MediaType)
		}
		if expected[idx].Index != l.Metadata.Index {
			t.Errorf("mismatched layer 'Index' (layer %d): %+v", idx, l.Metadata.Index)
		}
	}
}

func assertImageSimpleSquashedTrees(t *testing.T, i *image.Image) {
	t.Log("Asserting squashed trees...")
	one := filetree.NewFileTree()
	one.AddFile("/somefile-1.txt")

	two := filetree.NewFileTree()
	two.AddFile("/somefile-1.txt")
	two.AddFile("/somefile-2.txt")

	three := filetree.NewFileTree()
	three.AddFile("/somefile-1.txt")
	three.AddFile("/somefile-2.txt")
	three.AddFile("/really/.wh..wh..opq")
	three.AddFile("/really/nested/file-3.txt")

	expectedTrees := map[uint]*filetree.FileTree{
		0: one,
		1: two,
		2: three,
	}

	// there is a difference in behavior between docker 18 and 19 regarding opaque whiteout
	// creation during docker build (which could lead to test inconsistencies depending where
	// this test is run. However, this opaque whiteout is not important to theses tests, only
	// the correctness of the layer representation and squashing ability.
	ignorePaths := []file.Path{"/really/.wh..wh..opq"}

	compareLayerSquashTrees(t, expectedTrees, i, ignorePaths)

	squashed := filetree.NewFileTree()
	squashed.AddFile("/somefile-1.txt")
	squashed.AddFile("/somefile-2.txt")
	squashed.AddFile("/really/nested/file-3.txt")

	compareSquashTree(t, squashed, i)
}

func assertImageSimpleTrees(t *testing.T, i *image.Image) {
	t.Log("Asserting trees...")
	one := filetree.NewFileTree()
	one.AddFile("/somefile-1.txt")

	two := filetree.NewFileTree()
	two.AddFile("/somefile-2.txt")

	three := filetree.NewFileTree()
	three.AddFile("/really/.wh..wh..opq")
	three.AddFile("/really/nested/file-3.txt")

	expectedTrees := map[uint]*filetree.FileTree{
		0: one,
		1: two,
		2: three,
	}

	// there is a difference in behavior between docker 18 and 19 regarding opaque whiteout
	// creation during docker build (which could lead to test inconsistencies depending where
	// this test is run. However, this opaque whiteout is not important to theses tests, only
	// the correctness of the layer representation and squashing ability.
	ignorePaths := []file.Path{"/really/.wh..wh..opq"}

	compareLayerTrees(t, expectedTrees, i, ignorePaths)
}

func assertImageSimpleContents(t *testing.T, i *image.Image) {
	t.Log("Asserting contents...")

	expectedContents := map[string]string{
		"/somefile-1.txt":           "this file has contents",
		"/somefile-2.txt":           "file-2 contents!",
		"/really/nested/file-3.txt": "another file!\nwith lines...",
	}

	actualContents := make(map[string]io.Reader)
	for path := range expectedContents {
		reader, err := i.FileContentsFromSquash(file.Path(path))
		if err != nil {
			t.Fatal("unable to fetch multiple contents", err)
		}
		actualContents[path] = reader
	}

	if len(expectedContents) != len(actualContents) {
		t.Fatalf("mismatched number of contents: %d!=%d", len(expectedContents), len(actualContents))
	}

	for path, actual := range actualContents {
		expected, ok := expectedContents[path]
		if !ok {
			t.Errorf("extra path found: %+v", path)
		}
		b, err := ioutil.ReadAll(actual)
		if err != nil {
			t.Errorf("failed to read %+v : %+v", path, err)
		}
		if expected != string(b) {
			t.Errorf("mismatched contents (%s)", path)
		}
	}
}
