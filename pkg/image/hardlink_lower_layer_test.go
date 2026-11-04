package image

import (
	"archive/tar"
	"context"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// the tar entries below are hand-built: these tests pin how a hardlink header naming a lower layer's file is
// indexed, which only Windows images have, and which follows hcsshim's AddLink (see adoptLowerLayerHardLinks)

var lowerLayerSquashModes = []struct {
	name string
	ctx  func() context.Context
}{
	{name: "eager squashes", ctx: context.Background},
	{name: "lazy squashes", ctx: func() context.Context { return WithLazyLayerSquashes(context.Background()) }},
}

const (
	sys32  = "Files/Windows/System32/x.dll"
	winSxS = "Files/Windows/WinSxS/amd64_x/x.dll"
	uvm32  = "UtilityVM/Files/Windows/System32/x.dll"
	uvmSxS = "UtilityVM/Files/Windows/WinSxS/amd64_x/x.dll"
)

func reg(p, contents string) tarEntry {
	return tarEntry{path: p, typeFlag: tar.TypeReg, contents: contents}
}
func lnk(p, target string) tarEntry {
	return tarEntry{path: p, typeFlag: tar.TypeLink, linkPath: target}
}

// readImageWithOS reads layers as an image whose config names os.
func readImageWithOS(t *testing.T, ctx context.Context, os string, layers ...v1.Layer) *Image {
	t.Helper()
	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)
	img := New(configOverrideImage{Image: v1Img, override: func(c *v1.ConfigFile) { c.OS = os }}, file.NewTempDirGenerator("image-test"), t.TempDir())
	t.Cleanup(func() { require.NoError(t, img.Cleanup()) })
	require.NoError(t, img.Read(ctx))
	return img
}

func assertAdopted(t *testing.T, img *Image, p, contents string) {
	t.Helper()
	entry := catalogEntryForTest(t, img, p)
	assert.Equal(t, file.TypeRegular, entry.Metadata.Type, p)
	assert.Equal(t, int64(len(contents)), entry.Metadata.Size(), p)
	got, err := hardlinkContentsForTest(img, p)
	require.NoError(t, err)
	assert.Equal(t, contents, got, p)
}

func assertNotAdopted(t *testing.T, img *Image, p string) {
	t.Helper()
	entry := catalogEntryForTest(t, img, p)
	assert.Equal(t, file.TypeHardLink, entry.Metadata.Type, p)
	assert.Zero(t, entry.Metadata.Size(), p)
}

// the WinSxS shape: an update layer names a file only the base layer has. The name belongs to its own layer, MIME
// search finds it, and replacing the target later doesn't change what the link got
func TestHardLinkSemantics_WindowsLinkIntoLowerLayer(t *testing.T) {
	for _, mode := range lowerLayerSquashModes {
		t.Run(mode.name, func(t *testing.T) {
			img := readImageWithOS(t, mode.ctx(), "windows",
				layerFromTarEntries(t, reg(sys32, "REAL")),
				layerFromTarEntries(t, lnk(winSxS, sys32)),
				layerFromTarEntries(t, reg(sys32, "REPLACED")),
			)

			assertAdopted(t, img, "/"+winSxS, "REAL")
			entry := catalogEntryForTest(t, img, "/"+winSxS)
			assert.Equal(t, "text/plain", entry.Metadata.MIMEType)
			assert.Equal(t, uint(1), img.FileCatalog.Layer(entry.Reference).Metadata.Index)

			resolutions, err := img.SquashedSearchContext.SearchByMIMEType("text/plain")
			require.NoError(t, err)
			var paths []string
			for _, r := range resolutions {
				paths = append(paths, string(r.RequestPath))
			}
			assert.Contains(t, paths, "/"+winSxS)
			assert.Empty(t, img.SquashedTree().AllFiles(file.TypeHardLink))
			assert.Empty(t, img.Layers[1].deferredHardLinks)
		})
	}
}

// a Linux builder keeps a hardlink and its target in one layer, and a Linux applier could resolve a cross-layer name
// several ways, so a Linux image's link stays as its header describes it
func TestHardLinkSemantics_LinuxLinkIntoLowerLayerIsNotAdopted(t *testing.T) {
	for _, os := range []string{"linux", ""} {
		t.Run(os, func(t *testing.T) {
			img := readImageWithOS(t, context.Background(), os,
				layerFromTarEntries(t, reg(sys32, "REAL")),
				layerFromTarEntries(t, lnk(winSxS, sys32)),
			)
			assertNotAdopted(t, img, "/"+winSxS)
		})
	}
}

func TestHardLinkSemantics_WindowsLowerLayerLookup(t *testing.T) {
	tests := []struct {
		name   string
		layers [][]tarEntry
		// link is the name checked, winSxS when empty
		link string
		// want is the link's contents, or "" when it stays a bare hardlink
		want string
	}{
		{
			name:   "the nearest layer with the target wins",
			layers: [][]tarEntry{{reg(sys32, "OLD")}, {reg(sys32, "NEW")}, {lnk(winSxS, sys32)}},
			want:   "NEW",
		},
		{
			// hcsshim looks in each parent's own files, so a whiteout in between doesn't stop it
			name:   "a whiteout in between is not consulted",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {reg("Files/Windows/System32/.wh.x.dll", "")}, {lnk(winSxS, sys32)}},
			want:   "REAL",
		},
		{
			name:   "a link adopted in a lower layer is itself a target",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {lnk("Files/a.dll", sys32)}, {lnk(winSxS, "Files/a.dll")}},
			want:   "REAL",
		},
		{
			name:   "a chain within the layer shares the inode",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {lnk("Files/a.dll", sys32), lnk(winSxS, "Files/a.dll")}},
			want:   "REAL",
		},
		{
			name:   "a utility VM target",
			layers: [][]tarEntry{{reg(uvm32, "REAL")}, {reg("Files/other.dll", "X")}, {lnk(uvmSxS, uvm32)}},
			link:   uvmSxS,
			want:   "REAL",
		},
		{
			// the utility VM is cloned from the parent, so a whiteout there really deletes the file
			name:   "a utility VM target whited out in between",
			layers: [][]tarEntry{{reg(uvm32, "REAL")}, {reg("UtilityVM/Files/Windows/System32/.wh.x.dll", "")}, {lnk(uvmSxS, uvm32)}},
			link:   uvmSxS,
		},
		{
			name:   "a utility VM target under a directory whited out in between",
			layers: [][]tarEntry{{reg(uvm32, "REAL")}, {reg("UtilityVM/Files/Windows/.wh.System32", "")}, {lnk(uvmSxS, uvm32)}},
			link:   uvmSxS,
		},
		{
			// the link is created after the layer's files, so it would get this layer's file
			name:   "a utility VM target this layer writes after the link",
			layers: [][]tarEntry{{reg(uvm32, "OLD")}, {lnk(uvmSxS, uvm32), reg(uvm32, "NEW")}},
			link:   uvmSxS,
		},
		{
			name:   "in no layer below",
			layers: [][]tarEntry{{reg("Files/other.dll", "X")}, {lnk(winSxS, sys32)}},
		},
		{
			// an NTFS lookup would find the nearer layer's file, which the literal path doesn't name
			name:   "a nearer layer has the name in another case",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {reg("Files/windows/system32/X.DLL", "EVIL")}, {lnk(winSxS, sys32)}},
		},
		{
			// an NTFS directory can't hold both, so which one Windows would see is unknown
			name:   "the target's layer has the name in two cases",
			layers: [][]tarEntry{{reg(sys32, "REAL"), reg("Files/windows/system32/X.DLL", "EVIL")}, {lnk(winSxS, sys32)}},
		},
		{
			name:   "a nearer layer has an ancestor in another case that is not a directory",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {tarEntry{path: "Files/WINDOWS", typeFlag: tar.TypeSymlink, linkPath: "/elsewhere"}}, {lnk(winSxS, sys32)}},
		},
		{
			name:   "a reparse point on the way in the target's layer",
			layers: [][]tarEntry{{tarEntry{path: "Files/Windows", typeFlag: tar.TypeSymlink, linkPath: "/elsewhere"}, reg(sys32, "REAL")}, {lnk(winSxS, sys32)}},
		},
		{
			name:   "a target outside Files",
			layers: [][]tarEntry{{reg("Hives/x", "REAL")}, {lnk(winSxS, "Hives/x")}},
		},
		{
			// hcsshim cleans without a root, so this climbs out of Files and fails the import
			name:   "a target that climbs out of Files",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {lnk(winSxS, "../"+sys32)}},
		},
		{
			name:   "a target with a backslash",
			layers: [][]tarEntry{{reg(`Files/Windows\System32/x.dll`, "REAL")}, {lnk(winSxS, `Files/Windows\System32/x.dll`)}},
		},
		{
			name:   "a target that is a directory",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {lnk(winSxS, "Files/Windows/System32")}},
		},
		{
			// links are created after the layer's files, so this one would fail the import
			name:   "a link replaced by a later header binds nothing",
			layers: [][]tarEntry{{reg(sys32, "REAL")}, {lnk("Files/a.dll", sys32), lnk(winSxS, "Files/a.dll"), reg("Files/a.dll", "MINE")}},
		},
	}
	for _, tt := range tests {
		for _, mode := range lowerLayerSquashModes {
			t.Run(tt.name+"/"+mode.name, func(t *testing.T) {
				var layers []v1.Layer
				for _, entries := range tt.layers {
					layers = append(layers, layerFromTarEntries(t, entries...))
				}
				img := readImageWithOS(t, mode.ctx(), "windows", layers...)
				link := tt.link
				if link == "" {
					link = winSxS
				}
				if tt.want == "" {
					assertNotAdopted(t, img, "/"+link)
					return
				}
				assertAdopted(t, img, "/"+link, tt.want)
			})
		}
	}
}

// the name has to be under Files too, as hcsshim refuses the link otherwise
func TestHardLinkSemantics_WindowsLinkNameOutsideFiles(t *testing.T) {
	img := readImageWithOS(t, context.Background(), "windows",
		layerFromTarEntries(t, reg(sys32, "REAL")),
		layerFromTarEntries(t, lnk("Hives/x", sys32)),
	)
	assertNotAdopted(t, img, "/Hives/x")
}

// a later header at a deferred link's path wins, and the link's catalog entry goes with the node it lost
func TestHardLinkSemantics_WindowsDeferredLinkReplacedByLaterHeader(t *testing.T) {
	img := readImageWithOS(t, context.Background(), "windows",
		layerFromTarEntries(t, reg(sys32, "REAL")),
		layerFromTarEntries(t, lnk(winSxS, sys32), reg(winSxS, "MINE")),
	)
	assertAdopted(t, img, "/"+winSxS, "MINE")
	assertNoOrphanedCatalogEntries(t, img)
}

// a standalone Layer.Read has no layers below, so its links stay deferred and as their headers describe them
func TestLayerRead_KeepsLowerLayerHardLinkPending(t *testing.T) {
	layer := NewLayer(layerFromTarEntries(t, lnk(winSxS, sys32)))
	require.NoError(t, layer.Read(NewFileCatalog(), 0, t.TempDir()))
	t.Cleanup(func() { require.Empty(t, closeLayers([]*Layer{layer})) })
	require.Len(t, layer.deferredHardLinks, 1)
	assert.Equal(t, file.TypeHardLink, layer.deferredHardLinks[0].header.Type)
}
