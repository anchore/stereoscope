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

// a layer can carry two entries for one path. Extraction applies them in order and, unless both are
// directories, removes whatever the earlier entry left before writing the later one (containerd and docker
// pkg/archive), so the later entry is what the layer holds at that path, whatever the earlier one's type.
// An earlier entry with children is kept, so a type change over it still fails the read.
func TestLayerIndex_PathTypeChangeWithinLayer(t *testing.T) {
	reg, dir, sym, lnk := byte(tar.TypeReg), byte(tar.TypeDir), byte(tar.TypeSymlink), byte(tar.TypeLink)
	tests := []struct {
		name     string
		entries  []tarEntry
		path     string
		wantType file.Type
		// wantContents is checked when the path should end up a regular file
		wantContents string
		wantErr      bool
	}{
		{
			name: "dangling hardlink, then a regular file",
			entries: []tarEntry{
				{path: "a.txt", typeFlag: lnk, linkPath: "missing.txt"},
				{path: "a.txt", typeFlag: reg, contents: "REGULAR"},
			},
			path: "/a.txt", wantType: file.TypeRegular, wantContents: "REGULAR",
		},
		{
			name: "hardlink to a file of the layer, then a regular file",
			entries: []tarEntry{
				{path: "f.txt", typeFlag: reg, contents: "F"},
				{path: "a.txt", typeFlag: lnk, linkPath: "f.txt"},
				{path: "a.txt", typeFlag: reg, contents: "REGULAR"},
			},
			path: "/a.txt", wantType: file.TypeRegular, wantContents: "REGULAR",
		},
		{
			name: "dangling hardlink, then a hardlink to a file of the layer",
			entries: []tarEntry{
				{path: "f.txt", typeFlag: reg, contents: "F"},
				{path: "a.txt", typeFlag: lnk, linkPath: "missing.txt"},
				{path: "a.txt", typeFlag: lnk, linkPath: "f.txt"},
			},
			path: "/a.txt", wantType: file.TypeRegular, wantContents: "F",
		},
		{
			name: "symlink, then a regular file",
			entries: []tarEntry{
				{path: "f.txt", typeFlag: reg, contents: "F"},
				{path: "a.txt", typeFlag: sym, linkPath: "f.txt"},
				{path: "a.txt", typeFlag: reg, contents: "REGULAR"},
			},
			path: "/a.txt", wantType: file.TypeRegular, wantContents: "REGULAR",
		},
		{
			name: "regular file, then a symlink",
			entries: []tarEntry{
				{path: "a.txt", typeFlag: reg, contents: "REGULAR"},
				{path: "a.txt", typeFlag: sym, linkPath: "f.txt"},
			},
			path: "/a.txt", wantType: file.TypeSymLink,
		},
		{
			name: "regular file, then a directory",
			entries: []tarEntry{
				{path: "d", typeFlag: reg, contents: "REGULAR"},
				{path: "d/", typeFlag: dir},
				{path: "d/c.txt", typeFlag: reg, contents: "C"},
			},
			path: "/d", wantType: file.TypeDirectory,
		},
		{
			name: "symlink, then a symlink elsewhere",
			entries: []tarEntry{
				{path: "a.txt", typeFlag: reg, contents: "A"},
				{path: "b.txt", typeFlag: reg, contents: "B"},
				{path: "s", typeFlag: sym, linkPath: "/a.txt"},
				{path: "s", typeFlag: sym, linkPath: "/b.txt"},
			},
			path: "/s", wantType: file.TypeSymLink, wantContents: "B",
		},
		{
			name: "regular file, then a regular file",
			entries: []tarEntry{
				{path: "a.txt", typeFlag: reg, contents: "FIRST"},
				{path: "a.txt", typeFlag: reg, contents: "SECOND"},
			},
			path: "/a.txt", wantType: file.TypeRegular, wantContents: "SECOND",
		},
		{
			name: "empty directory, then a regular file",
			entries: []tarEntry{
				{path: "d/", typeFlag: dir},
				{path: "d", typeFlag: reg, contents: "REGULAR"},
			},
			path: "/d", wantType: file.TypeRegular, wantContents: "REGULAR",
		},
		{
			name: "directory with a child, then a directory",
			entries: []tarEntry{
				{path: "d/", typeFlag: dir},
				{path: "d/c.txt", typeFlag: reg, contents: "C"},
				{path: "d/", typeFlag: dir},
			},
			path: "/d/c.txt", wantType: file.TypeRegular, wantContents: "C",
		},
		{
			name: "directory with a child, then a regular file",
			entries: []tarEntry{
				{path: "d/", typeFlag: dir},
				{path: "d/c.txt", typeFlag: reg, contents: "C"},
				{path: "d", typeFlag: reg, contents: "REGULAR"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := tryReadImageFromLayers(t, layerFromTarEntries(t, tt.entries...))
			if tt.wantErr {
				require.Error(t, err, "an entry with children is not replaced, so the retype fails the read")
				return
			}
			require.NoError(t, err, "a later entry for a path replaces the earlier one rather than failing the image")

			entry := catalogEntryForTest(t, img, tt.path)
			assert.Equal(t, tt.wantType, entry.Metadata.Type)
			assertNoOrphanedCatalogEntries(t, img)
			if tt.wantContents != "" {
				contents, err := hardlinkContentsForTest(img, tt.path)
				require.NoError(t, err)
				assert.Equal(t, tt.wantContents, contents)
			}
		})
	}
}

// a hardlink keeps the inode it was made against: replacing the other name later in the same layer gives that
// name a new file, so the link still reads the old contents after the old reference leaves the catalog
func TestLayerIndex_ReplacedHardLinkTargetWithinLayer(t *testing.T) {
	img, err := tryReadImageFromLayers(t, layerFromTarEntries(t,
		tarEntry{path: "a", typeFlag: tar.TypeReg, contents: "OLD"},
		tarEntry{path: "h", typeFlag: tar.TypeLink, linkPath: "a"},
		tarEntry{path: "a", typeFlag: tar.TypeReg, contents: "NEW"},
	))
	require.NoError(t, err)

	for p, want := range map[string]string{"/h": "OLD", "/a": "NEW"} {
		contents, err := hardlinkContentsForTest(img, p)
		require.NoError(t, err)
		assert.Equal(t, want, contents, p)
	}
	assertNoOrphanedCatalogEntries(t, img)
}

// a layer that writes under a lower symlink and then retypes that path: a rootfs applier wrote REAL through the
// symlink to /usr/lib/x, then replaced only the symlink. The earlier node has children standing for that write,
// so it is not replaced: a retype fails the read, and a directory merges, keeping /lib/x.
func TestLayerIndex_RetypeOverWriteThroughLowerSymlink(t *testing.T) {
	reg, dir, sym := byte(tar.TypeReg), byte(tar.TypeDir), byte(tar.TypeSymlink)
	lower := func() v1.Layer {
		return layerFromTarEntries(t,
			tarEntry{path: "usr/lib/x", typeFlag: reg, contents: "DECOY"},
			tarEntry{path: "lib", typeFlag: sym, linkPath: "/usr/lib"},
		)
	}
	tests := []struct {
		name    string
		upper   []tarEntry
		wantErr bool
	}{
		{
			name:    "lower symlink, then a regular file",
			upper:   []tarEntry{{path: "lib/x", typeFlag: reg, contents: "REAL"}, {path: "lib", typeFlag: reg, contents: "FILE"}},
			wantErr: true,
		},
		{
			name: "own symlink, then a regular file",
			upper: []tarEntry{
				{path: "s", typeFlag: sym, linkPath: "/usr/lib"},
				{path: "s/x", typeFlag: reg, contents: "REAL"},
				{path: "s", typeFlag: reg, contents: "FILE"},
			},
			wantErr: true,
		},
		{
			name:  "lower symlink, then a directory",
			upper: []tarEntry{{path: "lib/x", typeFlag: reg, contents: "REAL"}, {path: "lib/", typeFlag: dir}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := tryReadImageFromLayers(t, lower(), layerFromTarEntries(t, tt.upper...))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			exists, _, err := img.Layers[1].Tree.File("/lib/x")
			require.NoError(t, err)
			assert.True(t, exists, "the write through the lower symlink stays in the layer tree")
			assertNoOrphanedCatalogEntries(t, img)
		})
	}
}

// assertNoOrphanedCatalogEntries checks that the catalog and the trees agree both ways: every catalog entry is
// reached by some layer tree (a reference an image read replaced must leave the catalog too, or index searches
// return names no tree has), and every reference a layer tree or squash holds has a catalog entry
func assertNoOrphanedCatalogEntries(t *testing.T, img *Image) {
	t.Helper()
	reached := file.NewIDSet()
	for _, l := range img.Layers {
		for _, ref := range l.Tree.AllFiles(file.AllTypes()...) {
			reached.Add(ref.ID())
		}
		for _, tr := range []filetree.Reader{l.Tree, l.SquashedTree} {
			for _, ref := range tr.AllFiles(file.AllTypes()...) {
				_, err := img.FileCatalog.Get(ref)
				assert.NoError(t, err, "%s is in a tree but not in the catalog", ref.RealPath)
			}
		}
	}
	entries, err := img.FileCatalog.GetByFileType(file.AllTypes()...)
	require.NoError(t, err)
	for _, e := range entries {
		assert.True(t, reached.Contains(e.ID()), "catalog entry %s (%s) is in no layer tree", e.RealPath, e.Type)
	}
}
