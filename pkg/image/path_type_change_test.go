package image

import (
	"archive/tar"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

// the #268 scenario: /chimera goes dir with files -> empty dir -> removed -> file -> removed ->
// symlink -> removed -> dir again (optionally opaque), then a.txt comes back.
func chimeraLayers(t *testing.T, opaque bool) ([]v1.Layer, []map[string]file.Type) {
	reg, dir, sym := byte(tar.TypeReg), byte(tar.TypeDir), byte(tar.TypeSymlink)
	last := []tarEntry{{path: "chimera/", typeFlag: dir}}
	if opaque {
		last = append(last, tarEntry{path: "chimera/.wh..wh..opq", typeFlag: reg})
	}
	layers := []v1.Layer{
		layerFromTarEntries(t, tarEntry{path: "etc/", typeFlag: dir}, tarEntry{path: "etc/hostname", typeFlag: reg, contents: "h"}),
		layerFromTarEntries(t, tarEntry{path: "chimera/", typeFlag: dir}, tarEntry{path: "chimera/a.txt", typeFlag: reg, contents: "a"}, tarEntry{path: "chimera/b.txt", typeFlag: reg, contents: "b"}),
		layerFromTarEntries(t, tarEntry{path: "chimera/", typeFlag: dir}, tarEntry{path: "chimera/.wh.a.txt", typeFlag: reg}, tarEntry{path: "chimera/.wh.b.txt", typeFlag: reg}),
		layerFromTarEntries(t, tarEntry{path: ".wh.chimera", typeFlag: reg}),
		layerFromTarEntries(t, tarEntry{path: "chimera", typeFlag: reg, contents: "f"}),
		layerFromTarEntries(t, tarEntry{path: ".wh.chimera", typeFlag: reg}),
		layerFromTarEntries(t, tarEntry{path: "chimera", typeFlag: sym, linkPath: "/etc/hostname"}),
		layerFromTarEntries(t, tarEntry{path: ".wh.chimera", typeFlag: reg}),
		layerFromTarEntries(t, last...),
		layerFromTarEntries(t, tarEntry{path: "chimera/a.txt", typeFlag: reg, contents: "a2"}),
		// layer 10: back to a file on top of a populated dir
		layerFromTarEntries(t, tarEntry{path: "chimera", typeFlag: reg, contents: "f2"}),
	}
	expect := []map[string]file.Type{
		{},
		{"/chimera": file.TypeDirectory, "/chimera/a.txt": file.TypeRegular, "/chimera/b.txt": file.TypeRegular},
		{"/chimera": file.TypeDirectory},
		{},
		{"/chimera": file.TypeRegular},
		{},
		{"/chimera": file.TypeSymLink},
		{},
		{"/chimera": file.TypeDirectory},
		{"/chimera": file.TypeDirectory, "/chimera/a.txt": file.TypeRegular},
		{"/chimera": file.TypeRegular},
	}
	return layers, expect
}

// literal (no link following) snapshot of /chimera and everything under it. Taken twice: once from the node
// table and once by walking parent->children edges from the roots, since #268 corrupted only the edges.
func chimeraSnapshot(t *testing.T, tr filetree.Reader) map[string]file.Type {
	keep := func(p file.Path) bool { return p == "/chimera" || strings.HasPrefix(string(p), "/chimera/") }
	byTable := map[string]file.Type{}
	for _, p := range tr.AllRealPaths() {
		if !keep(p) {
			continue
		}
		n := tr.TreeReader().Node(filenode.IDByPath(p))
		require.NotNil(t, n, p)
		byTable[string(p)] = n.(*filenode.FileNode).FileType
	}
	byEdges := map[string]file.Type{}
	tr2 := tr.TreeReader()
	var visit func(n node.Node)
	visit = func(n node.Node) {
		fn := n.(*filenode.FileNode)
		if keep(fn.RealPath) {
			byEdges[string(fn.RealPath)] = fn.FileType
		}
		for _, c := range tr2.Children(n) {
			require.NotNil(t, c, "dangling child edge under %s", fn.RealPath)
			visit(c)
		}
	}
	for _, r := range tr2.Roots() {
		visit(r)
	}
	assert.Equal(t, byTable, byEdges, "node table and child edges disagree")
	return byEdges
}

func TestLayerSquash_PathTypeChangeAcrossLayers(t *testing.T) {
	tests := []struct {
		name   string
		opaque bool
	}{
		{name: "plain"},
		{name: "opaque", opaque: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers, expect := chimeraLayers(t, tt.opaque)
			img := readImageFromLayers(t, layers...)
			require.Len(t, img.Layers, len(expect))
			for idx, l := range img.Layers {
				got := chimeraSnapshot(t, l.SquashedTree)
				assert.Equal(t, expect[idx], got, "layer %d squash", idx)
			}
		})
	}
}
