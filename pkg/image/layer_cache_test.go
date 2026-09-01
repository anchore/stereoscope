package image

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// the per-path lock hands the path to one writer at a time; others wait rather than interleave
func Test_lockCachePath_serializesWriters(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sha256-x")
	var active, maxActive int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := lockCachePath(target)
			defer unlock()
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			// simulate the stat-then-write critical section
			if _, err := os.Stat(target); os.IsNotExist(err) {
				require.NoError(t, os.WriteFile(target, []byte("layer-bytes"), 0o600))
			}
			mu.Lock()
			active--
			mu.Unlock()
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, maxActive, "writers must not overlap on one cache path")
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "layer-bytes", string(got))
	// no .partial temp files may remain
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".partial"), e.Name())
	}
}

// countingContentLayer serves fixed content and counts how many times it is asked for it.
type countingContentLayer struct {
	mockLayer
	opens   *int32
	content string
}

func (c countingContentLayer) Uncompressed() (io.ReadCloser, error) {
	atomic.AddInt32(c.opens, 1)
	return io.NopCloser(strings.NewReader(c.content)), nil
}

// truncatedContentLayer fails partway through being read, the shape of a dropped connection.
type truncatedContentLayer struct{ mockLayer }

func (truncatedContentLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(strings.NewReader("partial-bytes"), iotest.ErrReader(errors.New("connection dropped")))), nil
}

func Test_uncompressedCache_concurrentSameDigestPopulatesOnce(t *testing.T) {
	dir := t.TempDir()
	var opens int32

	// several layers of one image can share a digest; every reader must see a complete layer and
	// the content must only be materialized once
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := &Layer{layer: countingContentLayer{opens: &opens, content: "layer-bytes"}}
			l.Metadata.Digest = "sha256:shared"
			path, err := l.uncompressedCache(dir)
			if assert.NoError(t, err) {
				got, err := os.ReadFile(path)
				if assert.NoError(t, err) {
					assert.Equal(t, "layer-bytes", string(got))
				}
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), opens, "the layer content must be fetched exactly once")
	assertNoPartialFiles(t, dir)
}

func Test_uncompressedCache_existingCompleteFileIsReused(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sha256:cached"), []byte("already-here"), 0o600))

	var opens int32
	l := &Layer{layer: countingContentLayer{opens: &opens, content: "should-not-be-read"}}
	l.Metadata.Digest = "sha256:cached"

	path, err := l.uncompressedCache(dir)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "already-here", string(got))
	assert.Zero(t, opens, "a complete cached layer must not be re-fetched")
}

func Test_uncompressedCache_failedReadLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	l := &Layer{layer: truncatedContentLayer{}}
	l.Metadata.Digest = "sha256:doomed"

	_, err := l.uncompressedCache(dir)
	require.Error(t, err)

	// neither a partial at the final path (a later hit would trust it) nor an orphaned temp file
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed write must leave the cache directory untouched")
}

func assertNoPartialFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".partial"), "leftover temp file: %s", e.Name())
	}
}
