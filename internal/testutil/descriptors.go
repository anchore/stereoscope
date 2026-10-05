package testutil

import (
	"os"
	"runtime"
	"testing"
	"time"
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

// OpenDescriptorBaseline is OpenDescriptorCount for the "before" side of a leak check. The count is
// process wide, so an *os.File some earlier test dropped without closing is still counted until a GC
// runs its cleanup. If that GC lands inside the window under test the count drops and the check fails
// for code it never ran. This forces those cleanups first so the baseline only holds live descriptors.
//
// Only the baseline flushes: doing the same on the "after" side would let GC close a descriptor the
// code under test leaked, which is exactly what the check is meant to catch.
func OpenDescriptorBaseline(t testing.TB) int {
	t.Helper()

	// cleanups run asynchronously after a GC and there is no API to wait on them, so settle by polling
	// until two consecutive passes agree
	count := -1
	for range 20 {
		runtime.GC()
		time.Sleep(time.Millisecond)
		next := OpenDescriptorCount(t)
		if next == count {
			return count
		}
		count = next
	}
	return count
}
