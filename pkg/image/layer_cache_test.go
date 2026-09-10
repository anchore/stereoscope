package image

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncatedContentLayer fails partway through being read, the shape of a dropped connection.
type truncatedContentLayer struct{ mockLayer }

func (truncatedContentLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(io.MultiReader(
		strings.NewReader("partial-bytes"),
		iotest.ErrReader(errors.New("connection dropped")),
	)), nil
}

// a write that dies partway must leave nothing behind: no partial at the final path for a later
// os.Stat to trust, and no orphaned temp file.
func Test_uncompressedCache_failedWriteLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	l := &Layer{layer: truncatedContentLayer{}}
	l.Metadata.Digest = "sha256:doomed"

	_, err := l.uncompressedCache(dir)
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed write must leave the cache directory untouched")
}
