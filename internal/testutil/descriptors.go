package testutil

import (
	"os"
	"testing"
)

// OpenDescriptorCount reports how many file descriptors this process currently holds. Use it to
// assert that a code path released what it opened: on unix a leaked descriptor is otherwise
// invisible, since unlinking a file succeeds while a handle to it is still open.
//
// Reads /dev/fd with Readdirnames rather than os.ReadDir, which stats each entry and fails on darwin
// for the directory handle doing the reading.
func OpenDescriptorCount(t testing.TB) int {
	t.Helper()

	fh, err := os.Open("/dev/fd")
	if err != nil {
		t.Skipf("cannot enumerate open descriptors on this platform: %v", err)
	}
	defer fh.Close()

	names, err := fh.Readdirnames(-1)
	if err != nil {
		t.Fatalf("unable to enumerate open descriptors: %v", err)
	}

	// the handle opened above is itself listed
	return len(names) - 1
}
