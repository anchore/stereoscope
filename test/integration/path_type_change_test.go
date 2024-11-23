package integration

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

func TestPathTypeChangeImage(t *testing.T) {
	image := imagetest.GetFixtureImage(t, "docker-archive", "image-path-type-change")

	layerAssertions := []map[string]file.Type{
		make(map[string]file.Type),
		{
			"/chimera/a.txt": file.TypeRegular,
			"/chimera/b.txt": file.TypeRegular,
			"/chimera":       file.TypeDirectory,
		},
		{
			"/chimera": file.TypeDirectory,
		},
		make(map[string]file.Type),
		{
			"/chimera": file.TypeRegular,
		},
		make(map[string]file.Type),
		{
			"/chimera": file.TypeSymLink,
		},
		make(map[string]file.Type),
		{
			"/chimera": file.TypeDirectory,
		},
	}

	for idx, layer := range image.Layers {
		assertions := layerAssertions[idx]
		err := layer.SquashedTree.Walk(func(path file.Path, f filenode.FileNode) error {
			expect, ok := assertions[string(path)]
			if !ok {
				return nil
			}
			if f.FileType != expect {
				t.Errorf("at %v got %v want %v", path, f.FileType, expect)
			}
			delete(assertions, string(path))
			return nil
		}, nil)
		if err != nil {
			t.Error(err)
		}
	}
	for i, a := range layerAssertions {
		if len(a) > 0 {
			for k, v := range a {
				t.Errorf("for layer %v, never saw %v of type %v", i, k, v)
			}
		}
	}
}
