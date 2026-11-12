package image

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A1 -- the cache directory is only trusted when it belongs to this user and nobody else may write to
// it. A hit is accepted on what the sidecar says (diff ID, size, spot-check digest of the two ends of
// the tar), so whoever can write in the directory chooses what a later scan indexes. These tests plant
// exactly that: a tar and a sidecar that agree with each other.

// plantAnEntry drops a self-consistent entry under a digest -- which is all a lookup can check without
// re-hashing the layer.
func plantAnEntry(t *testing.T, pc *persistentLayerCache, diffID string, content []byte) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	key, ok := layerCacheKey(diffID)
	require.True(t, ok)
	require.NoError(t, os.WriteFile(pc.tarPath(key), content, 0o444))
	edge, size, err := layerCacheEdgeDigest(pc.tarPath(key))
	require.NoError(t, err)
	require.NoError(t, pc.writeSidecar(key, layerCacheSidecar{DiffID: diffID, Size: size, Edge: edge}))
	return key
}

func TestPersistentLayerCache_groupWritableDirIsNotUsed(t *testing.T) {
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir()})
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	require.NoError(t, os.Chmod(pc.dir, 0o770))

	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	diffID, err := raw.DiffID()
	require.NoError(t, err)
	plantAnEntry(t, pc, diffID.String(), []byte("content somebody else chose"))

	l := fetchWith(t, raw, pc)

	assert.False(t, l.contentPersistent, "the planted entry is not used")
	assert.Equal(t, int64(0), pc.hits.Load(), "no lookup is even attempted")
	assert.Equal(t, int64(1), pc.bypassed.Load())
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog), "the layer is read the per-run way instead")
}

func TestPersistentLayerCache_worldWritableDirIsNotUsed(t *testing.T) {
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir()})
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	require.NoError(t, os.Chmod(pc.dir, 0o757))

	assert.Contains(t, pc.trustCheck(), "writable by group or other")
}

func TestPersistentLayerCache_foreignOwnedDirIsNotUsed(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs the privilege to hand a directory to another uid")
	}
	const nobody = 65534 // present on every Linux host this runs on

	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir()})
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	require.NoError(t, os.Chown(pc.dir, nobody, -1))
	t.Cleanup(func() { _ = os.Chown(pc.dir, 0, -1) })

	assert.Contains(t, pc.trustCheck(), "owned by uid")

	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	l := fetchWith(t, raw, pc)

	assert.False(t, l.contentPersistent)
	assert.Equal(t, int64(1), pc.bypassed.Load())
	assert.Equal(t, int64(0), pc.populated.Load())
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
}

func TestPersistentLayerCache_evictNeverDeletesInAnUntrustedDir(t *testing.T) {
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir(), MaxBytes: 1})
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))

	// seed while the directory is still ours, then make it untrusted and try to evict
	key := seedEntry(t, pc, "layer", 4096, time.Now().Add(-time.Hour))
	require.NoError(t, os.Chmod(pc.dir, 0o770))

	pc.evict()

	assert.True(t, exists(pc.tarPath(key)), "the cap is not a licence to delete files we would not read")
	assert.True(t, exists(pc.sidecarPath(key)))
}

func TestPersistentLayerCache_populateRechecksAfterMkdir(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs the privilege to hand a directory to another uid")
	}
	// The directory appearing -- or being swapped for another user's -- between materialize's check and
	// populate's MkdirAll is the race that would otherwise write into, and read back from, it.
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: filepath.Join(t.TempDir(), "cache")})

	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	l := NewLayer(raw)
	t.Cleanup(func() { _ = l.close() })
	l.Metadata, err = newLayerMetadata(raw, 0, "")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	require.NoError(t, os.Chown(pc.dir, 65534, -1))
	t.Cleanup(func() { _ = os.Chown(pc.dir, 0, -1) })

	_, ok, err := pc.materialize(l)
	require.NoError(t, err, "the scan still works; the layer just is not cached")
	assert.False(t, ok)
	assert.Equal(t, int64(1), pc.bypassed.Load())
	assert.Equal(t, int64(0), pc.populated.Load())
	assertNoPartials(t, pc)
}

func TestPersistentLayerCache_symlinkToAForeignDirIsNotUsed(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs the privilege to hand a directory to another uid")
	}
	target := t.TempDir()
	require.NoError(t, os.Chown(target, 65534, -1))
	t.Cleanup(func() { _ = os.Chown(target, 0, -1) })

	link := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.Symlink(target, link))

	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: link})
	assert.Contains(t, pc.trustCheck(), "owned by uid", "a link is judged for what it points at")
}

func TestPersistentLayerCache_nothingThereYetIsTrusted(t *testing.T) {
	// the common case: populate creates the directory itself, 0700, owned by us
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: filepath.Join(t.TempDir(), "layers")})
	assert.Empty(t, pc.trustCheck())
}

func TestPersistentLayerCache_fileInPlaceOfADirIsReported(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o644))
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: blocker})
	assert.Contains(t, pc.trustCheck(), "not a directory")
}

func TestPersistentLayerCache_guardWarnsOnce(t *testing.T) {
	pc := newPersistentLayerCacheFromConfig(LayerCacheConfig{Dir: t.TempDir()})
	require.NoError(t, os.MkdirAll(pc.dir, 0o700))
	require.NoError(t, os.Chmod(pc.dir, 0o770))

	raw, err := random.Layer(256, v1Types.DockerLayer)
	require.NoError(t, err)
	fetchWith(t, raw, pc)
	fetchWith(t, raw, pc)

	assert.True(t, pc.guardWarned.Load(), "the first failure is the one worth logging")
	assert.Equal(t, int64(2), pc.bypassed.Load(), "every layer is bypassed, only one warning is logged")
}

// A2 -- a bypass that happens after the bytes are already on disk must not unpack the layer again.

func TestPersistentLayerCache_layerOverTheCapIsUnpackedOnce(t *testing.T) {
	pc := testLayerCache(t, 4096) // the ~5.6KB tar is over the cap, inside cap+slack
	raw, calls := newCounted(t, 4096)

	l := fetchWith(t, raw, pc)

	assert.Equal(t, int64(1), calls.Load(),
		"the layer was already decompressed into the cache; the scan must not pull it a second time")
	assert.False(t, l.contentPersistent)
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
	assert.Empty(t, cacheFiles(t, pc), "nothing is kept in the cache")
	assertNoPartials(t, pc)
}

func TestPersistentLayerCache_diffIDMismatchIsUnpackedOnce(t *testing.T) {
	pc := testLayerCache(t, 0)
	raw, calls := newCounted(t, 4096)

	l := fetchWith(t, lyingLayer{Layer: raw}, pc)

	assert.Equal(t, int64(1), calls.Load(), "the bytes are handed to the per-run cache instead")
	assert.False(t, l.contentPersistent)
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
	assert.Empty(t, cacheFiles(t, pc))
	assertNoPartials(t, pc)
}

func TestPersistentLayerCache_handedOverLayerBelongsToTheRun(t *testing.T) {
	// The per-run cache owns a handed-over file and deletes it with the run. It must not be marked
	// persistent and must stay writable by us, or it outlives the run it was unpacked for.
	pc := testLayerCache(t, 4096) // the ~5.6KB tar is over the cap, inside cap+slack
	runDir := t.TempDir()
	raw, calls := newCounted(t, 4096)

	l := NewLayer(raw)
	t.Cleanup(func() { _ = l.close() })
	require.NoError(t, l.fetch(0, runDir, pc))

	assert.False(t, l.contentPersistent)
	assert.Equal(t, int64(1), calls.Load())

	entries, err := os.ReadDir(runDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the unpacked layer sits where the per-run cache would have put it")
	assert.Equal(t, l.runCacheName(), entries[0].Name())

	info, err := entries[0].Info()
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o200, "the per-run cache must be able to delete it")
}

func TestPersistentLayerCache_partialReadIsNeverHandedOver(t *testing.T) {
	// A read that dies partway leaves a partial tar. The per-run cache trusts whatever is already at the
	// name it looks for, so a partial file handed over would be indexed as a whole layer and the SBOM
	// would be quietly wrong. The failure has to surface as the error it is.
	pc := testLayerCache(t, 30000) // the read fails at 40000: past the cap, inside cap+slack
	runDir := t.TempDir()
	raw, err := random.Layer(8192, v1Types.DockerLayer)
	require.NoError(t, err)

	l := NewLayer(droppedConnLayer{Layer: raw})
	t.Cleanup(func() { _ = l.close() })
	require.Error(t, l.fetch(0, runDir, pc))

	entries, err := os.ReadDir(runDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, l.runCacheName(), e.Name(), "a partial tar was handed to the per-run cache")
	}
	assert.Empty(t, cacheFiles(t, pc))
	assertNoPartials(t, pc)
}

// droppedConnLayer reads for a while and then fails, the way a dropped registry connection does. It is
// large enough to cross the cache cap first, which is when a hand over would otherwise be considered.
type droppedConnLayer struct{ v1.Layer }

func (droppedConnLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(
		io.LimitReader(&byteReader{}, 40000),
		errReader{},
	)), nil
}

type byteReader struct{ i int }

func (b *byteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b.i + i)
	}
	b.i += len(p)
	return len(p), nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestLayer_runCacheName(t *testing.T) {
	l := &Layer{}
	l.Metadata.Index = 3
	l.Metadata.Digest = "sha256:abcd"
	assert.Equal(t, "3-sha256-abcd", l.runCacheName())

	// two layers claiming one digest still get separate files
	other := &Layer{}
	other.Metadata.Index = 4
	other.Metadata.Digest = "sha256:abcd"
	assert.NotEqual(t, l.runCacheName(), other.runCacheName())
}

func TestPersistentLayerCache_cacheSinkWritesToCapPlusSlack(t *testing.T) {
	// Past the cap the layer cannot be cached, but writing continues to cap+slack so the unpack this run
	// is already paying for can be handed to the caller; past that it is abandoned rather than letting
	// one oversized layer fill a volume the user sized.
	sink := &cacheSink{w: io.Discard, limit: 100, slack: 100}
	_, err := sink.Write(make([]byte, 150))
	require.NoError(t, err)
	assert.Greater(t, sink.written, sink.limit, "over the cap, still writing")

	_, err = sink.Write(make([]byte, 60))
	require.Error(t, err, "cap plus slack is the ceiling")
}

func TestPersistentLayerCache_cacheSinkHugeCapDoesNotOverflow(t *testing.T) {
	// populate passes the cap as both limit and slack; for a cap above MaxInt64/2 the sum used to
	// wrap negative and the very first write was refused, so every layer bypassed the cache.
	for _, limit := range []int64{math.MaxInt64/2 + 1, math.MaxInt64} {
		sink := &cacheSink{w: io.Discard, limit: limit, slack: limit}
		n, err := sink.Write(make([]byte, 4096))
		require.NoError(t, err, "limit=%d", limit)
		assert.Equal(t, 4096, n)
		require.NoError(t, sink.err)
	}

	// the saturated ceiling still refuses a write that would take written past MaxInt64
	sink := &cacheSink{w: io.Discard, limit: math.MaxInt64, slack: math.MaxInt64, written: math.MaxInt64 - 10}
	_, err := sink.Write(make([]byte, 11))
	require.Error(t, err)
}

func TestPersistentLayerCache_layerPastCapPlusSlackIsAbandoned(t *testing.T) {
	// The bound on what one oversized layer may write into the cache volume: past cap+slack the copy is
	// abandoned and the layer is unpacked the per-run way, paying the second read A2 otherwise avoids.
	pc := testLayerCache(t, 1024)
	raw, calls := newCounted(t, 4096) // ~5.6KB tar, past 2048

	l := fetchWith(t, raw, pc)

	assert.Equal(t, int64(2), calls.Load())
	assert.False(t, l.contentPersistent)
	assert.NotEmpty(t, fileTuples(t, l.Tree, l.fileCatalog))
	assert.Empty(t, cacheFiles(t, pc))
	assertNoPartials(t, pc)
}
