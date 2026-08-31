package image

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
