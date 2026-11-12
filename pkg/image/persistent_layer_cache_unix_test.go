//go:build !windows

package image

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// A FIFO planted where an entry's tar should be must be refused without opening it: an open blocks
// until a writer appears, which would hang every scan that looked the layer up.
func TestPersistentLayerCache_edgeDigestRefusesAFIFOWithoutBlocking(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, unix.Mkfifo(fifo, 0o600))

	done := make(chan error, 1)
	go func() {
		_, _, err := layerCacheEdgeDigest(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("layerCacheEdgeDigest blocked on a FIFO")
	}
}
