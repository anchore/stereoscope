package image

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/tree"
)

const lazyFixtureLayers = 6

// lazySquashFixtureLayers returns layers whose squashes differ at every level, so a squash built from the
// wrong layer, in the wrong order, or onto the wrong lower squash shows up. Beyond plain overwrites it has
// a whiteout (layer 2 removes gone.txt), an opaque dir (layer 2 hides opaque/lower.txt), a symlink added in
// one layer whose target is replaced in a later one (link-to-target -> /etc/target.txt), and a hardlink.
func lazySquashFixtureLayers(t *testing.T) []v1.Layer {
	t.Helper()
	reg, sym, lnk := byte(tar.TypeReg), byte(tar.TypeSymlink), byte(tar.TypeLink)

	var layers []v1.Layer
	for idx := 0; idx < lazyFixtureLayers; idx++ {
		entries := []tarEntry{
			{path: fmt.Sprintf("only-in-%d.txt", idx), typeFlag: reg, contents: fmt.Sprintf("c%d", idx)},
			{path: "shared.txt", typeFlag: reg, contents: fmt.Sprintf("v%d", idx)},
		}
		switch idx {
		case 0:
			entries = append(entries,
				tarEntry{path: "opaque/lower.txt", typeFlag: reg, contents: "hidden by the opaque dir above"},
				tarEntry{path: "gone.txt", typeFlag: reg, contents: "removed by the whiteout above"},
				tarEntry{path: "etc/target.txt", typeFlag: reg, contents: "t0"},
			)
		case 1:
			entries = append(entries,
				tarEntry{path: "link-to-target", typeFlag: sym, linkPath: "/etc/target.txt"},
				tarEntry{path: "hard-source.txt", typeFlag: reg, contents: "h1"},
				tarEntry{path: "hard-link.txt", typeFlag: lnk, linkPath: "hard-source.txt"},
			)
		case 2:
			entries = append(entries,
				tarEntry{path: "opaque/.wh..wh..opq", typeFlag: reg},
				tarEntry{path: "opaque/upper.txt", typeFlag: reg, contents: "visible"},
				tarEntry{path: ".wh.gone.txt", typeFlag: reg},
			)
		case 3:
			entries = append(entries, tarEntry{path: "etc/target.txt", typeFlag: reg, contents: "t3"})
		}
		layers = append(layers, layerFromTarEntries(t, entries...))
	}
	return layers
}

// layerTreeSpy is a layer tree that calls onRead the first time a lazy squash build reads it. A build reads
// its layer tree through Copy (the lowest layer, via UnionFileTree.Squash) or Merge (HasPath and TreeReader),
// and nothing else reads the tree handed to NewLazySquash, so the first read is the build starting. Each
// squash builds at most once (pkg/filetree pins that), so firing once per tree is enough.
type layerTreeSpy struct {
	filetree.ReadWriter
	once   sync.Once
	onRead func()
}

func (s *layerTreeSpy) read() { s.once.Do(s.onRead) }

func (s *layerTreeSpy) Copy() (filetree.ReadWriter, error) {
	s.read()
	return s.ReadWriter.Copy()
}

func (s *layerTreeSpy) HasPath(p file.Path, options ...filetree.LinkResolutionOption) bool {
	s.read()
	return s.ReadWriter.HasPath(p, options...)
}

func (s *layerTreeSpy) TreeReader() tree.Reader {
	s.read()
	return s.ReadWriter.TreeReader()
}

// onLazySquashBuild returns an Image.wrapLazyLayerTree that calls fn with the layer index when that layer's
// lazy squash starts building.
func onLazySquashBuild(fn func(idx int)) func(int, filetree.ReadWriter) filetree.ReadWriter {
	return func(idx int, layerTree filetree.ReadWriter) filetree.ReadWriter {
		return &layerTreeSpy{ReadWriter: layerTree, onRead: func() { fn(idx) }}
	}
}

// lazyBuildCounter counts lazy layer squash builds per layer index, for onLazySquashBuild.
type lazyBuildCounter struct {
	mu     sync.Mutex
	counts map[int]int
}

func (c *lazyBuildCounter) hook(idx int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts == nil {
		c.counts = map[int]int{}
	}
	c.counts[idx]++
}

func (c *lazyBuildCounter) snapshot() map[int]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int]int{}
	for k, v := range c.counts {
		out[k] = v
	}
	return out
}

// readLazyImage reads the layers with WithLazyLayerSquashes, counting lazy builds.
func readLazyImage(t *testing.T, layers ...v1.Layer) (*Image, *lazyBuildCounter) {
	t.Helper()
	img := newImageFromLayers(t, layers...)
	counter := &lazyBuildCounter{}
	img.wrapLazyLayerTree = onLazySquashBuild(counter.hook)
	require.NoError(t, img.Read(WithLazyLayerSquashes(context.Background())))
	return img, counter
}

// eagerSquashesOf builds every layer's squash the way an eager read does, from the given image's own
// layer trees, so the result shares file references with that image and can be compared exactly.
func eagerSquashesOf(t *testing.T, img *Image) []filetree.ReadWriter {
	t.Helper()
	var squashes []filetree.ReadWriter
	var lower filetree.ReadWriter
	for idx, layer := range img.Layers {
		sq, err := squashOnto(lower, layer.Tree.(filetree.ReadWriter), true)
		require.NoErrorf(t, err, "layer %d", idx)
		squashes = append(squashes, sq)
		lower = sq
	}
	return squashes
}

// refsOf returns every file reference in the tree, sorted, for exact comparison (same IDs, same paths).
func refsOf(tree filetree.Reader) []file.Reference {
	refs := tree.AllFiles(file.AllTypes()...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID() < refs[j].ID() })
	return refs
}

func sortedPaths(paths []file.Path) []file.Path {
	out := append([]file.Path(nil), paths...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func readAllFrom(t *testing.T, open func() (io.ReadCloser, error)) string {
	t.Helper()
	rc, err := open()
	require.NoError(t, err)
	b, err := io.ReadAll(rc)
	require.NoError(t, rc.Close())
	require.NoError(t, err)
	return string(b)
}

func TestImage_Read_lazyLayerSquashesBuildNothingDuringRead(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)

	assert.Empty(t, counter.snapshot(), "a lazy read built lower layer squashes")

	top := len(img.Layers) - 1
	assert.IsType(t, &filetree.FileTree{}, img.Layers[top].SquashedTree, "the image squash must be built during the read")
	for idx, layer := range img.Layers[:top] {
		require.NotNilf(t, layer.SquashedTree, "layer %d", idx)
		require.NotNilf(t, layer.SquashedSearchContext, "layer %d", idx)
		_, isTree := layer.SquashedTree.(*filetree.FileTree)
		assert.Falsef(t, isTree, "layer %d squash was built during the read", idx)
	}

	// using only the image squash, through every image-level accessor, builds nothing below it
	_ = img.SquashedTree().AllFiles()
	_, err := img.SquashedSearchContext.SearchByGlob("**/*")
	require.NoError(t, err)
	_ = readAllFrom(t, func() (io.ReadCloser, error) { return img.OpenPathFromSquash("/shared.txt") })
	_, err = img.FilesByMIMETypeFromSquash("text/plain")
	require.NoError(t, err)
	ref := img.SquashedTree().AllFiles(file.AllTypes()...)[0]
	_, err = img.ResolveLinkByImageSquash(ref)
	require.NoError(t, err)
	_, err = img.ResolveLinkByLayerSquash(ref, top)
	require.NoError(t, err)
	_, err = img.Layers[top].FilesByMIMETypeFromSquash("text/plain")
	require.NoError(t, err)

	assert.Empty(t, counter.snapshot(), "using the image squash built lower layer squashes")
}

func TestImage_Read_lazyLayerSquashesMatchEager(t *testing.T) {
	layers := lazySquashFixtureLayers(t)
	eager := readImageFromLayers(t, layers...)
	img, counter := readLazyImage(t, layers...)

	// the image squash is the same however it was built
	assert.Equal(t,
		fileTuples(t, eager.SquashedTree(), eager.FileCatalog),
		fileTuples(t, img.SquashedTree(), img.FileCatalog),
		"image squash differs from an eager read")

	// exact comparison against eager squashes built from this image's own layer trees: same references
	want := eagerSquashesOf(t, img)
	require.Len(t, img.Layers, len(want))

	queries := []file.Path{
		"/", "/shared.txt", "/gone.txt", "/opaque", "/opaque/lower.txt", "/opaque/upper.txt",
		"/etc/target.txt", "/link-to-target", "/hard-link.txt", "/hard-source.txt", "/missing",
	}
	for idx, layer := range img.Layers {
		w := want[idx]
		assert.Equalf(t, refsOf(w), refsOf(layer.SquashedTree), "layer %d references", idx)
		assert.Equalf(t, sortedPaths(w.AllRealPaths()), sortedPaths(layer.SquashedTree.AllRealPaths()), "layer %d paths", idx)

		for _, q := range queries {
			wantOK, wantRes, wantErr := w.File(q, filetree.FollowBasenameLinks)
			gotOK, gotRes, gotErr := layer.SquashedTree.File(q, filetree.FollowBasenameLinks)
			assert.Equalf(t, wantOK, gotOK, "layer %d File(%s) exists", idx, q)
			assert.Equalf(t, wantRes, gotRes, "layer %d File(%s)", idx, q)
			assert.Equalf(t, wantErr, gotErr, "layer %d File(%s) err", idx, q)
			assert.Equalf(t, w.HasPath(q), layer.SquashedTree.HasPath(q), "layer %d HasPath(%s)", idx, q)
		}

		wantList, err := w.ListPaths("/")
		require.NoError(t, err)
		gotList, err := layer.SquashedTree.ListPaths("/")
		require.NoError(t, err)
		assert.Equalf(t, sortedPaths(wantList), sortedPaths(gotList), "layer %d ListPaths", idx)

		wantGlob, err := w.FilesByGlob("**/*.txt", filetree.FollowBasenameLinks)
		require.NoError(t, err)
		gotGlob, err := layer.SquashedTree.FilesByGlob("**/*.txt", filetree.FollowBasenameLinks)
		require.NoError(t, err)
		assert.ElementsMatchf(t, wantGlob, gotGlob, "layer %d FilesByGlob", idx)

		// searches go through the (possibly lazy) search context and must match one over the eager tree
		wantSearch := filetree.NewSearchContext(w, img.FileCatalog.(*FileCatalog).Index)
		for _, glob := range []string{"**/*", "**/*.txt", "/link-to-target", "**/target.txt"} {
			wantRes, err := wantSearch.SearchByGlob(glob, filetree.FollowBasenameLinks)
			require.NoError(t, err)
			gotRes, err := layer.SquashedSearchContext.SearchByGlob(glob, filetree.FollowBasenameLinks)
			require.NoError(t, err)
			assert.ElementsMatchf(t, wantRes, gotRes, "layer %d SearchByGlob(%s)", idx, glob)
		}
		wantPath, err := wantSearch.SearchByPath("/link-to-target", filetree.FollowBasenameLinks)
		require.NoError(t, err)
		gotPath, err := layer.SquashedSearchContext.SearchByPath("/link-to-target", filetree.FollowBasenameLinks)
		require.NoError(t, err)
		assert.Equalf(t, wantPath, gotPath, "layer %d SearchByPath", idx)
		wantMIME, err := wantSearch.SearchByMIMEType("text/plain")
		require.NoError(t, err)
		gotMIME, err := layer.SquashedSearchContext.SearchByMIMEType("text/plain")
		require.NoError(t, err)
		assert.ElementsMatchf(t, wantMIME, gotMIME, "layer %d SearchByMIMEType", idx)
	}

	// every lower layer was built exactly once, even though each was touched many times and building
	// layer k needs every layer below it
	built := counter.snapshot()
	for idx := 0; idx < len(img.Layers)-1; idx++ {
		assert.Equalf(t, 1, built[idx], "layer %d build count", idx)
	}
	assert.NotContains(t, built, len(img.Layers)-1, "the image squash must never be built lazily")
}

func TestImage_Read_lazyLayerSquashesHaveTheRightContents(t *testing.T) {
	// independent of how eager builds squashes: what each layer's squash must show, by construction
	img, _ := readLazyImage(t, lazySquashFixtureLayers(t)...)

	for idx, layer := range img.Layers {
		t.Run(fmt.Sprintf("layer %d", idx), func(t *testing.T) {
			sq := layer.SquashedTree
			for other := 0; other < lazyFixtureLayers; other++ {
				assert.Equalf(t, other <= idx, sq.HasPath(file.Path(fmt.Sprintf("/only-in-%d.txt", other))), "only-in-%d.txt", other)
			}
			assert.Equal(t, idx < 2, sq.HasPath("/gone.txt"), "whiteout in layer 2")
			assert.Equal(t, idx < 2, sq.HasPath("/opaque/lower.txt"), "opaque dir in layer 2")
			assert.Equal(t, idx >= 2, sq.HasPath("/opaque/upper.txt"))
			assert.Equal(t, idx >= 1, sq.HasPath("/link-to-target"))
			assert.Equal(t, idx >= 1, sq.HasPath("/hard-link.txt"))
			for _, p := range sq.AllRealPaths() {
				assert.Falsef(t, p.IsWhiteout(), "squash carries whiteout %s", p)
			}

			assert.Equal(t, fmt.Sprintf("v%d", idx), readAllFrom(t, func() (io.ReadCloser, error) {
				return layer.OpenPathFromSquash("/shared.txt")
			}))
			assert.Equal(t, fmt.Sprintf("v%d", idx), readAllFrom(t, func() (io.ReadCloser, error) {
				return layer.FileContentsFromSquash("/shared.txt") //nolint:staticcheck // deprecated, still covered
			}))

			if idx >= 1 {
				wantTarget := "t0"
				if idx >= 3 {
					wantTarget = "t3"
				}
				assert.Equal(t, wantTarget, readAllFrom(t, func() (io.ReadCloser, error) {
					return layer.OpenPathFromSquash("/link-to-target")
				}), "symlink must follow the target visible at this layer")

				_, linkRes, err := sq.File("/link-to-target")
				require.NoError(t, err)
				require.True(t, linkRes.HasReference())
				resolved, err := img.ResolveLinkByLayerSquash(*linkRes.Reference, idx)
				require.NoError(t, err)
				require.NotNil(t, resolved)
				assert.Equal(t, file.Path("/etc/target.txt"), resolved.RealPath)
			}

			refs, err := layer.FilesByMIMETypeFromSquash("text/plain") //nolint:staticcheck // deprecated, still covered
			require.NoError(t, err)
			assert.NotEmpty(t, refs)
		})
	}
}

func TestImage_Read_lazyLayerSquashesBuildOnlyWhatIsTouched(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)

	// touching layer 2 needs 0 and 1 underneath it, and nothing above
	assert.True(t, img.Layers[2].SquashedTree.HasPath("/opaque/upper.txt"))
	assert.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, counter.snapshot())

	// layers already built are not built again, from the tree or the search context
	assert.True(t, img.Layers[1].SquashedTree.HasPath("/link-to-target"))
	_, err := img.Layers[0].SquashedSearchContext.SearchByGlob("**/*")
	require.NoError(t, err)
	_, err = img.Layers[2].SquashedSearchContext.SearchByPath("/shared.txt")
	require.NoError(t, err)
	assert.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, counter.snapshot())

	// a search on an unbuilt layer builds it (and only it, everything below is built already)
	_, err = img.Layers[4].SquashedSearchContext.SearchByMIMEType("text/plain")
	require.NoError(t, err)
	assert.Equal(t, map[int]int{0: 1, 1: 1, 2: 1, 3: 1, 4: 1}, counter.snapshot())
}

func TestImage_Read_lazyLayerSquashesLeaveSharedTreesAlone(t *testing.T) {
	img, _ := readLazyImage(t, lazySquashFixtureLayers(t)...)

	layerRefs := make([][]file.Reference, len(img.Layers))
	layerPaths := make([][]file.Path, len(img.Layers))
	for idx, layer := range img.Layers {
		layerRefs[idx] = refsOf(layer.Tree)
		layerPaths[idx] = sortedPaths(layer.Tree.AllRealPaths())
	}
	imageRefs := refsOf(img.SquashedTree())
	imagePaths := sortedPaths(img.SquashedTree().AllRealPaths())

	// build every lazy squash, then hand callers copies to write into
	for idx, layer := range img.Layers {
		cp, err := layer.SquashedTree.Copy()
		require.NoErrorf(t, err, "layer %d", idx)
		_, err = cp.AddFile("/written-into-a-copy")
		require.NoError(t, err)
		require.NoError(t, cp.RemovePath("/shared.txt"))
		assert.Falsef(t, layer.SquashedTree.HasPath("/written-into-a-copy"), "layer %d squash changed by a write to its copy", idx)
		assert.Truef(t, layer.SquashedTree.HasPath("/shared.txt"), "layer %d squash changed by a write to its copy", idx)
	}

	// layer trees are what every lazy build reads, and the image squash was built in place: neither may
	// have been changed by building the lazy squashes
	for idx, layer := range img.Layers {
		assert.Equalf(t, layerRefs[idx], refsOf(layer.Tree), "layer %d tree references", idx)
		assert.Equalf(t, layerPaths[idx], sortedPaths(layer.Tree.AllRealPaths()), "layer %d tree paths", idx)
	}
	assert.Equal(t, imageRefs, refsOf(img.SquashedTree()))
	assert.Equal(t, imagePaths, sortedPaths(img.SquashedTree().AllRealPaths()))
	assert.False(t, img.SquashedTree().HasPath("/written-into-a-copy"))
}

func TestImage_Read_lazyLayerSquashesSingleLayer(t *testing.T) {
	// with one layer there is nothing below the image squash, so nothing is lazy
	img, counter := readLazyImage(t, layerFromTarEntries(t,
		tarEntry{path: "a.txt", typeFlag: tar.TypeReg, contents: "a"},
		tarEntry{path: ".wh.never-there", typeFlag: tar.TypeReg},
	))
	require.Len(t, img.Layers, 1)
	assert.IsType(t, &filetree.FileTree{}, img.Layers[0].SquashedTree)
	assert.Same(t, img.Layers[0].SquashedTree, img.SquashedTree())
	assert.True(t, img.SquashedTree().HasPath("/a.txt"))
	assert.False(t, img.SquashedTree().HasPath("/.wh.never-there"), "the image squash must not carry whiteouts")
	assert.Equal(t, "a", readAllFrom(t, func() (io.ReadCloser, error) { return img.OpenPathFromSquash("/a.txt") }))
	assert.Empty(t, counter.snapshot())
}

func TestImage_Read_lazyLayerSquashesTwoLayers(t *testing.T) {
	// the smallest image with a lazy layer: layer 0 is lazy and built from nothing below it
	img, counter := readLazyImage(t,
		layerFromTarEntries(t,
			tarEntry{path: "a.txt", typeFlag: tar.TypeReg, contents: "a0"},
			tarEntry{path: ".wh.never-there", typeFlag: tar.TypeReg},
		),
		layerFromTarEntries(t, tarEntry{path: "a.txt", typeFlag: tar.TypeReg, contents: "a1"}),
	)
	require.Len(t, img.Layers, 2)
	assert.Equal(t, "a0", readAllFrom(t, func() (io.ReadCloser, error) { return img.Layers[0].OpenPathFromSquash("/a.txt") }))
	assert.Equal(t, "a1", readAllFrom(t, func() (io.ReadCloser, error) { return img.OpenPathFromSquash("/a.txt") }))
	assert.False(t, img.Layers[0].SquashedTree.HasPath("/.wh.never-there"), "the lowest lazy squash must drop its whiteouts")
	assert.Equal(t, map[int]int{0: 1}, counter.snapshot())
}

func TestImage_Read_lazyLayerSquashesFreshOnEveryRead(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	firstLayers := img.Layers

	// build everything from the first read
	for _, layer := range firstLayers {
		_ = layer.SquashedTree.AllFiles()
	}
	require.Len(t, counter.snapshot(), lazyFixtureLayers-1)

	// a second lazy read must not reuse any lazy state from the first
	second := &lazyBuildCounter{}
	img.wrapLazyLayerTree = onLazySquashBuild(second.hook)
	require.NoError(t, img.Read(WithLazyLayerSquashes(context.Background())))
	require.Len(t, img.Layers, lazyFixtureLayers)
	for idx := range img.Layers {
		assert.NotSamef(t, firstLayers[idx], img.Layers[idx], "layer %d was reused", idx)
	}
	assert.Empty(t, second.snapshot(), "a fresh read built lower squashes")
	for idx, layer := range img.Layers {
		assert.Equal(t, fmt.Sprintf("v%d", idx), readAllFrom(t, func() (io.ReadCloser, error) {
			return layer.OpenPathFromSquash("/shared.txt")
		}))
	}
	for idx := 0; idx < lazyFixtureLayers-1; idx++ {
		assert.Equalf(t, 1, second.snapshot()[idx], "layer %d build count on the second read", idx)
	}

	// and an eager read after a lazy one is fully eager again
	require.NoError(t, img.Read(context.Background()))
	for idx, layer := range img.Layers {
		assert.IsTypef(t, &filetree.FileTree{}, layer.SquashedTree, "layer %d", idx)
	}
}

func TestImage_Read_eagerByDefault(t *testing.T) {
	img := readImageFromLayers(t, lazySquashFixtureLayers(t)...)
	for idx, layer := range img.Layers {
		assert.IsTypef(t, &filetree.FileTree{}, layer.SquashedTree, "layer %d squash must be built during a default read", idx)
	}
	// and a context value set to something other than true does not switch it on
	ctx := context.WithValue(context.Background(), lazyLayerSquashesKey{}, "yes")
	assert.False(t, lazyLayerSquashes(ctx))
	assert.True(t, lazyLayerSquashes(WithLazyLayerSquashes(context.Background())))
}

func TestLayer_squashAccessorsOnAnUnreadLayer(t *testing.T) {
	// a layer that was never read has no squash, and the accessors must say so rather than panic
	l := &Layer{Metadata: LayerMetadata{Index: 3}}
	_, err := l.OpenPathFromSquash("/a")
	assert.ErrorContains(t, err, "layer 3 has no squashed tree (squashes are built by Image.Read)")
	_, err = l.FileContentsFromSquash("/a") //nolint:staticcheck // deprecated, still covered
	assert.ErrorContains(t, err, "layer 3 has no squashed tree")
	_, err = l.FilesByMIMETypeFromSquash("text/plain") //nolint:staticcheck // deprecated, still covered
	assert.ErrorContains(t, err, "layer 3 has no squashed tree")

	img := &Image{Layers: []*Layer{l}}
	_, err = img.ResolveLinkByLayerSquash(*file.NewFileReference("/a"), 0)
	assert.ErrorContains(t, err, "layer 3 has no squashed tree")
}

func TestImage_Read_lazyLayerSquashFailureNamesTheLayer(t *testing.T) {
	// a lazy build that panics must fail its own layer and every layer above it with an error that says
	// which layer broke, leave the layers below it working, and never take the caller down
	img := newImageFromLayers(t, lazySquashFixtureLayers(t)...)
	counter := &lazyBuildCounter{}
	img.wrapLazyLayerTree = onLazySquashBuild(func(idx int) {
		counter.hook(idx)
		if idx == 2 {
			panic("boom")
		}
	})
	require.NoError(t, img.Read(WithLazyLayerSquashes(context.Background())))

	for _, idx := range []int{4, 3, 2} {
		_, _, err := img.Layers[idx].SquashedTree.File("/shared.txt")
		require.Errorf(t, err, "layer %d", idx)
		assert.Containsf(t, err.Error(), "failed to squash tree 2: panicked: boom", "layer %d", idx)
		assert.Falsef(t, img.Layers[idx].SquashedTree.HasPath("/shared.txt"), "layer %d must look empty", idx)
		_, err = img.Layers[idx].SquashedSearchContext.SearchByGlob("**/*")
		assert.Errorf(t, err, "layer %d search", idx)
		_, err = img.Layers[idx].OpenPathFromSquash("/shared.txt")
		assert.Errorf(t, err, "layer %d open", idx)
	}
	for _, idx := range []int{0, 1} {
		assert.Equal(t, fmt.Sprintf("v%d", idx), readAllFrom(t, func() (io.ReadCloser, error) {
			return img.Layers[idx].OpenPathFromSquash("/shared.txt")
		}))
	}
	// the image squash was built during the read and is unaffected
	assert.True(t, img.SquashedTree().HasPath("/shared.txt"))

	// failures are final: nothing is retried, however many times the layers are touched. Layers 3 and 4 fail
	// copying the squash below them, before they read their own layer tree, so the hook never sees them
	_ = img.Layers[4].SquashedTree.AllFiles()
	assert.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, counter.snapshot())
}

// describeSquash renders what a caller can observe of one layer's squash (every real path with its type, link
// target and contents, and what its search context resolves the path to) in terms that don't depend on file
// reference IDs, so squashes from two separate reads can be compared.
func describeSquash(t *testing.T, img *Image, idx int) []string {
	t.Helper()
	layer := img.Layers[idx]
	var out []string
	for _, p := range sortedPaths(layer.SquashedTree.AllRealPaths()) {
		_, res, err := layer.SquashedTree.File(p)
		require.NoErrorf(t, err, "layer %d: %s", idx, p)
		if !res.HasReference() {
			// a parent directory no tar entry named
			out = append(out, fmt.Sprintf("%s implied", p))
			continue
		}
		entry, err := img.FileCatalog.Get(*res.Reference)
		require.NoErrorf(t, err, "layer %d: %s", idx, p)
		line := fmt.Sprintf("%s type=%v link=%s", p, entry.Metadata.Type, entry.Metadata.LinkDestination)
		if entry.Metadata.Type == file.TypeRegular {
			line += " contents=" + readAllFrom(t, func() (io.ReadCloser, error) { return layer.OpenPathFromSquash(p) })
		}
		found, err := layer.SquashedSearchContext.SearchByPath(string(p), filetree.FollowBasenameLinks)
		require.NoErrorf(t, err, "layer %d: %s", idx, p)
		if found != nil && found.Reference != nil {
			line += " search=" + string(found.Reference.RealPath)
		}
		out = append(out, line)
	}
	return out
}

func TestImage_Read_lazyLayerSquashesMatchAnEagerRead(t *testing.T) {
	// the expected squashes come from a second, independent eager read, never from the lazy image's own
	// layer trees, so a read that wrote into a layer tree while squashing can't corrupt both sides alike
	layers := lazySquashFixtureLayers(t)
	eager := readImageFromLayers(t, layers...)
	lazy, _ := readLazyImage(t, layers...)
	require.Len(t, lazy.Layers, len(eager.Layers))

	// top down, so the first touch of a high layer builds the chain below it
	for idx := len(eager.Layers) - 1; idx >= 0; idx-- {
		require.NotNilf(t, lazy.Layers[idx].SquashedTree, "layer %d has no squashed tree", idx)
		require.NotNilf(t, lazy.Layers[idx].SquashedSearchContext, "layer %d has no squashed search context", idx)
		assert.Equalf(t, describeSquash(t, eager, idx), describeSquash(t, lazy, idx), "layer %d", idx)
	}
}

// squashWarnings counts the warnings logged through internal/log (as pkg/filetree's lazy tests do, after the
// capturing logger in internal/podman/ssh_knownhosts_test.go).
type squashWarnings struct {
	logger.Logger
	warnings atomic.Int32
}

func (w *squashWarnings) WithFields(...any) logger.MessageLogger { return w }
func (w *squashWarnings) Warn(...any)                            { w.warnings.Add(1) }
func (w *squashWarnings) Warnf(string, ...any)                   { w.warnings.Add(1) }

func TestImage_Read_lazyLayerSquashes_aBrokenLayerIsLoggedOnce(t *testing.T) {
	// one lazy layer that can't be squashed fails every layer above it too, but it is one fault: a caller
	// that searches and reads every layer should see one warning, not one per layer and search context
	counter := &squashWarnings{Logger: discard.New()}
	prev := log.Log
	log.Log = counter
	t.Cleanup(func() { log.Log = prev })

	img := newImageFromLayers(t, lazySquashFixtureLayers(t)...)
	img.wrapLazyLayerTree = onLazySquashBuild(func(idx int) {
		if idx == 2 {
			panic("boom")
		}
	})
	require.NoError(t, img.Read(WithLazyLayerSquashes(context.Background())))

	for idx, layer := range img.Layers[:len(img.Layers)-1] {
		_, err := layer.SquashedSearchContext.SearchByGlob("**/*")
		_, _, fileErr := layer.SquashedTree.File("/shared.txt")
		if idx >= 2 {
			require.Errorf(t, err, "layer %d search", idx)
			require.Errorf(t, fileErr, "layer %d file", idx)
		}
	}
	assert.Equal(t, int32(1), counter.warnings.Load(), "warnings for one broken layer")
}
