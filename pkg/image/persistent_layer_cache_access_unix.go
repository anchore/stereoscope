//go:build !windows

package image

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// layerCacheDirAccess returns why this directory cannot be trusted, or "" when it can: it belongs to
// someone else, or anyone other than its owner may write to it.
//
// Only the write bits for group and other matter. Those are what let a second user on this host plant
// a layer that every later scan then indexes as the real one (see trustCheck).
func layerCacheDirAccess(dir string, info fs.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// nothing better to go on than the mode
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Sprintf("%q is writable by group or other (mode %s)", dir, info.Mode().Perm())
		}
		return ""
	}
	if st.Uid != uint32(os.Geteuid()) {
		return fmt.Sprintf("%q is owned by uid %d, not by this user", dir, st.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Sprintf("%q is writable by group or other (mode %s)", dir, info.Mode().Perm())
	}
	return ""
}
