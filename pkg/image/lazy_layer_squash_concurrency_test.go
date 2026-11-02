package image

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// the image level concurrency suite for lazy layer squashes.

const lazyConcurrencyGoroutines = 16

var lazyFixtureQueries = []file.Path{
	"/", "/shared.txt", "/gone.txt", "/opaque", "/opaque/lower.txt", "/opaque/upper.txt", "/etc/target.txt",
	"/link-to-target", "/hard-link.txt", "/hard-source.txt", "/only-in-0.txt", "/only-in-5.txt", "/missing",
}

var lazyConcurrencyGlobs = []string{"**/*", "**/*.txt", "**/target.txt", "/link-to-target", "**/link*"}

// squashResult is one call's outcome, and squashObs is every call made against one layer's squash, keyed
// by call and argument.
type squashResult struct {
	Exists  bool
	Res     []file.Resolution
	Refs    []file.Reference
	Content string
	Err     string
}

type squashObs map[string]squashResult

// observeSquash makes every squash call the public API offers against layers[idx], the way an
// all-layers consumer would. It must be safe to call from any goroutine, so it takes no *testing.T.
func observeSquash(layers []*Layer, idx int, queries []file.Path) squashObs {
	img := &Image{Layers: layers}
	l := layers[idx]
	obs := squashObs{}
	for _, q := range queries {
		exists, res, err := l.SquashedTree.File(q, filetree.FollowBasenameLinks)
		obs["file "+string(q)] = squashResult{Exists: exists, Res: resList(res), Err: errString(err)}

		res, err = l.SquashedSearchContext.SearchByPath(string(q), filetree.FollowBasenameLinks)
		obs["path "+string(q)] = squashResult{Res: resList(res), Err: errString(err)}

		res, err = img.ResolveLinkByLayerSquash(*file.NewFileReference(q), idx)
		obs["resolve "+string(q)] = squashResult{Res: resList(res), Err: errString(err)}

		obs["open "+string(q)] = openResult(l.OpenPathFromSquash(q))
	}
	for _, g := range lazyConcurrencyGlobs {
		res, err := l.SquashedSearchContext.SearchByGlob(g, filetree.FollowBasenameLinks)
		obs["glob "+g] = squashResult{Res: sortedRes(res), Err: errString(err)}
	}
	res, err := l.SquashedSearchContext.SearchByMIMEType("text/plain")
	obs["mime"] = squashResult{Res: sortedRes(res), Err: errString(err)}
	refs, err := l.FilesByMIMETypeFromSquash("text/plain") //nolint:staticcheck // deprecated, still covered
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID() < refs[j].ID() })
	obs["mime refs"] = squashResult{Refs: refs, Err: errString(err)}
	return obs
}

// withoutContents drops the content reads, for comparisons where the layer tars may be closed.
func (o squashObs) withoutContents() squashObs {
	out := squashObs{}
	for k, v := range o {
		if !strings.HasPrefix(k, "open ") {
			out[k] = v
		}
	}
	return out
}

// norm renders the observation by path only, so observations from two different reads can be compared.
func (o squashObs) norm() []string {
	var out []string
	for k, r := range o {
		var res, refs []string
		for _, x := range r.Res {
			res = append(res, resKey(&x))
		}
		sort.Strings(res)
		for _, x := range r.Refs {
			refs = append(refs, string(x.RealPath))
		}
		sort.Strings(refs)
		out = append(out, fmt.Sprintf("%s: exists=%v res=%v refs=%v content=%q err=%q", k, r.Exists, res, refs, r.Content, r.Err))
	}
	sort.Strings(out)
	return out
}

func resKey(r *file.Resolution) string {
	s := string(r.RequestPath) + "=>"
	if r.Reference != nil {
		s += string(r.RealPath)
	}
	for _, l := range r.LinkResolutions {
		s += " via " + resKey(&l)
	}
	return s
}

func resList(r *file.Resolution) []file.Resolution {
	if r == nil {
		return nil
	}
	return []file.Resolution{*r}
}

func sortedRes(rs []file.Resolution) []file.Resolution {
	out := append([]file.Resolution(nil), rs...)
	sort.Slice(out, func(i, j int) bool { return resKey(&out[i]) < resKey(&out[j]) })
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func openResult(rc io.ReadCloser, err error) squashResult {
	if err != nil {
		return squashResult{Err: "open failed"}
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return squashResult{Err: "read failed"}
	}
	return squashResult{Content: string(b)}
}

// eagerShadowLayers stands eagerly built squashes (see eagerSquashesOf) in for the given layers' own,
// sharing their trees and catalog, so the same calls on both compare exactly, references included.
func eagerShadowLayers(t *testing.T, layers []*Layer) []*Layer {
	t.Helper()
	squashes := eagerSquashesOf(t, &Image{Layers: layers})
	out := make([]*Layer, len(layers))
	for idx, l := range layers {
		out[idx] = &Layer{
			Metadata:              l.Metadata,
			Tree:                  l.Tree,
			SquashedTree:          squashes[idx],
			SquashedSearchContext: filetree.NewSearchContext(squashes[idx], l.fileCatalog.Index),
			fileCatalog:           l.fileCatalog,
		}
	}
	return out
}

func observeAll(layers []*Layer, queries []file.Path) []squashObs {
	out := make([]squashObs, len(layers))
	for idx := range layers {
		out[idx] = observeSquash(layers, idx, queries)
	}
	return out
}

// runTogether runs fn on n goroutines lined up behind a start barrier so their first touches overlap. A
// panic fails the test instead of crashing the binary, and a hang fails it with every stack.
func runTogether(t *testing.T, n int, fn func(g int)) {
	t.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for g := range n {
		wg.Go(func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("goroutine %d panicked: %v\n%s", g, p, debug.Stack())
				}
			}()
			<-start
			fn(g)
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	close(start)
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatalf("goroutines did not finish (deadlock?)\n%s", allStacks())
	}
}

func shuffledOrder(_, n int) []int { return rand.Perm(n) }

// upDownOrder sends even goroutines bottom-up and odd ones top-down.
func upDownOrder(g, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
		if g%2 == 1 {
			out[i] = n - 1 - i
		}
	}
	return out
}

func assertLowerLayersBuiltOnce(t *testing.T, c *lazyBuildCounter, layers int) {
	t.Helper()
	want := map[int]int{}
	for idx := 0; idx < layers-1; idx++ {
		want[idx] = 1
	}
	assert.Equal(t, want, c.snapshot(), "every lower layer must build exactly once, the top layer never")
}

func readFixtureImage(t *testing.T, lazy bool, layers ...v1.Layer) (*Image, *lazyBuildCounter) {
	t.Helper()
	if lazy {
		return readLazyImage(t, layers...)
	}
	img := newImageFromLayers(t, layers...)
	counter := &lazyBuildCounter{}
	img.wrapLazyLayerTree = onLazySquashBuild(counter.hook)
	require.NoError(t, img.Read(context.Background()))
	return img, counter
}

func TestImage_lazyLayerSquashesConcurrent_fanOut(t *testing.T) {
	layers := lazySquashFixtureLayers(t)

	// cross-read expectation: a plain eager read of the same layers, compared by path
	eagerImg := readImageFromLayers(t, layers...)
	var wantNorm [][]string
	for _, obs := range observeAll(eagerImg.Layers, lazyFixtureQueries) {
		wantNorm = append(wantNorm, obs.norm())
	}

	orders := []struct {
		name  string
		order func(g, n int) []int
	}{
		{name: "shuffled", order: shuffledOrder},
		{name: "top-down vs bottom-up", order: upDownOrder},
	}
	for _, lazy := range []bool{true, false} {
		for _, o := range orders {
			t.Run(fmt.Sprintf("lazy=%v/%s", lazy, o.name), func(t *testing.T) {
				img, counter := readFixtureImage(t, lazy, layers...)
				// same-read expectation: eager squashes over this image's own trees, compared exactly
				want := observeAll(eagerShadowLayers(t, img.Layers), lazyFixtureQueries)
				require.Empty(t, counter.snapshot())

				runTogether(t, lazyConcurrencyGoroutines, func(g int) {
					for _, idx := range o.order(g, len(img.Layers)) {
						got := observeSquash(img.Layers, idx, lazyFixtureQueries)
						assert.Equalf(t, want[idx], got, "layer %d", idx)
						assert.Equalf(t, wantNorm[idx], got.norm(), "layer %d vs an eager read", idx)
					}
				})

				if lazy {
					assertLowerLayersBuiltOnce(t, counter, len(img.Layers))
				} else {
					assert.Empty(t, counter.snapshot(), "an eager read has nothing to build lazily")
				}
			})
		}
	}
}

func TestImage_lazyLayerSquashesConcurrent_imageSquashUnderLoad(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	want := observeAll(eagerShadowLayers(t, img.Layers), lazyFixtureQueries)

	// snapshot the image squash before any lower layer is touched
	top := img.SquashedTree()
	topSearch := img.SquashedSearchContext
	wantRefs := refsOf(top)
	wantPaths := sortedPaths(top.AllRealPaths())
	wantGlob, err := topSearch.SearchByGlob("**/*", filetree.FollowBasenameLinks)
	require.NoError(t, err)
	wantGlob = sortedRes(wantGlob)
	wantShared := openResult(img.OpenPathFromSquash("/shared.txt"))
	require.Empty(t, counter.snapshot())

	checkImageSquash := func() {
		assert.Same(t, top, img.SquashedTree())
		assert.Equal(t, wantRefs, refsOf(img.SquashedTree()))
		assert.Equal(t, wantPaths, sortedPaths(img.SquashedTree().AllRealPaths()))
		gotGlob, err := img.SquashedSearchContext.SearchByGlob("**/*", filetree.FollowBasenameLinks)
		assert.NoError(t, err)
		assert.Equal(t, wantGlob, sortedRes(gotGlob))
		assert.Equal(t, wantShared, openResult(img.OpenPathFromSquash("/shared.txt")))
	}

	runTogether(t, lazyConcurrencyGoroutines, func(g int) {
		if g%2 == 0 {
			for range 10 {
				checkImageSquash()
			}
			return
		}
		for _, idx := range rand.Perm(len(img.Layers) - 1) {
			assert.Equalf(t, want[idx], observeSquash(img.Layers, idx, lazyFixtureQueries), "layer %d", idx)
		}
	})

	checkImageSquash()
	assertLowerLayersBuiltOnce(t, counter, len(img.Layers))
}

func TestImage_lazyLayerSquashesConcurrent_layerTreesNeverWritten(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)

	// snapshot every layer tree right after the read, before any lazy build reads it
	wantRefs := make([][]file.Reference, len(img.Layers))
	wantPaths := make([][]file.Path, len(img.Layers))
	for idx, l := range img.Layers {
		wantRefs[idx] = refsOf(l.Tree)
		wantPaths[idx] = sortedPaths(l.Tree.AllRealPaths())
	}

	runTogether(t, lazyConcurrencyGoroutines, func(g int) {
		for _, idx := range upDownOrder(g, len(img.Layers)) {
			_ = observeSquash(img.Layers, idx, lazyFixtureQueries)
		}
	})
	assertLowerLayersBuiltOnce(t, counter, len(img.Layers))

	for idx, l := range img.Layers {
		assert.Equalf(t, wantRefs[idx], refsOf(l.Tree), "layer %d tree references", idx)
		assert.Equalf(t, wantPaths[idx], sortedPaths(l.Tree.AllRealPaths()), "layer %d tree paths", idx)
	}
}

func TestImage_lazyLayerSquashesConcurrent_reReadWhileOldLayersInUse(t *testing.T) {
	img, oldCounter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	oldLayers := img.Layers
	oldWant := observeAll(eagerShadowLayers(t, oldLayers), lazyFixtureQueries)

	// build part of the old generation up front, so old layers are a mix of built and unbuilt
	require.True(t, oldLayers[2].SquashedTree.HasPath("/opaque/upper.txt"))
	require.Equal(t, map[int]int{0: 1, 1: 1, 2: 1}, oldCounter.snapshot())

	// the hook is captured per read, so swapping it splits the build counts by generation
	newCounter := &lazyBuildCounter{}
	img.wrapLazyLayerTree = onLazySquashBuild(newCounter.hook)
	require.NoError(t, img.Read(WithLazyLayerSquashes(context.Background())))
	newLayers := img.Layers
	require.Len(t, newLayers, len(oldLayers))
	newWant := observeAll(eagerShadowLayers(t, newLayers), lazyFixtureQueries)
	require.Empty(t, newCounter.snapshot())

	runTogether(t, lazyConcurrencyGoroutines, func(g int) {
		for _, idx := range rand.Perm(len(oldLayers)) {
			if g%2 == 0 {
				// the re-read closed the old layer tars, so only the trees and searches are compared
				got := observeSquash(oldLayers, idx, lazyFixtureQueries)
				assert.Equalf(t, oldWant[idx].withoutContents(), got.withoutContents(), "old layer %d", idx)
			} else {
				assert.Equalf(t, newWant[idx], observeSquash(newLayers, idx, lazyFixtureQueries), "new layer %d", idx)
			}
		}
	})

	assertLowerLayersBuiltOnce(t, oldCounter, len(oldLayers))
	assertLowerLayersBuiltOnce(t, newCounter, len(newLayers))
	for idx := range oldLayers {
		assert.NotSamef(t, oldLayers[idx].SquashedTree, newLayers[idx].SquashedTree, "layer %d squash shared between reads", idx)
	}
}

func TestImage_lazyLayerSquashesConcurrent_cleanupDuringAndAfterBuilds(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	want := observeAll(eagerShadowLayers(t, img.Layers), lazyFixtureQueries)

	runTogether(t, lazyConcurrencyGoroutines+1, func(g int) {
		if g == 0 {
			assert.NoError(t, img.Cleanup())
			return
		}
		// content reads may succeed or fail depending on when Cleanup lands, the rest must not change
		for _, idx := range rand.Perm(len(img.Layers)) {
			got := observeSquash(img.Layers, idx, lazyFixtureQueries)
			assert.Equalf(t, want[idx].withoutContents(), got.withoutContents(), "layer %d during cleanup", idx)
		}
	})
	assertLowerLayersBuiltOnce(t, counter, len(img.Layers))

	// a second Cleanup is harmless (and t.Cleanup makes a third)
	require.NoError(t, img.Cleanup())

	for idx := range img.Layers {
		got := observeSquash(img.Layers, idx, lazyFixtureQueries)
		assert.Equalf(t, want[idx].withoutContents(), got.withoutContents(), "layer %d after cleanup", idx)
		for k, w := range want[idx] {
			if strings.HasPrefix(k, "open ") && w.Err == "" && w.Content != "" {
				assert.NotEmptyf(t, got[k].Err, "layer %d %s still readable after cleanup", idx, k)
			}
		}
	}
	assertLowerLayersBuiltOnce(t, counter, len(img.Layers))
}

func TestImage_lazyLayerSquashesConcurrent_callersMakeTheirOwnSearchers(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	index := img.FileCatalog.(*FileCatalog).Index

	runTogether(t, lazyConcurrencyGoroutines, func(g int) {
		for _, idx := range rand.Perm(len(img.Layers) - 1) {
			l := img.Layers[idx]
			own := filetree.NewSearchContext(l.SquashedTree, index)
			for _, glob := range lazyConcurrencyGlobs {
				gotRes, gotErr := own.SearchByGlob(glob, filetree.FollowBasenameLinks)
				wantRes, wantErr := l.SquashedSearchContext.SearchByGlob(glob, filetree.FollowBasenameLinks)
				assert.Equalf(t, wantErr, gotErr, "layer %d glob %s", idx, glob)
				assert.Equalf(t, sortedRes(wantRes), sortedRes(gotRes), "layer %d glob %s", idx, glob)
			}
			for _, q := range lazyFixtureQueries {
				gotRes, gotErr := own.SearchByPath(string(q), filetree.FollowBasenameLinks)
				wantRes, wantErr := l.SquashedSearchContext.SearchByPath(string(q), filetree.FollowBasenameLinks)
				assert.Equalf(t, wantErr, gotErr, "layer %d path %s", idx, q)
				assert.Equalf(t, wantRes, gotRes, "layer %d path %s", idx, q)
			}
			gotRes, gotErr := own.SearchByMIMEType("text/plain")
			wantRes, wantErr := l.SquashedSearchContext.SearchByMIMEType("text/plain")
			assert.Equalf(t, wantErr, gotErr, "layer %d mime", idx)
			assert.Equalf(t, sortedRes(wantRes), sortedRes(gotRes), "layer %d mime", idx)
		}
	})

	assertLowerLayersBuiltOnce(t, counter, len(img.Layers))
}

func TestImage_lazyLayerSquashesConcurrent_untouchedLayersStayUnbuilt(t *testing.T) {
	img, counter := readLazyImage(t, lazySquashFixtureLayers(t)...)
	top := len(img.Layers) - 1
	want := observeSquash(img.Layers, top, lazyFixtureQueries)
	require.Empty(t, counter.snapshot())

	runTogether(t, lazyConcurrencyGoroutines, func(int) {
		for range 5 {
			// every image level accessor, plus the top layer's own, which is the image squash
			assert.Equal(t, want, observeSquash(img.Layers, top, lazyFixtureQueries))
			_ = img.SquashedTree().AllFiles()
			_, err := img.SquashedSearchContext.SearchByGlob("**/*")
			assert.NoError(t, err)
			_, err = img.FilesByMIMETypeFromSquash("text/plain")
			assert.NoError(t, err)
			_ = openResult(img.OpenPathFromSquash("/link-to-target"))
			_, err = img.ResolveLinkByImageSquash(*file.NewFileReference("/link-to-target"))
			assert.NoError(t, err)
		}
	})

	assert.Empty(t, counter.snapshot(), "using only the image squash built lower layer squashes")
}

// manyLazyLayers generates n layers that overwrite shared files, add a unique file each, and now and then
// white out an earlier unique file, retarget or remove a symlink, or make a directory opaque.
func manyLazyLayers(t *testing.T, n int) ([]v1.Layer, []file.Path) {
	t.Helper()
	reg, sym := byte(tar.TypeReg), byte(tar.TypeSymlink)
	unique := func(idx int) string { return fmt.Sprintf("dir%d/unique-%d.txt", idx%4, idx) }

	queries := []file.Path{"/", "/shared-a.txt", "/shared-b.txt", "/link", "/missing"}
	var layers []v1.Layer
	for idx := range n {
		entries := []tarEntry{
			{path: "shared-a.txt", typeFlag: reg, contents: fmt.Sprintf("a%d", idx)},
			{path: unique(idx), typeFlag: reg, contents: fmt.Sprintf("u%d", idx)},
		}
		queries = append(queries, file.Path("/"+unique(idx)))
		if idx%3 == 0 {
			entries = append(entries, tarEntry{path: "shared-b.txt", typeFlag: reg, contents: fmt.Sprintf("b%d", idx)})
		}
		if idx%5 == 4 {
			entries = append(entries, tarEntry{path: fmt.Sprintf("dir%d/.wh.unique-%d.txt", (idx-2)%4, idx-2), typeFlag: reg})
		}
		if idx%7 == 1 {
			entries = append(entries, tarEntry{path: "link", typeFlag: sym, linkPath: "/" + unique(idx)})
		}
		if idx%10 == 3 {
			entries = append(entries, tarEntry{path: ".wh.link", typeFlag: reg})
		}
		if idx == n/2 {
			entries = append(entries, tarEntry{path: fmt.Sprintf("dir%d/.wh..wh..opq", (idx+1)%4), typeFlag: reg})
		}
		layers = append(layers, layerFromTarEntries(t, entries...))
	}
	for d := range 4 {
		queries = append(queries, file.Path(fmt.Sprintf("/dir%d", d)))
	}
	return layers, queries
}

func TestImage_lazyLayerSquashesConcurrent_manyLayersRandomAccess(t *testing.T) {
	const layerCount = 40
	layers, queries := manyLazyLayers(t, layerCount)
	img, counter := readLazyImage(t, layers...)
	require.Len(t, img.Layers, layerCount)
	shadow := eagerShadowLayers(t, img.Layers)
	require.Empty(t, counter.snapshot())

	runTogether(t, lazyConcurrencyGoroutines, func(int) {
		for range 10 {
			idx := rand.IntN(layerCount)
			// a few random queries per touch, checked against the same calls on the eager shadow
			var qs []file.Path
			for range 4 {
				qs = append(qs, queries[rand.IntN(len(queries))])
			}
			assert.Equalf(t, observeSquash(shadow, idx, qs), observeSquash(img.Layers, idx, qs), "layer %d %v", idx, qs)
		}
	})

	for idx, l := range img.Layers {
		w := shadow[idx].SquashedTree
		assert.Equalf(t, refsOf(w), refsOf(l.SquashedTree), "layer %d references", idx)
		assert.Equalf(t, sortedPaths(w.AllRealPaths()), sortedPaths(l.SquashedTree.AllRealPaths()), "layer %d paths", idx)
		assert.Equalf(t, squashResult{Content: fmt.Sprintf("a%d", idx)}, openResult(l.OpenPathFromSquash("/shared-a.txt")), "layer %d", idx)
	}
	assertLowerLayersBuiltOnce(t, counter, layerCount)
}
