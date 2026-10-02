package image

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
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

// cache files must be nameable on every OS we ship to, and this runs on any OS. A colon in particular
// is NTFS alternate data stream syntax, so os.Create succeeds but the rename into place fails (#705).
func Test_uncompressedCache_filenameIsPortable(t *testing.T) {
	dir := t.TempDir()
	raw, err := random.Layer(64, types.DockerLayer)
	require.NoError(t, err)
	l := &Layer{layer: raw}
	l.Metadata.Index = 3
	l.Metadata.Digest = "sha256:a6dc765193a52cbaede3c93a9ff48b9540948dd0774bbe7031fcb5847606f9ac"

	p, err := l.uncompressedCache(dir)
	require.NoError(t, err)

	name := filepath.Base(p)
	assert.Equal(t, "3-sha256-a6dc765193a52cbaede3c93a9ff48b9540948dd0774bbe7031fcb5847606f9ac", name)
	assert.False(t, strings.ContainsAny(name, `<>:"/\|?*`), "cache filename %q has a character Windows reserves", name)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, name, entries[0].Name())
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
