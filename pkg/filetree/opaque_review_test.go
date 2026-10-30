package filetree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// the opaque marker is looked up at the exact real path: a symlink that points at an opaque directory is not itself
// opaque (main resolved the link and said it was), while the directory holding the marker still is
func TestHasOpaqueDirectory_ExactRealPathOnly(t *testing.T) {
	upper := New()
	_, err := upper.AddSymLink("/x", "/t")
	require.NoError(t, err)
	_, err = upper.AddDir("/t")
	require.NoError(t, err)
	_, err = upper.AddFile(file.Path("/t/" + file.OpaqueWhiteout))
	require.NoError(t, err)

	assert.True(t, hasOpaqueDirectory(upper, "/t"))
	assert.False(t, hasOpaqueDirectory(upper, "/x"))
}
