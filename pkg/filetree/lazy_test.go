package filetree

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
)

const (
	lazyRacers  = 64
	lazyTimeout = 30 * time.Second
)

var errLazyBuild = errors.New("lazy build failed")

// newLazyFixture returns a small tree with regular files, a dir symlink, a basename symlink, a hardlink and a
// dead link, plus an index over all of it.
func newLazyFixture(t *testing.T) (*FileTree, Index) {
	t.Helper()
	ft := New()
	idx := NewIndex()

	add := func(ref *file.Reference, err error, m file.Metadata) {
		t.Helper()
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, m)
	}

	ref, err := ft.AddFile("/etc/os-release")
	add(ref, err, file.Metadata{Type: file.TypeRegular, MIMEType: "text/plain"})
	ref, err = ft.AddFile("/usr/lib/libc.so.6")
	add(ref, err, file.Metadata{Type: file.TypeRegular, MIMEType: "application/x-sharedlib"})
	ref, err = ft.AddSymLink("/usr/lib/libc.so", "/usr/lib/libc.so.6")
	add(ref, err, file.Metadata{Type: file.TypeSymLink})
	ref, err = ft.AddSymLink("/lib", "/usr/lib")
	add(ref, err, file.Metadata{Type: file.TypeSymLink})
	ref, err = ft.AddFile("/bin/sh")
	add(ref, err, file.Metadata{Type: file.TypeRegular, MIMEType: "application/x-executable"})
	ref, err = ft.AddHardLink("/bin/bash", "/bin/sh")
	add(ref, err, file.Metadata{Type: file.TypeHardLink})
	ref, err = ft.AddSymLink("/dead", "/nowhere")
	add(ref, err, file.Metadata{Type: file.TypeSymLink})
	ref, err = ft.AddDir("/var/log")
	add(ref, err, file.Metadata{Type: file.TypeDirectory})

	return ft, idx
}

// countingIndex counts link resolution index builds: GetByFileType is only called from
// searchContext.buildLinkResolutionIndex.
type countingIndex struct {
	IndexReader
	linkIndexBuilds atomic.Int32
}

func (c *countingIndex) GetByFileType(fTypes ...file.Type) ([]IndexEntry, error) {
	c.linkIndexBuilds.Add(1)
	return c.IndexReader.GetByFileType(fTypes...)
}

// notAFileTree is a valid ReadWriter that NewSearchContext cannot use (it asserts *FileTree).
type notAFileTree struct {
	ReadWriter
}

func countedLazy(build func() (ReadWriter, error)) (Reader, *atomic.Int32) {
	var n atomic.Int32
	return NewLazyReader(func() (ReadWriter, error) {
		n.Add(1)
		return build()
	}), &n
}

func lazyOverCopy(t *testing.T, ft *FileTree) (Reader, *atomic.Int32) {
	t.Helper()
	return countedLazy(func() (ReadWriter, error) { return ft.Copy() })
}

var (
	lazyQueryPaths  = []string{"/etc/os-release", "/usr/lib/libc.so", "/lib/libc.so", "/lib/libc.so.6", "/bin/bash", "/dead", "/missing", "/"}
	lazyQueryGlobs  = []string{"**/*", "**/libc*", "/lib/*", "/bin/*"}
	lazyQueryDirs   = []string{"/", "/usr/lib", "/lib", "/missing", "/etc/os-release"}
	lazyQueryMIMEs  = []string{"text/plain", "application/x-sharedlib", "application/x-executable"}
	lazyFollowLinks = [][]LinkResolutionOption{nil, {FollowBasenameLinks}, {DoNotFollowDeadBasenameLinks}}
)

type readerCall struct {
	name   string
	canErr bool
	call   func(r Reader) any
}

func sortedPaths(ps []file.Path) []file.Path {
	out := slices.Clone(ps)
	slices.Sort(out)
	return out
}

func sortedRefs(refs []file.Reference) []file.Reference {
	out := slices.Clone(refs)
	slices.SortFunc(out, func(a, b file.Reference) int {
		return cmp.Or(cmp.Compare(a.RealPath, b.RealPath), cmp.Compare(a.ID(), b.ID()))
	})
	return out
}

// readerCalls exercises every Reader method and returns comparable, order-normalized results.
func readerCalls() []readerCall {
	return []readerCall{
		{name: "File", canErr: true, call: func(r Reader) any {
			var out []any
			for _, p := range lazyQueryPaths {
				for _, opts := range lazyFollowLinks {
					exists, res, err := r.File(file.Path(p), opts...)
					out = append(out, exists, res, err)
				}
			}
			return out
		}},
		{name: "FilesByGlob", canErr: true, call: func(r Reader) any {
			var out []any
			for _, g := range lazyQueryGlobs {
				res, err := r.FilesByGlob(g)
				out = append(out, res, err)
			}
			return out
		}},
		{name: "ListPaths", canErr: true, call: func(r Reader) any {
			var out []any
			for _, d := range lazyQueryDirs {
				ps, err := r.ListPaths(file.Path(d))
				out = append(out, sortedPaths(ps), err)
			}
			return out
		}},
		{name: "Walk", canErr: true, call: func(r Reader) any {
			var visited []file.Path
			err := r.Walk(func(p file.Path, _ filenode.FileNode) error {
				visited = append(visited, p)
				return nil
			}, nil)
			return []any{sortedPaths(visited), err}
		}},
		{name: "Copy", canErr: true, call: func(r Reader) any {
			c, err := r.Copy()
			if err != nil {
				return []any{nil, err}
			}
			return []any{sortedPaths(c.AllRealPaths()), err}
		}},
		{name: "AllFiles", call: func(r Reader) any {
			return []any{
				sortedRefs(r.AllFiles()),
				sortedRefs(r.AllFiles(file.TypeSymLink, file.TypeHardLink, file.TypeDirectory)),
			}
		}},
		{name: "AllRealPaths", call: func(r Reader) any {
			return sortedPaths(r.AllRealPaths())
		}},
		{name: "HasPath", call: func(r Reader) any {
			var out []bool
			for _, p := range lazyQueryPaths {
				for _, opts := range lazyFollowLinks {
					out = append(out, r.HasPath(file.Path(p), opts...))
				}
			}
			return out
		}},
		{name: "TreeReader", call: func(r Reader) any {
			var ids []string
			for _, n := range r.TreeReader().Nodes() {
				ids = append(ids, string(n.ID()))
			}
			slices.Sort(ids)
			return ids
		}},
	}
}

// readerErrCalls calls each error-returning Reader method and returns its error.
var readerErrCalls = map[string]func(r Reader) error{
	"File": func(r Reader) error {
		_, _, err := r.File("/etc/os-release")
		return err
	},
	"FilesByGlob": func(r Reader) error {
		_, err := r.FilesByGlob("**/*")
		return err
	},
	"ListPaths": func(r Reader) error {
		_, err := r.ListPaths("/")
		return err
	},
	"Walk": func(r Reader) error {
		return r.Walk(func(file.Path, filenode.FileNode) error { return nil }, nil)
	},
	"Copy": func(r Reader) error {
		_, err := r.Copy()
		return err
	},
}

type searcherCall struct {
	name string
	call func(s Searcher) any
}

func searcherCalls() []searcherCall {
	return []searcherCall{
		{name: "SearchByPath", call: func(s Searcher) any {
			var out []any
			for _, p := range lazyQueryPaths {
				res, err := s.SearchByPath(p)
				out = append(out, res, err)
			}
			return out
		}},
		{name: "SearchByGlob", call: func(s Searcher) any {
			var out []any
			for _, g := range []string{"**/libc.so*", "**/os-release", "/lib/*", "**/bash", "**/*.so.6"} {
				res, err := s.SearchByGlob(g)
				res = slices.Clone(res)
				slices.SortFunc(res, func(a, b file.Resolution) int {
					return cmp.Or(cmp.Compare(a.RequestPath, b.RequestPath), cmp.Compare(a.RealPath, b.RealPath))
				})
				out = append(out, res, err)
			}
			return out
		}},
		{name: "SearchByMIMEType", call: func(s Searcher) any {
			res, err := s.SearchByMIMEType(lazyQueryMIMEs...)
			res = slices.Clone(res)
			slices.SortFunc(res, func(a, b file.Resolution) int {
				return cmp.Or(cmp.Compare(a.RequestPath, b.RequestPath), cmp.Compare(a.RealPath, b.RealPath))
			})
			return []any{res, err}
		}},
	}
}

var searcherErrCalls = map[string]func(s Searcher) error{
	"SearchByPath": func(s Searcher) error {
		_, err := s.SearchByPath("/etc/os-release")
		return err
	},
	"SearchByGlob": func(s Searcher) error {
		_, err := s.SearchByGlob("**/*")
		return err
	},
	"SearchByMIMEType": func(s Searcher) error {
		_, err := s.SearchByMIMEType("text/plain")
		return err
	},
}

// waitOrFail fails the test instead of hanging when the goroutines deadlock.
func waitOrFail(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("goroutines did not finish within %s (deadlock?)", d)
	}
}

// raceN runs fn on n goroutines held at a start barrier, so the first touches really overlap.
func raceN(t *testing.T, n int, fn func(i int)) {
	t.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			fn(i)
		})
	}
	close(start)
	waitOrFail(t, &wg, lazyTimeout)
}

func TestLazyReader_delegatesToBuiltTree(t *testing.T) {
	eager, _ := newLazyFixture(t)
	built, err := eager.Copy()
	require.NoError(t, err)
	lazy, builds := countedLazy(func() (ReadWriter, error) { return built, nil })

	assert.Zero(t, builds.Load(), "build ran before first use")

	for _, c := range readerCalls() {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.call(eager), c.call(lazy))
		})
	}

	// the lazy reader delegates to exactly the tree build returned
	assert.Same(t, built.TreeReader(), lazy.TreeReader())
	assert.EqualValues(t, 1, builds.Load())
}

func TestLazyReader_buildFailures(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	empty := New()

	tests := []struct {
		name    string
		build   func() (ReadWriter, error)
		wantErr func(t *testing.T, err error)
	}{
		{
			name:  "error",
			build: func() (ReadWriter, error) { return nil, errLazyBuild },
			wantErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, errLazyBuild)
			},
		},
		{
			name:  "nil tree and nil error",
			build: func() (ReadWriter, error) { return nil, nil },
			wantErr: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "returned no tree")
			},
		},
		{
			name:  "typed nil tree and nil error",
			build: func() (ReadWriter, error) { return (*FileTree)(nil), nil },
			wantErr: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "returned no tree")
			},
		},
		{
			name:  "tree and error discards the tree",
			build: func() (ReadWriter, error) { return fixture, errLazyBuild },
			wantErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, errLazyBuild)
			},
		},
		{
			name:  "panic",
			build: func() (ReadWriter, error) { panic("boom") },
			wantErr: func(t *testing.T, err error) {
				assert.ErrorContains(t, err, "panicked")
				assert.ErrorContains(t, err, "boom")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lazy, builds := countedLazy(tt.build)

			// two rounds: the failure must be sticky and build must not run again
			var first error
			for round := range 2 {
				for name, call := range readerErrCalls {
					var err error
					require.NotPanics(t, func() { err = call(lazy) }, "%s round %d", name, round)
					require.Error(t, err, name)
					tt.wantErr(t, err)
					if first == nil {
						first = err
					}
					assert.Same(t, first, err, "%s round %d returned a different error", name, round)
				}
				for _, c := range readerCalls() {
					if c.canErr {
						continue
					}
					assert.Equal(t, c.call(empty), c.call(lazy), "%s should see an empty tree", c.name)
				}
			}
			assert.EqualValues(t, 1, builds.Load())
		})
	}
}

func TestLazyReader_buildRunsOnceAfterSuccess(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	for range 3 {
		for _, c := range readerCalls() {
			c.call(lazy)
		}
	}
	assert.EqualValues(t, 1, builds.Load())
}

func TestLazySearcher_deferredUntilFirstSearch(t *testing.T) {
	fixture, idx := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	cidx := &countingIndex{IndexReader: idx}

	s := NewSearchContext(lazy, cidx)
	require.IsType(t, &lazySearcher{}, s)
	assert.Zero(t, builds.Load(), "tree built by NewSearchContext")
	assert.Zero(t, cidx.linkIndexBuilds.Load(), "link index built by NewSearchContext")

	eager := NewSearchContext(fixture, idx)
	for _, c := range searcherCalls() {
		assert.Equal(t, c.call(eager), c.call(s), c.name)
	}
	assert.EqualValues(t, 1, builds.Load())
	assert.EqualValues(t, 1, cidx.linkIndexBuilds.Load())
}

func TestLazySearcher_failedTree(t *testing.T) {
	_, idx := newLazyFixture(t)
	lazy, builds := countedLazy(func() (ReadWriter, error) { return nil, errLazyBuild })
	s := NewSearchContext(lazy, idx)

	for range 2 {
		for name, call := range searcherErrCalls {
			assert.ErrorIs(t, call(s), errLazyBuild, name)
		}
	}
	assert.EqualValues(t, 1, builds.Load())
}

func TestLazySearcher_nonFileTreeBuild(t *testing.T) {
	_, idx := newLazyFixture(t)
	lazy, builds := countedLazy(func() (ReadWriter, error) { return notAFileTree{ReadWriter: New()}, nil })

	var s Searcher
	require.NotPanics(t, func() { s = NewSearchContext(lazy, idx) })
	for range 2 {
		for name, call := range searcherErrCalls {
			var err error
			require.NotPanics(t, func() { err = call(s) }, name)
			assert.ErrorContains(t, err, "panicked", name)
		}
	}
	// the tree itself is fine, only the search context could not be built
	assert.Equal(t, []file.Path{"/"}, lazy.AllRealPaths())
	assert.EqualValues(t, 1, builds.Load())
}

// concurrency suite. Every test is meant to run under -race.

func TestLazyReader_concurrent_sameMethod(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)

	type result struct {
		exists bool
		res    *file.Resolution
		err    error
		tree   any
	}
	results := make([]result, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		exists, res, err := lazy.File("/lib/libc.so", FollowBasenameLinks)
		results[i] = result{exists: exists, res: res, err: err, tree: lazy.TreeReader()}
	})

	wantExists, wantRes, wantErr := fixture.File("/lib/libc.so", FollowBasenameLinks)
	require.NoError(t, wantErr)
	require.True(t, wantExists)
	require.Equal(t, file.Path("/usr/lib/libc.so.6"), wantRes.RealPath)
	assert.EqualValues(t, 1, builds.Load())
	for i, r := range results {
		assert.NoError(t, r.err, i)
		assert.Equal(t, wantExists, r.exists, i)
		assert.Equal(t, wantRes, r.res, i)
		// every caller sees the same realized tree
		assert.Same(t, results[0].tree, r.tree, i)
	}
}

func TestLazyReader_concurrent_everyMethod(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	calls := readerCalls()

	got := make([]any, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		got[i] = calls[i%len(calls)].call(lazy)
	})

	assert.EqualValues(t, 1, builds.Load())
	for i, g := range got {
		c := calls[i%len(calls)]
		assert.Equal(t, c.call(fixture), g, "%s (goroutine %d)", c.name, i)
	}
}

func TestLazyReader_concurrent_slowBuildHoldsEveryone(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	lazy, builds := countedLazy(func() (ReadWriter, error) {
		close(started)
		<-release
		return fixture.Copy()
	})
	calls := readerCalls()

	var returned atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range lazyRacers {
		wg.Go(func() {
			<-start
			calls[i%len(calls)].call(lazy)
			returned.Add(1)
		})
	}
	close(start)

	select {
	case <-started:
	case <-time.After(lazyTimeout):
		t.Fatal("build never started")
	}
	// give the waiters a chance to (wrongly) return
	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, returned.Load(), "callers returned before the build finished")

	close(release)
	waitOrFail(t, &wg, 5*time.Second)
	assert.EqualValues(t, lazyRacers, returned.Load())
	assert.EqualValues(t, 1, builds.Load())
}

func testLazyReaderConcurrentFailure(t *testing.T, build func() (ReadWriter, error), check func(t *testing.T, err error)) {
	t.Helper()
	lazy, builds := countedLazy(build)
	empty := New()
	calls := readerCalls()
	errNames := slices.Sorted(maps.Keys(readerErrCalls))
	var plain []readerCall
	for _, c := range calls {
		if !c.canErr {
			plain = append(plain, c)
		}
	}

	type result struct {
		panicked any
		err      error
		plain    any
	}
	wave := func() []result {
		results := make([]result, lazyRacers)
		raceN(t, lazyRacers, func(i int) {
			defer func() { results[i].panicked = recover() }()
			if i%2 == 0 {
				results[i].err = readerErrCalls[errNames[i/2%len(errNames)]](lazy)
				return
			}
			// odd goroutines hit the methods without an error return
			results[i].plain = plain[i/2%len(plain)].call(lazy)
		})
		return results
	}

	var first error
	for round := range 2 {
		for i, r := range wave() {
			require.Nil(t, r.panicked, "goroutine %d panicked in round %d", i, round)
			if i%2 != 0 {
				continue
			}
			check(t, r.err)
			if first == nil {
				first = r.err
			}
			assert.Same(t, first, r.err, "goroutine %d round %d got a different error", i, round)
		}
	}
	for _, c := range plain {
		assert.Equal(t, c.call(empty), c.call(lazy), c.name)
	}
	assert.EqualValues(t, 1, builds.Load())
}

func TestLazyReader_concurrent_buildError(t *testing.T) {
	testLazyReaderConcurrentFailure(t,
		func() (ReadWriter, error) { return nil, errLazyBuild },
		func(t *testing.T, err error) { assert.ErrorIs(t, err, errLazyBuild) },
	)
}

func TestLazyReader_concurrent_buildPanic(t *testing.T) {
	testLazyReaderConcurrentFailure(t,
		func() (ReadWriter, error) { panic("boom") },
		func(t *testing.T, err error) { assert.ErrorContains(t, err, "panicked: boom") },
	)
}

func TestLazySearcher_concurrent_racesTree(t *testing.T) {
	fixture, idx := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	cidx := &countingIndex{IndexReader: idx}
	s := NewSearchContext(lazy, cidx)
	eagerSearch := NewSearchContext(fixture, idx)
	rCalls, sCalls := readerCalls(), searcherCalls()

	got := make([]any, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		if i%2 == 0 {
			got[i] = sCalls[i/2%len(sCalls)].call(s)
			return
		}
		got[i] = rCalls[i/2%len(rCalls)].call(lazy)
	})

	assert.EqualValues(t, 1, builds.Load())
	assert.EqualValues(t, 1, cidx.linkIndexBuilds.Load())
	for i, g := range got {
		if i%2 == 0 {
			c := sCalls[i/2%len(sCalls)]
			assert.Equal(t, c.call(eagerSearch), g, "%s (goroutine %d)", c.name, i)
			continue
		}
		c := rCalls[i/2%len(rCalls)]
		assert.Equal(t, c.call(fixture), g, "%s (goroutine %d)", c.name, i)
	}
}

func TestLazySearcher_concurrent_manySearchers(t *testing.T) {
	fixture, idx := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	cidx := &countingIndex{IndexReader: idx}
	calls := searcherCalls()

	got := make([][]any, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		s := NewSearchContext(lazy, cidx)
		for _, c := range calls {
			got[i] = append(got[i], c.call(s))
		}
	})

	eagerSearch := NewSearchContext(fixture, idx)
	var want []any
	for _, c := range calls {
		want = append(want, c.call(eagerSearch))
	}
	mimeHits, err := eagerSearch.SearchByMIMEType(lazyQueryMIMEs...)
	require.NoError(t, err)
	require.Len(t, mimeHits, 3, "fixture should give the searches something to find")
	assert.EqualValues(t, 1, builds.Load())
	// each searcher builds its own link index, once
	assert.EqualValues(t, lazyRacers, cidx.linkIndexBuilds.Load())
	for i, g := range got {
		assert.Equal(t, want, g, "goroutine %d", i)
	}
}

const lazyChainDepth = 100

// newChainLayers returns one small layer tree per layer: its own file, an override of a shared file, a
// symlink now and then, and a whiteout of a lower layer's file now and then.
func newChainLayers(t *testing.T) []*FileTree {
	t.Helper()
	layers := make([]*FileTree, lazyChainDepth)
	for k := range layers {
		lt := New()
		_, err := lt.AddFile(file.Path(fmt.Sprintf("/layer-%d/file", k)))
		require.NoError(t, err)
		_, err = lt.AddFile("/shared/file")
		require.NoError(t, err)
		if k%7 == 3 {
			_, err = lt.AddSymLink(file.Path(fmt.Sprintf("/link-%d", k)), file.Path(fmt.Sprintf("/layer-%d/file", k)))
			require.NoError(t, err)
		}
		if k%10 == 5 {
			_, err = lt.AddFile(file.Path(fmt.Sprintf("/layer-%d/.wh.file", k-2)))
			require.NoError(t, err)
		}
		layers[k] = lt
	}
	return layers
}

// eagerChain squashes layers 0..k with UnionFileTree for every k.
func eagerChain(t *testing.T, layers []*FileTree) []ReadWriter {
	t.Helper()
	out := make([]ReadWriter, len(layers))
	for k := range layers {
		u := NewUnionFileTree()
		for _, lt := range layers[:k+1] {
			u.PushTree(lt)
		}
		sq, err := u.Squash()
		require.NoError(t, err)
		out[k] = sq
	}
	return out
}

type chainFailure int

const (
	chainOK chainFailure = iota
	chainError
	chainPanic
)

const chainFailAt = 40

// lazyChain builds each layer by copying the realized layer below and merging its own layer tree on top.
func lazyChain(layers []*FileTree, failure chainFailure) ([]Reader, []atomic.Int32) {
	chain := make([]Reader, len(layers))
	builds := make([]atomic.Int32, len(layers))
	for k := range layers {
		var below Reader
		if k > 0 {
			below = chain[k-1]
		}
		chain[k] = NewLazyReader(func() (ReadWriter, error) {
			builds[k].Add(1)
			if k == chainFailAt {
				switch failure {
				case chainError:
					return nil, fmt.Errorf("layer %d: %w", k, errLazyBuild)
				case chainPanic:
					panic(fmt.Sprintf("layer %d exploded", k))
				}
			}
			if below == nil {
				return layers[k].Copy()
			}
			rw, err := below.Copy()
			if err != nil {
				return nil, fmt.Errorf("layer %d: %w", k, err)
			}
			if err := rw.Merge(layers[k]); err != nil {
				return nil, err
			}
			return rw, nil
		})
	}
	return chain, builds
}

// chainView is a cheap per-layer result that differs between every pair of layers.
func chainView(r Reader, k int) any {
	exists, res, err := r.File("/shared/file")
	return []any{exists, res, err, r.HasPath(file.Path(fmt.Sprintf("/layer-%d/file", k)))}
}

// touchChain has goroutines walk the chain top-down, bottom-up and in shuffled order, recording each view.
func touchChain(t *testing.T, chain []Reader) [][]any {
	t.Helper()
	views := make([][]any, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		order := make([]int, len(chain))
		for k := range order {
			order[k] = k
		}
		switch i % 3 {
		case 0:
			slices.Reverse(order)
		case 1:
			// bottom-up
		case 2:
			rng := rand.New(rand.NewPCG(uint64(i), 0))
			rng.Shuffle(len(order), func(a, b int) { order[a], order[b] = order[b], order[a] })
		}
		views[i] = make([]any, len(chain))
		for _, k := range order {
			views[i][k] = chainView(chain[k], k)
		}
	})
	return views
}

func TestLazyReader_concurrent_deepChain(t *testing.T) {
	layers := newChainLayers(t)
	eager := eagerChain(t, layers)
	chain, builds := lazyChain(layers, chainOK)

	views := touchChain(t, chain)

	for k := range chain {
		assert.EqualValues(t, 1, builds[k].Load(), "layer %d", k)
		assert.Equal(t, sortedPaths(eager[k].AllRealPaths()), sortedPaths(chain[k].AllRealPaths()), "layer %d", k)
		want := chainView(eager[k], k)
		for i := range views {
			assert.Equal(t, want, views[i][k], "layer %d goroutine %d", k, i)
		}
	}
}

func testLazyChainFailure(t *testing.T, failure chainFailure) {
	t.Helper()
	layers := newChainLayers(t)
	eager := eagerChain(t, layers)
	chain, builds := lazyChain(layers, failure)

	views := touchChain(t, chain)

	_, rootErr := chain[chainFailAt].Copy()
	require.Error(t, rootErr)
	if failure == chainError {
		require.ErrorIs(t, rootErr, errLazyBuild)
	} else {
		require.ErrorContains(t, rootErr, "panicked")
	}

	for k := range chain {
		assert.EqualValues(t, 1, builds[k].Load(), "layer %d", k)
		if k < chainFailAt {
			assert.Equal(t, sortedPaths(eager[k].AllRealPaths()), sortedPaths(chain[k].AllRealPaths()), "layer %d", k)
			want := chainView(eager[k], k)
			for i := range views {
				assert.Equal(t, want, views[i][k], "layer %d goroutine %d", k, i)
			}
			continue
		}
		for name, call := range readerErrCalls {
			assert.ErrorIs(t, call(chain[k]), rootErr, "layer %d %s", k, name)
		}
		for i := range views {
			err, _ := views[i][k].([]any)[2].(error)
			assert.ErrorIs(t, err, rootErr, "layer %d goroutine %d", k, i)
		}
	}
}

func TestLazyReader_concurrent_chainErrorPartway(t *testing.T) {
	testLazyChainFailure(t, chainError)
}

func TestLazyReader_concurrent_chainPanicPartway(t *testing.T) {
	testLazyChainFailure(t, chainPanic)
}

func TestLazyReader_concurrent_copyIsIndependent(t *testing.T) {
	fixture, _ := newLazyFixture(t)
	lazy, builds := lazyOverCopy(t, fixture)
	want := sortedPaths(fixture.AllRealPaths())

	copies := make([]ReadWriter, lazyRacers)
	errs := make([]error, lazyRacers)
	reads := make([][]file.Path, lazyRacers)
	raceN(t, lazyRacers, func(i int) {
		c, err := lazy.Copy()
		if err != nil {
			errs[i] = err
			return
		}
		copies[i] = c
		if _, err := c.AddFile(file.Path(fmt.Sprintf("/copy-%d/file", i))); err != nil {
			errs[i] = err
			return
		}
		if err := c.RemovePath("/etc/os-release"); err != nil {
			errs[i] = err
			return
		}
		// read the original while other goroutines are writing their copies
		reads[i] = sortedPaths(lazy.AllRealPaths())
	})

	assert.EqualValues(t, 1, builds.Load())
	assert.Equal(t, want, sortedPaths(lazy.AllRealPaths()), "lazy tree changed")
	for i := range lazyRacers {
		require.NoError(t, errs[i], i)
		assert.Equal(t, want, reads[i], "goroutine %d saw a changed original", i)

		c := copies[i]
		assert.True(t, c.HasPath(file.Path(fmt.Sprintf("/copy-%d/file", i))), i)
		assert.False(t, c.HasPath("/etc/os-release"), i)
		for j := range lazyRacers {
			if j != i {
				assert.False(t, c.HasPath(file.Path(fmt.Sprintf("/copy-%d", j))), "copy %d sees copy %d's write", i, j)
			}
		}
	}
}

// warnCounter counts the warnings logged through internal/log (copied from the capturing logger in
// internal/podman/ssh_knownhosts_test.go). The embedded discard logger handles every other level.
type warnCounter struct {
	logger.Logger
	warnings atomic.Int32
}

func (w *warnCounter) WithFields(...any) logger.MessageLogger { return w }
func (w *warnCounter) Warn(...any)                            { w.warnings.Add(1) }
func (w *warnCounter) Warnf(string, ...any)                   { w.warnings.Add(1) }

func countWarnings(t *testing.T) *warnCounter {
	t.Helper()
	counter := &warnCounter{Logger: discard.New()}
	prev := log.Log
	log.Log = counter
	t.Cleanup(func() { log.Log = prev })
	return counter
}

func TestNewLazyReader_aWrappedFailureIsLoggedAgain(t *testing.T) {
	// a build that adds its own context to a lower reader's failure says something new, so it gets its
	// own warning, and the original is still reachable with errors.Is
	counter := countWarnings(t)

	lower := NewLazyReader(func() (ReadWriter, error) { return nil, errLazyBuild })
	upper := NewLazyReader(func() (ReadWriter, error) {
		_, err := lower.Copy()
		return nil, fmt.Errorf("building upper: %w", err)
	})

	_, _, err := upper.File("/a")
	require.ErrorIs(t, err, errLazyBuild)
	assert.ErrorContains(t, err, "building upper: ")
	assert.Equal(t, int32(2), counter.warnings.Load(), "warnings for the lower failure and the upper one wrapping it")
}

func TestLazySearcher_ownPanicIsLogged(t *testing.T) {
	// a search context that can't be built over a tree that could is a failure nothing else logs, so it
	// must keep its own warning whatever is done to stop repeats of a failed build's
	counter := countWarnings(t)
	_, idx := newLazyFixture(t)
	lazy := NewLazyReader(func() (ReadWriter, error) { return notAFileTree{ReadWriter: New()}, nil })

	s := NewSearchContext(lazy, idx)
	_, err := s.SearchByPath("/a")
	require.ErrorContains(t, err, "panicked")
	_, err = s.SearchByGlob("**/*")
	require.ErrorContains(t, err, "panicked")

	assert.Equal(t, int32(1), counter.warnings.Load(), "warnings for one search context that failed to build")
}
