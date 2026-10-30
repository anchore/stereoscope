package image

import (
	"archive/tar"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/tree"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

// layer trees and squashes share FileNodes (see tree.Tree). This builds an image that exercises every squash
// path, then works over copies of all of its trees with every mutating operation, and checks that no node any
// original tree holds was changed and that no original tree's structure moved.
func TestLayerSquash_SharedNodesAreNeverMutated(t *testing.T) {
	reg, dir, sym, hard := byte(tar.TypeReg), byte(tar.TypeDir), byte(tar.TypeSymlink), byte(tar.TypeLink)
	layers, _ := chimeraLayers(t, true)
	layers = append(layers,
		layerFromTarEntries(t,
			// the file comes first, so the dir entries hit existing nodes with no reference
			tarEntry{path: "deep/x/file", typeFlag: reg, contents: "1"},
			tarEntry{path: "deep/x/", typeFlag: dir},
			tarEntry{path: "deep/hard", typeFlag: hard, linkPath: "deep/x/file"},
			tarEntry{path: "deep/link", typeFlag: sym, linkPath: "/deep/x/file"},
			tarEntry{path: "deeplink", typeFlag: sym, linkPath: "/deep/x"},
			tarEntry{path: "deep/gone", typeFlag: reg, contents: "g"},
		),
		layerFromTarEntries(t,
			tarEntry{path: "deep/x/file", typeFlag: reg, contents: "2"}, // replaces the symlink's target
			tarEntry{path: "deep/.wh.gone", typeFlag: reg},
			tarEntry{path: "deep/x/.wh..wh..opq", typeFlag: reg},
			tarEntry{path: "deep/x/new", typeFlag: reg, contents: "n"},
		),
	)
	img := readImageFromLayers(t, layers...)

	var originals []filetree.Reader
	for _, l := range img.Layers {
		originals = append(originals, l.Tree, l.SquashedTree)
	}
	originals = append(originals, img.SquashedTree())

	// record every node by pointer with a value snapshot, and every tree's structure
	nodes := map[*filenode.FileNode]filenode.FileNode{}
	owners := map[*filenode.FileNode]int{}
	var structures []map[string]string
	for _, tr := range originals {
		for _, n := range tr.TreeReader().Nodes() {
			fn := n.(*filenode.FileNode)
			nodes[fn] = *fn
			owners[fn]++
		}
		structures = append(structures, treeStructure(tr.TreeReader()))
	}
	shared := 0
	for _, count := range owners {
		if count > 1 {
			shared++
		}
	}
	require.NotZero(t, shared, "expected trees to share nodes, otherwise this test proves nothing")

	for _, tr := range originals {
		cp, err := tr.Copy()
		require.NoError(t, err)
		mutateCopy(t, cp, img)

		cpOfCp, err := cp.Copy()
		require.NoError(t, err)
		mutateCopy(t, cpOfCp, img)
	}

	for fn, before := range nodes {
		assert.Equal(t, before, *fn, "node %s was modified in place", before.RealPath)
	}
	for idx, tr := range originals {
		assert.Equal(t, structures[idx], treeStructure(tr.TreeReader()), "tree %d structure changed", idx)
	}
}

// mutateCopy runs every mutating FileTree operation over the given copy.
func mutateCopy(t *testing.T, cp filetree.ReadWriter, img *Image) {
	t.Helper()
	// re-add every path as itself, which fills in the reference on any node that has none
	for _, n := range cp.TreeReader().Nodes() {
		fn := n.(*filenode.FileNode)
		var err error
		switch fn.FileType {
		case file.TypeDirectory:
			_, err = cp.AddDir(fn.RealPath)
		case file.TypeSymLink:
			_, err = cp.AddSymLink(fn.RealPath, fn.LinkPath)
		case file.TypeHardLink:
			_, err = cp.AddHardLink(fn.RealPath, fn.LinkPath)
		default:
			_, err = cp.AddFile(fn.RealPath)
		}
		require.NoError(t, err)
	}
	for _, l := range img.Layers {
		require.NoError(t, cp.Merge(l.Tree))
	}
	require.NoError(t, cp.RemovePath("/deeplink/file")) // through a link
	require.NoError(t, cp.RemovePath("/chimera"))
	ft := cp.(*filetree.FileTree)
	require.NoError(t, ft.RemoveChildPaths("/deep"))
	require.NoError(t, ft.RemoveChildPaths("/"))
}

// treeStructure is each node's fields plus its children, gathered by walking parent to child edges from the
// roots, and checked against the node table (a broken edge would otherwise go unnoticed).
func treeStructure(tr tree.Reader) map[string]string {
	out := map[string]string{}
	var visit func(n node.Node)
	visit = func(n node.Node) {
		var kids []string
		for _, c := range tr.Children(n) {
			kids = append(kids, string(c.ID()))
			visit(c)
		}
		sort.Strings(kids)
		fn := n.(*filenode.FileNode)
		out[string(n.ID())] = fmt.Sprintf("%+v kids=%s", *fn, strings.Join(kids, ","))
	}
	for _, r := range tr.Roots() {
		visit(r)
	}
	for _, n := range tr.Nodes() {
		if _, ok := out[string(n.ID())]; !ok {
			out[string(n.ID())] = "unreachable from the roots"
		}
	}
	return out
}
