package filetree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// A malformed lower layer can hold entries under a symlink path (AddFile accepts them as children of the link). An
// upper non-directory at that path masks everything under it ("non-directories are always opaque",
// https://docs.kernel.org/filesystems/overlayfs.html). Before 15a150b the link-resolving opaque check cleared
// /x/vuln here only because the upper link's target happened to be opaque; with the exact-path lookup it survives.
func TestFileTree_Merge_UpperSymlinkMasksChildrenOfLowerSymlink(t *testing.T) {
	lower := New()
	_, err := lower.AddSymLink("/x", "/y")
	require.NoError(t, err)
	_, err = lower.AddFile("/x/vuln")
	require.NoError(t, err)

	upper := New()
	_, err = upper.AddSymLink("/x", "/t")
	require.NoError(t, err)
	_, err = upper.AddDir("/t")
	require.NoError(t, err)
	_, err = upper.AddFile(file.Path("/t/" + file.OpaqueWhiteout))
	require.NoError(t, err)

	require.NoError(t, lower.Merge(upper))

	assert.NotContainsf(t, lower.AllRealPaths(), file.Path("/x/vuln"), "an upper symlink masks the lower path's children; real paths are %v", lower.AllRealPaths())
}

// the same shape, but the upper link's target is not opaque: neither main nor the PR head masks /x/vuln
func TestFileTree_Merge_UpperSymlinkMasksChildrenOfLowerSymlink_TargetNotOpaque(t *testing.T) {
	lower := New()
	_, err := lower.AddSymLink("/x", "/y")
	require.NoError(t, err)
	_, err = lower.AddFile("/x/vuln")
	require.NoError(t, err)

	upper := New()
	_, err = upper.AddSymLink("/x", "/t")
	require.NoError(t, err)
	_, err = upper.AddDir("/t")
	require.NoError(t, err)

	require.NoError(t, lower.Merge(upper))

	assert.NotContainsf(t, lower.AllRealPaths(), file.Path("/x/vuln"), "an upper symlink masks the lower path's children; real paths are %v", lower.AllRealPaths())
}

// the same rule for a lower regular file that holds a child (only a malformed layer can build one): an upper
// non-directory at that path masks the child. Neither main nor the PR head clears it.
func TestFileTree_Merge_UpperFileMasksChildrenOfLowerFile(t *testing.T) {
	lower := New()
	_, err := lower.AddFile("/x")
	require.NoError(t, err)
	_, err = lower.AddFile("/x/vuln")
	require.NoError(t, err)

	upper := New()
	_, err = upper.AddFile("/x")
	require.NoError(t, err)

	require.NoError(t, lower.Merge(upper))

	assert.NotContainsf(t, lower.AllRealPaths(), file.Path("/x/vuln"), "an upper file masks the lower path's children; real paths are %v", lower.AllRealPaths())
}
