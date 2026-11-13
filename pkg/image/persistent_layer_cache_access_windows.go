//go:build windows

package image

import (
	"io/fs"
)

// layerCacheDirAccess reports nothing on Windows.
//
// Who may write to a directory there is decided by its security descriptor, and reading that needs the
// Windows security API, which this library does not take on for a cache. Go's mode bits are no
// substitute: every directory is reported 0777 and every file 0666 regardless of the ACL, so a mode
// check here would reject every cache directory on every Windows host.
//
// The practical limit is the same one the Unix check enforces: one OS user per cache directory, and do
// not grant a second account write access to it.
func layerCacheDirAccess(string, fs.FileInfo) string {
	return ""
}
