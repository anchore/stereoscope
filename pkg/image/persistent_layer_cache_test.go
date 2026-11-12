package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// countedLayer counts how often the layer is decompressed, which is what the cache exists to avoid.
type countedLayer struct {
	v1.Layer
	calls *atomic.Int64
}

func (c countedLayer) Uncompressed() (io.ReadCloser, error) {
	c.calls.Add(1)
	return c.Layer.Uncompressed()
}

// lyingLayer claims a diff ID its content does not have.
type lyingLayer struct{ v1.Layer }

func (lyingLayer) DiffID() (v1.Hash, error) {
	return v1.NewHash("sha256:" + strings.Repeat("ab", 32))
}

func newCounted(t *testing.T, size int64) (v1.Layer, *atomic.Int64) {
	t.Helper()
	raw, err := random.Layer(size, v1Types.DockerLayer)
	require.NoError(t, err)
	calls := &atomic.Int64{}
	return countedLayer{Layer: raw, calls: calls}, calls
}

func testLayerCache(t *testing.T, maxBytes int64) *persistentLayerCache {
	t.Helper()
	return newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir(), MaxBytes: maxBytes})
}

// fetchWith fetches a fresh Layer over raw through the cache and indexes it, the way Image.Read does.
func fetchWith(t *testing.T, raw v1.Layer, pc *persistentLayerCache) *Layer {
	t.Helper()
	l := NewLayer(raw)
	t.Cleanup(func() { _ = l.close() })
	require.NoError(t, l.fetch(0, t.TempDir(), pc))
	require.NoError(t, l.index(NewFileCatalog()))
	return l
}

func cacheFiles(t *testing.T, pc *persistentLayerCache) []string {
	t.Helper()
	entries, err := os.ReadDir(pc.dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func assertNoPartials(t *testing.T, pc *persistentLayerCache) {
	t.Helper()
	for _, name := range cacheFiles(t, pc) {
		assert.Falsef(t, strings.HasPrefix(name, layerCachePartialPrefix), "left an in-flight file behind: %s", name)
	}
}

func keyOf(t *testing.T, raw v1.Layer) string {
	t.Helper()
	diffID, err := raw.DiffID()
	require.NoError(t, err)
	key, ok := layerCacheKey(diffID.String())
	require.True(t, ok)
	return key
}

func TestLayerCacheFromContext(t *testing.T) {
	_, ok := LayerCacheFromContext(context.Background())
	assert.False(t, ok)

	ctx := WithLayerCache(context.Background(), LayerCacheConfig{})
	_, ok = LayerCacheFromContext(ctx)
	assert.False(t, ok, "an empty dir is off")
	assert.Nil(t, newPersistentLayerCache(ctx))

	ctx = WithLayerCache(context.Background(), LayerCacheConfig{Dir: "/x", MaxBytes: 5})
	cfg, ok := LayerCacheFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, LayerCacheConfig{Dir: "/x", MaxBytes: 5}, cfg)
}

func TestPersistentLayerCache_defaultCap(t *testing.T) {
	for _, maxBytes := range []int64{0, -1} {
		pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir(), MaxBytes: maxBytes})
		assert.Equal(t, DefaultLayerCacheMaxBytes, pc.maxBytes)
	}
	assert.Equal(t, int64(20<<30), DefaultLayerCacheMaxBytes)
}

func TestLayerCacheKey(t *testing.T) {
	hexPart := strings.Repeat("0f", 32)
	key, ok := layerCacheKey("sha256:" + hexPart)
	require.True(t, ok)
	assert.Equal(t, "sha256-"+hexPart, key)

	for _, bad := range []string{"", "sha256:", "sha256:abc", "sha512:" + hexPart, "sha256:" + strings.ToUpper(hexPart), "sha256:" + strings.Repeat("zz", 32), "sha256:../" + hexPart[3:]} {
		_, ok := layerCacheKey(bad)
		assert.Falsef(t, ok, "%q", bad)
	}
}

func TestPersistentLayerCache_missThenHit(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, calls := newCounted(t, 256)

	first := fetchWith(t, raw, pc)
	assert.Equal(t, int64(1), calls.Load())
	assert.True(t, first.contentPersistent)
	key := keyOf(t, raw)
	assert.Equal(t, pc.tarPath(key), first.contentPath)

	info, err := os.Stat(first.contentPath)
	require.NoError(t, err)
	if os.PathSeparator == '/' {
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm(), "entries are read-only")
		dirInfo, err := os.Stat(pc.dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	}

	var sc layerCacheSidecar
	raw2, err := os.ReadFile(pc.sidecarPath(key))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw2, &sc))
	diffID, _ := raw.DiffID()
	assert.Equal(t, diffID.String(), sc.DiffID)
	assert.Equal(t, info.Size(), sc.Size)
	edge, _, err := layerCacheEdgeDigest(first.contentPath)
	require.NoError(t, err)
	assert.Equal(t, edge, sc.Edge)

	second := fetchWith(t, raw, pc)
	assert.Equal(t, int64(1), calls.Load(), "a hit must not decompress the layer again")
	assert.Equal(t, first.contentPath, second.contentPath)
	assert.Equal(t, fileTuples(t, first.Tree, first.fileCatalog), fileTuples(t, second.Tree, second.fileCatalog))
	assert.Equal(t, int64(1), pc.hits.Load())
	assert.Equal(t, int64(1), pc.populated.Load())
	assertNoPartials(t, pc)
}

func TestPersistentLayerCache_diffIDMismatchIsNeverCached(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)

	l := fetchWith(t, lyingLayer{raw}, pc)
	assert.False(t, l.contentPersistent, "the layer is read the per-run way instead")
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
	assert.Empty(t, cacheFiles(t, pc), "nothing is kept under a name its content does not match")
	assert.Equal(t, int64(1), pc.bypassed.Load())
}

func TestPersistentLayerCache_damagedEntriesAreReplaced(t *testing.T) {
	cases := map[string]func(t *testing.T, tarPath, sidecarPath string){
		"truncated": func(t *testing.T, tarPath, _ string) {
			require.NoError(t, os.Truncate(tarPath, 1000))
		},
		"head zeroed": func(t *testing.T, tarPath, _ string) {
			overwrite(t, tarPath, 0, make([]byte, 4096))
		},
		"tail zeroed": func(t *testing.T, tarPath, _ string) {
			info, err := os.Stat(tarPath)
			require.NoError(t, err)
			overwrite(t, tarPath, info.Size()-4096, make([]byte, 4096))
		},
		"sidecar unreadable": func(t *testing.T, _, sidecarPath string) {
			require.NoError(t, os.WriteFile(sidecarPath, []byte("{not json"), 0o644))
		},
		"sidecar names another layer": func(t *testing.T, _, sidecarPath string) {
			raw, err := os.ReadFile(sidecarPath)
			require.NoError(t, err)
			var sc layerCacheSidecar
			require.NoError(t, json.Unmarshal(raw, &sc))
			sc.DiffID = "sha256:" + strings.Repeat("cd", 32)
			raw, err = json.Marshal(sc)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(sidecarPath, raw, 0o644))
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			pc := testLayerCache(t, 0)
			raw, calls := newCounted(t, 1<<20) // well over the two 64 KiB edges
			first := fetchWith(t, raw, pc)
			want := fileTuples(t, first.Tree, first.fileCatalog)
			require.NoError(t, first.close())

			key := keyOf(t, raw)
			damage(t, pc.tarPath(key), pc.sidecarPath(key))

			again := fetchWith(t, raw, pc)
			assert.Equal(t, int64(2), calls.Load(), "a damaged entry must be unpacked again")
			assert.Equal(t, want, fileTuples(t, again.Tree, again.fileCatalog))
			_, hit := pc.lookup(key, again.Metadata.Digest)
			assert.True(t, hit, "the rebuilt entry is good")
			assertNoPartials(t, pc)
		})
	}
}

// The hit check reads only the two ends of the tar. A same-size change in the middle is a known,
// accepted gap (impl plan R2): this test pins it so a stronger check is a deliberate change.
func TestPersistentLayerCache_interiorSameSizeCorruptionIsAKnownGap(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, calls := newCounted(t, 1<<20)
	first := fetchWith(t, raw, pc)
	require.NoError(t, first.close())

	key := keyOf(t, raw)
	info, err := os.Stat(pc.tarPath(key))
	require.NoError(t, err)
	overwrite(t, pc.tarPath(key), info.Size()/2, []byte{0xff})

	_, hit := pc.lookup(key, first.Metadata.Digest)
	assert.True(t, hit)
	assert.Equal(t, int64(1), calls.Load())
}

func overwrite(t *testing.T, path string, at int64, data []byte) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o644))
	fh, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = fh.WriteAt(data, at)
	require.NoError(t, err)
	require.NoError(t, fh.Close())
}

// droppedDockerLayer is a docker layer whose download drops partway through.
type droppedDockerLayer struct{ truncatedContentLayer }

func (droppedDockerLayer) MediaType() (v1Types.MediaType, error) { return v1Types.DockerLayer, nil }

func TestPersistentLayerCache_layerReadErrorFailsTheFetch(t *testing.T) {
	pc := testLayerCache(t, 0)
	l := newLayer(droppedDockerLayer{}, "sha256:"+strings.Repeat("ab", 32))
	err := l.fetch(0, t.TempDir(), pc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection dropped")
	assert.Empty(t, cacheFiles(t, pc), "a failed read leaves nothing behind")
}

func TestPersistentLayerCache_unusableDirFallsBack(t *testing.T) {
	// a regular file where the cache directory should be: fails for root and on Windows too
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o644))
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: blocker})

	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	l := fetchWith(t, raw, pc)
	assert.False(t, l.contentPersistent)
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
	assert.Equal(t, int64(1), pc.bypassed.Load())
}

func TestPersistentLayerCache_layerLargerThanTheCapIsNotCached(t *testing.T) {
	pc := testLayerCache(t, 1024)
	raw, err := random.Layer(4096, v1Types.DockerLayer)
	require.NoError(t, err)

	l := fetchWith(t, raw, pc)
	assert.False(t, l.contentPersistent)
	assert.Empty(t, cacheFiles(t, pc))
}

func TestPersistentLayerCache_ineligibleLayersAreNotCached(t *testing.T) {
	pc := testLayerCache(t, 0)

	t.Run("not sha256", func(t *testing.T) {
		raw, err := random.Layer(256, v1Types.DockerLayer)
		require.NoError(t, err)
		l := newLayer(raw, "sha512:"+strings.Repeat("ab", 64))
		t.Cleanup(func() { _ = l.close() })
		require.NoError(t, l.fetch(0, t.TempDir(), pc))
		assert.False(t, l.contentPersistent)
	})
	t.Run("squashfs", func(t *testing.T) {
		l := &Layer{}
		l.Metadata.MediaType = SingularitySquashFSLayer
		l.Metadata.Digest = "sha256:" + strings.Repeat("ab", 32)
		_, ok, err := pc.materialize(l)
		require.NoError(t, err)
		assert.False(t, ok)
	})
	_, err := os.Stat(pc.dir)
	assert.True(t, os.IsNotExist(err), "the cache directory is not even created")
}

func TestPersistentLayerCache_concurrentFetchesOfOneLayerDecompressOnce(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, calls := newCounted(t, 1<<16)

	var wg sync.WaitGroup
	paths := make([]string, 8)
	for idx := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := NewLayer(raw)
			if err := l.fetch(idx, t.TempDir(), pc); err != nil {
				t.Error(err)
				return
			}
			paths[idx] = l.contentPath
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), calls.Load())
	for _, p := range paths {
		assert.Equal(t, pc.tarPath(keyOf(t, raw)), p)
	}
	assertNoPartials(t, pc)
}

func TestPersistentLayerCache_concurrentProcessesShareOneEntry(t *testing.T) {
	// two handles on one directory stand in for two processes: no shared lock between them
	dir := t.TempDir()
	raw, _ := newCounted(t, 1<<18)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: dir})
			l := NewLayer(raw)
			defer l.close()
			if err := l.fetch(0, t.TempDir(), pc); err != nil {
				t.Error(err)
				return
			}
			if err := l.index(NewFileCatalog()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: dir})
	assertNoPartials(t, pc)
	assert.Len(t, cacheFiles(t, pc), 2, "one tar and one sidecar")
	diffID, _ := raw.DiffID()
	_, hit := pc.lookup(keyOf(t, raw), diffID.String())
	assert.True(t, hit)
}

func TestPersistentLayerCache_entryEvictedBeforeIndexIsFetchedAgain(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, calls := newCounted(t, 256)

	l := NewLayer(raw)
	t.Cleanup(func() { _ = l.close() })
	require.NoError(t, l.fetch(0, t.TempDir(), pc))
	key := keyOf(t, raw)
	// another scan's eviction, between this one's fetch and index
	require.NoError(t, os.Remove(pc.sidecarPath(key)))
	require.NoError(t, removeReadOnly(pc.tarPath(key)))

	require.NoError(t, l.index(NewFileCatalog()))
	assert.Equal(t, int64(2), calls.Load())
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
}

func TestLayer_missingPerRunContentIsStillAnError(t *testing.T) {
	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	l := NewLayer(raw)
	require.NoError(t, l.fetch(0, t.TempDir(), nil))
	require.NoError(t, os.Remove(l.contentPath))
	assert.Error(t, l.index(NewFileCatalog()))
}

// --- eviction ---

// seedEntry writes a well-formed entry of size bytes, last used at used.
func seedEntry(t *testing.T, pc *persistentLayerCache, name string, size int, used time.Time) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	sum := sha256.Sum256([]byte(name))
	key := "sha256-" + hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(pc.tarPath(key), make([]byte, size), 0o444))
	edge, gotSize, err := layerCacheEdgeDigest(pc.tarPath(key))
	require.NoError(t, err)
	require.NoError(t, pc.writeSidecar(key, layerCacheSidecar{DiffID: "sha256:" + hex.EncodeToString(sum[:]), Size: gotSize, Edge: edge}))
	require.NoError(t, os.Chtimes(pc.sidecarPath(key), used, used))
	require.NoError(t, os.Chtimes(pc.tarPath(key), used, used))
	return key
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestPersistentLayerCache_evictsLeastRecentlyUsedFirst(t *testing.T) {
	pc := testLayerCache(t, 30)
	now := time.Now()
	pc.now = func() time.Time { return now }

	var keys []string
	for idx := range 5 {
		// entry 0 is the oldest
		keys = append(keys, seedEntry(t, pc, fmt.Sprint(idx), 10, now.Add(-time.Duration(10-idx)*time.Hour)))
	}
	pc.evict()

	for idx, key := range keys {
		want := idx >= 2
		assert.Equalf(t, want, exists(pc.tarPath(key)), "entry %d tar", idx)
		assert.Equalf(t, want, exists(pc.sidecarPath(key)), "entry %d sidecar", idx)
	}
}

func TestPersistentLayerCache_aHitRefreshesAnEntry(t *testing.T) {
	pc := testLayerCache(t, 10)
	now := time.Now()
	pc.now = func() time.Time { return now }

	older := seedEntry(t, pc, "older", 10, now.Add(-3*time.Hour))
	newer := seedEntry(t, pc, "newer", 10, now.Add(-2*time.Hour))

	_, hit := pc.lookup(older, "sha256:"+strings.TrimPrefix(older, "sha256-"))
	require.True(t, hit)
	// out of the grace window again, but now the more recently used of the two
	pc.now = func() time.Time { return now.Add(time.Hour) }
	pc.evict()

	assert.True(t, exists(pc.tarPath(older)))
	assert.False(t, exists(pc.tarPath(newer)))
}

func TestPersistentLayerCache_recentlyUsedEntriesSurviveOverTheCap(t *testing.T) {
	pc := testLayerCache(t, 10)
	now := time.Now()
	pc.now = func() time.Time { return now }

	a := seedEntry(t, pc, "a", 10, now.Add(-time.Minute))
	b := seedEntry(t, pc, "b", 10, now.Add(-2*time.Minute))
	pc.evict()

	assert.True(t, exists(pc.tarPath(a)))
	assert.True(t, exists(pc.tarPath(b)), "inside the grace window: another scan may be about to open it")
}

func TestPersistentLayerCache_sweepsAbandonedFiles(t *testing.T) {
	pc := testLayerCache(t, 0)
	now := time.Now()
	pc.now = func() time.Time { return now }
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))

	write := func(name string, age time.Duration) string {
		p := filepath.Join(pc.dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o444))
		require.NoError(t, os.Chtimes(p, now.Add(-age), now.Add(-age)))
		return p
	}
	oldPartial := write(layerCachePartialPrefix+"old", 2*time.Hour)
	newPartial := write(layerCachePartialPrefix+"new", 5*time.Minute)
	oldOrphan := write("sha256-"+strings.Repeat("1", 64)+layerCacheTarSuffix, 2*time.Hour)
	newOrphan := write("sha256-"+strings.Repeat("2", 64)+layerCacheTarSuffix, 5*time.Minute)
	loneSidecar := write("sha256-"+strings.Repeat("3", 64)+layerCacheSidecarSuffix, 5*time.Minute)

	pc.evict()

	assert.False(t, exists(oldPartial))
	assert.True(t, exists(newPartial), "may belong to a scan still writing")
	assert.False(t, exists(oldOrphan))
	assert.True(t, exists(newOrphan), "may be between its rename and its sidecar")
	assert.False(t, exists(loneSidecar))
}

func TestPersistentLayerCache_evictionLeavesOtherFilesAlone(t *testing.T) {
	pc := testLayerCache(t, 1)
	now := time.Now()
	pc.now = func() time.Time { return now }

	other := filepath.Join(pc.root, "v0", "sha256-old"+layerCacheTarSuffix)
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o700))
	require.NoError(t, os.WriteFile(other, make([]byte, 100), 0o644))
	beside := filepath.Join(pc.root, "README")
	require.NoError(t, os.WriteFile(beside, make([]byte, 100), 0o644))
	key := seedEntry(t, pc, "e", 10, now.Add(-time.Hour))

	pc.evict()

	assert.True(t, exists(other))
	assert.True(t, exists(beside))
	assert.False(t, exists(pc.tarPath(key)))
}

func TestPersistentLayerCache_evictionOfAMissingDirIsQuiet(t *testing.T) {
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: filepath.Join(t.TempDir(), "never-created")})
	pc.evict()
	assert.False(t, exists(pc.dir))
}

func TestPersistentLayerCache_contains(t *testing.T) {
	root := t.TempDir()
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: root})
	assert.True(t, pc.contains(root))
	assert.True(t, pc.contains(filepath.Join(root, "v1", "x")))
	assert.False(t, pc.contains(filepath.Dir(root)))
	assert.False(t, pc.contains(root+"-sibling"))
	assert.False(t, pc.contains(filepath.Join(root, "..", "elsewhere")))
}

// --- through Image ---

func readImageWithCache(t *testing.T, ctx context.Context, layers []v1.Layer) *Image {
	t.Helper()
	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)
	img := New(v1Img, file.NewTempDirGenerator("layer-cache-test"), t.TempDir())
	require.NoError(t, img.Read(ctx))
	return img
}

func TestImage_Read_persistentCacheIsReusedAcrossImagesAndSurvivesCleanup(t *testing.T) {
	cacheDir := t.TempDir()
	ctx := WithLayerCache(context.Background(), LayerCacheConfig{Dir: cacheDir})

	base, err := random.Layer(1024, v1Types.DockerLayer)
	require.NoError(t, err)
	onlyA, err := random.Layer(1024, v1Types.DockerLayer)
	require.NoError(t, err)
	onlyB, err := random.Layer(1024, v1Types.DockerLayer)
	require.NoError(t, err)

	a := readImageWithCache(t, ctx, []v1.Layer{base, onlyA})
	assert.Equal(t, int64(2), a.layerCache.populated.Load())
	runDir := a.contentCacheDir
	require.NoError(t, a.Cleanup())
	assert.False(t, exists(runDir), "the per-run directory still goes")

	b := readImageWithCache(t, ctx, []v1.Layer{base, onlyB})
	t.Cleanup(func() { require.NoError(t, b.Cleanup()) })
	assert.Equal(t, int64(1), b.layerCache.hits.Load(), "the shared base layer")
	assert.Equal(t, int64(1), b.layerCache.populated.Load())

	plain := readImageWithCache(t, context.Background(), []v1.Layer{base, onlyB})
	t.Cleanup(func() { require.NoError(t, plain.Cleanup()) })
	assert.Nil(t, plain.layerCache)
	for idx := range plain.Layers {
		assert.Equal(t, fileTuples(t, plain.Layers[idx].SquashedTree, plain.FileCatalog), fileTuples(t, b.Layers[idx].SquashedTree, b.FileCatalog))
	}

	entries, err := os.ReadDir(filepath.Join(cacheDir, layerCacheFormat))
	require.NoError(t, err)
	assert.Len(t, entries, 6, "three layers, a tar and a sidecar each")
}

func TestImage_Read_sameDiffIDTwiceInOneImage(t *testing.T) {
	ctx := WithLayerCache(context.Background(), LayerCacheConfig{Dir: t.TempDir()})
	raw, calls := newCounted(t, 512)

	img := readImageWithCache(t, ctx, []v1.Layer{raw, raw})
	t.Cleanup(func() { require.NoError(t, img.Cleanup()) })
	assert.Equal(t, int64(1), calls.Load())
	for _, l := range img.Layers {
		assert.NotEmpty(t, fileTuples(t, l.Tree, img.FileCatalog))
	}
}

func TestImage_Cleanup_neverRemovesThePersistentCache(t *testing.T) {
	root := t.TempDir()
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: root})
	runDir := filepath.Join(root, "run")
	require.NoError(t, os.MkdirAll(runDir, 0o700))

	img := &Image{tmpDirGen: file.NewTempDirGenerator("layer-cache-test"), contentCacheDir: runDir, layerCache: pc}
	require.NoError(t, img.Cleanup())
	assert.True(t, exists(runDir))
	assert.True(t, exists(root))
}
