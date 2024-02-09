//go:build !windows
// +build !windows

package integration

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/scylladb/go-set"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/sif"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

// Common layer metadata for OCI / Docker / Podman. MediaType will be filled in during test.
var simpleImageLayers = []image.LayerMetadata{
	{
		Index: 0,
		Size:  22,
	},
	{
		Index: 1,
		Size:  16,
	},
	{
		Index: 2,
		Size:  27,
	},
}

// Singularity images are squashed to a single layer.
var simpleImageSingularityLayer = []image.LayerMetadata{
	{
		Index: 0,
		// Disable size check. Size can vary - image build embeds variable length timestamps etc.
		Size: -1,
	},
}

var simpleImageTestCases = []testCase{
	{
		source:         "docker-archive",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		layers:         simpleImageLayers,
		tagCount:       1,
		size:           65,
	},
	{
		source:         "docker",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		layers:         simpleImageLayers,
		// name:hash
		// name:latest
		tagCount: 2,
		size:     65,
	},
	{
		source:         "podman",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		layers:         simpleImageLayers,
		tagCount:       2,
		size:           65,
	},
	{
		source:         "containerd",
		imageMediaType: v1Types.DockerManifestSchema2,
		layerMediaType: v1Types.DockerLayer,
		layers:         simpleImageLayers,
		tagCount:       2,
		size:           65,
	},
	{
		source:         "oci-archive",
		imageMediaType: v1Types.OCIManifestSchema1,
		layerMediaType: v1Types.OCILayer,
		layers:         simpleImageLayers,
		tagCount:       0,
		size:           65,
	},
	{
		source:         "oci-dir",
		imageMediaType: v1Types.OCIManifestSchema1,
		layerMediaType: v1Types.OCILayer,
		layers:         simpleImageLayers,
		tagCount:       0,
		size:           65,
	},
	{
		source:         "singularity",
		imageMediaType: sif.SingularityMediaType,
		layerMediaType: image.SingularitySquashFSLayer,
		layers:         simpleImageSingularityLayer,
		tagCount:       0,
		// Disable size check. Size can vary - image build embeds timestamps etc.
		size: -1,
	},
}

type testCase struct {
	source         string
	imageMediaType v1Types.MediaType
	layerMediaType v1Types.MediaType
	layers         []image.LayerMetadata
	tagCount       int
	size           int
}

func TestSimpleImage(t *testing.T) {
	expectedSet := set.NewIntSet()
	for _, src := range image.AllSources {
		expectedSet.Add(int(src))
	}
	expectedSet.Remove(int(image.OciRegistrySource))

	for _, c := range simpleImageTestCases {
		t.Run(c.source, func(t *testing.T) {
			if runtime.GOOS != "linux" {
				switch c.source {
				case "containerd":
					t.Skip("containerd is only supported on linux")
				case "podman":
					t.Skip("podman is only supported on linux")
				}
			}

			i := imagetest.GetFixtureImage(t, c.source, "image-simple")

			assertImageSimpleMetadata(t, i, c)
			// Singularity images are a single layer. Don't verify content per layer.
			if c.source != "singularity" {
				assertImageSimpleTrees(t, i)
				assertImageSimpleSquashedTrees(t, i)
			}
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
		filter := func(path string) bool { return true }

		b.Run(c.source, func(b *testing.B) {
			var bi *image.Image
			for i := 0; i < b.N; i++ {

				bi, err = stereoscope.GetImage(context.TODO(), request, filter)
				b.Cleanup(func() {
					require.NoError(b, bi.Cleanup())
				})

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
		b.Run(c.source, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for _, ref := range paths {
					f, err := img.FileCatalog.Open(ref)
					if err != nil {
						b.Fatalf("unable to read: %+v", err)
					}
					_, err = io.ReadAll(f)
				}
			}
		})
	}
}

func assertImageSimpleMetadata(t *testing.T, i *image.Image, expectedValues testCase) {
	t.Helper()
	t.Log("Asserting metadata...")

	if i.Metadata.MediaType != expectedValues.imageMediaType {
		t.Errorf("unexpected image media type: %+v", i.Metadata.MediaType)
	}
	if len(i.Metadata.Tags) != expectedValues.tagCount {
		t.Errorf("unexpected number of tags: %d != %d : %+v", len(i.Metadata.Tags), expectedValues.tagCount, i.Metadata.Tags)
	} else if expectedValues.tagCount > 0 {
		if !strings.Contains(i.Metadata.Tags[0].String(), fmt.Sprintf("%s-image-simple:", imagetest.ImagePrefix)) {
			t.Errorf("unexpected image tag: %+v", i.Metadata.Tags)
		}
	}

	if expectedValues.size >= 0 && i.Metadata.Size != int64(expectedValues.size) {
		t.Errorf("unexpected image size: %d", i.Metadata.Size)
	}

	if len(expectedValues.layers) != len(i.Layers) {
		t.Fatal("unexpected number of layers:", len(i.Layers))
	}

	for idx, l := range i.Layers {
		expected := expectedValues.layers[idx]
		expected.MediaType = expectedValues.layerMediaType
		if expected.Size >= 0 && expected.Size != l.Metadata.Size {
			t.Errorf("mismatched layer 'Size' (layer %d): %+v", idx, l.Metadata.Size)
		}
		if expected.MediaType != l.Metadata.MediaType {
			t.Errorf("mismatched layer 'MediaType' (layer %d): %+v", idx, l.Metadata.MediaType)
		}
		if expected.Index != l.Metadata.Index {
			t.Errorf("mismatched layer 'Index' (layer %d): %+v", idx, l.Metadata.Index)
		}
	}
}

func assertImageSimpleSquashedTrees(t *testing.T, i *image.Image) {
	t.Helper()
	t.Log("Asserting squashed trees...")

	one := filetree.New()
	one.AddFile("/somefile-1.txt")

	two := filetree.New()
	two.AddFile("/somefile-1.txt")
	two.AddFile("/somefile-2.txt")

	three := filetree.New()
	three.AddFile("/somefile-1.txt")
	three.AddFile("/somefile-2.txt")
	three.AddFile("/really/.wh..wh..opq")
	three.AddFile("/really/nested/file-3.txt")

	expectedTrees := map[uint]filetree.Reader{
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

	squashed := filetree.New()
	squashed.AddFile("/somefile-1.txt")
	squashed.AddFile("/somefile-2.txt")
	squashed.AddFile("/really/nested/file-3.txt")

	compareSquashTree(t, squashed, i)
}

func assertImageSimpleTrees(t *testing.T, i *image.Image) {
	t.Helper()
	t.Log("Asserting trees...")

	one := filetree.New()
	one.AddFile("/somefile-1.txt")

	two := filetree.New()
	two.AddFile("/somefile-2.txt")

	three := filetree.New()
	three.AddFile("/really/.wh..wh..opq")
	three.AddFile("/really/nested/file-3.txt")

	expectedTrees := map[uint]filetree.Reader{
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
	t.Helper()
	t.Log("Asserting contents...")

	expectedContents := map[string]string{
		"/somefile-1.txt":           "this file has contents",
		"/somefile-2.txt":           "file-2 contents!",
		"/really/nested/file-3.txt": "another file!\nwith lines...",
	}

	actualContents := make(map[string]io.Reader)
	for path := range expectedContents {
		reader, err := i.OpenPathFromSquash(file.Path(path))
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
		b, err := io.ReadAll(actual)
		if err != nil {
			t.Errorf("failed to read %+v : %+v", path, err)
		}
		if expected != string(b) {
			t.Errorf("mismatched contents (%s)", path)
		}
	}
}
