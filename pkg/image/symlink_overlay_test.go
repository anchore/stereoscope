package image

import (
	"archive/tar"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
)

// layer 0 has s -> /t, layer 1 writes s/x, layer 2 whites out s. The squash follows overlay (see FileTree.Merge): s/
// replaces the link and the whiteout drops it along with x, so nothing lands at /t/x. This holds however layer 1
// spells the directory, even though merged-rootfs runtimes only agree when the s/ header comes first.
func TestLayerSquash_WriteUnderLowerSymlinkFollowsOverlay(t *testing.T) {
	writes := []struct {
		name    string
		entries []tarEntry
	}{
		{
			name:    "implied dir",
			entries: []tarEntry{{path: "s/x", typeFlag: tar.TypeReg, contents: "x"}},
		},
		{
			name: "dir header first",
			entries: []tarEntry{
				{path: "s/", typeFlag: tar.TypeDir},
				{path: "s/x", typeFlag: tar.TypeReg, contents: "x"},
			},
		},
		{
			name: "dir header after entries",
			entries: []tarEntry{
				{path: "s/x", typeFlag: tar.TypeReg, contents: "x"},
				{path: "s/", typeFlag: tar.TypeDir},
			},
		},
	}
	modes := []struct {
		name string
		ctx  context.Context
	}{
		{name: "eager", ctx: context.Background()},
		{name: "lazy", ctx: WithLazyLayerSquashes(context.Background())},
	}

	for _, w := range writes {
		for _, m := range modes {
			t.Run(w.name+"/"+m.name, func(t *testing.T) {
				img, err := tryReadImageFromLayersWithContext(t, m.ctx,
					// the link target exists, so writing through it is a real option for a merged-rootfs applier
					layerFromTarEntries(t,
						tarEntry{path: "t/", typeFlag: tar.TypeDir},
						tarEntry{path: "s", typeFlag: tar.TypeSymlink, linkPath: "/t"},
					),
					layerFromTarEntries(t, w.entries...),
					layerFromTarEntries(t, tarEntry{path: ".wh.s", typeFlag: tar.TypeReg}),
					// a top layer, so layers 1 and 2 get lazy squashes in lazy mode
					layerFromTarEntries(t, tarEntry{path: "unrelated", typeFlag: tar.TypeReg, contents: "u"}),
				)
				require.NoError(t, err)

				assert.True(t, img.Layers[1].Tree.HasPath("/s/x"), "layer 1 tree: /s/x")

				// in the layer 1 squash the directory replaced the link, so nothing landed at the link's target.
				// Look nodes up by literal path, since HasPath would follow the link.
				sq := img.Layers[1].SquashedTree.TreeReader()
				n := sq.Node(filenode.IDByPath("/s"))
				require.NotNil(t, n, "layer 1 squash: /s")
				assert.Equal(t, file.TypeDirectory, n.(*filenode.FileNode).FileType, "layer 1 squash: /s type")
				assert.True(t, hasRealPath(img.Layers[1].SquashedTree, "/s/x"), "layer 1 squash: /s/x")
				assert.False(t, hasRealPath(img.Layers[1].SquashedTree, "/t/x"), "layer 1 squash: /t/x")

				// the whiteout drops the directory along with x, and the link target is left empty
				for _, idx := range []int{2, 3} {
					tr := img.Layers[idx].SquashedTree
					for _, p := range []file.Path{"/s", "/s/x", "/t/x"} {
						assert.Falsef(t, hasRealPath(tr, p), "layer %d squash: %s", idx, p)
					}
					assert.Truef(t, hasRealPath(tr, "/t"), "layer %d squash: /t", idx)
				}
			})
		}
	}
}

func hasRealPath(tr filetree.Reader, p file.Path) bool {
	return tr.TreeReader().Node(filenode.IDByPath(p)) != nil
}
