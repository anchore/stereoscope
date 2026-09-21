//go:build containers_image_openpgp

package integration

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// TestContainersStorageSource is an integration test for the containers-storage image source. It is only compiled with
// the `containers_image_openpgp` build tag (the same tag that enables the provider) and is skipped unless buildah is
// available and the process can actually open a store (see the user namespace note below).
//
// Run it explicitly with:
//
//	buildah unshare go test -tags containers_image_openpgp,exclude_graphdriver_btrfs -run TestContainersStorageSource ./test/integration
//
// It builds a tiny image into the current user's containers-storage store and verifies that stereoscope resolves it
// via the explicit `containers-storage:` scheme, observing a marker file that is unique to the local image (so we know
// it did not pull from a registry). Implicit (bare reference) resolution is not asserted here since the podman daemon
// provider is tried first and reads the same store; provider ordering is covered by the unit tests in providers_test.go.
func TestContainersStorageSource(t *testing.T) {
	if _, err := exec.LookPath("buildah"); err != nil {
		t.Skip("buildah not available; skipping containers-storage integration test")
	}

	// a rootless store can only be opened in-process from inside a user namespace (buildah, podman, and skopeo all
	// re-exec themselves into one; a plain go test binary does not), otherwise the overlay graphdriver cannot make
	// its home mount private.
	if os.Geteuid() != 0 && os.Getenv("_CONTAINERS_USERNS_CONFIGURED") == "" {
		t.Skip("rootless containers-storage requires a user namespace; re-run this test under `buildah unshare`")
	}

	const (
		imageRef   = "localhost/stereoscope-containers-storage-test:latest"
		markerPath = "/stereoscope-containers-storage-marker.txt"
	)

	// build the image into the local containers-storage store using buildah
	cmd := exec.Command("buildah", "bud", "-t", imageRef, ".")
	cmd.Dir = "testdata/containers-storage"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("unable to build test image with buildah (environment may not support rootless storage): %v\n%s", err, string(out))
	}
	t.Cleanup(func() {
		// best-effort cleanup of the test image from the store
		_ = exec.Command("buildah", "rmi", imageRef).Run()
	})

	img, err := stereoscope.GetImage(context.Background(), "containers-storage:"+imageRef)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})

	_, ref, err := img.SquashedTree().File(file.Path(markerPath), filetree.FollowBasenameLinks)
	require.NoError(t, err)
	require.NotNil(t, ref, "expected marker file %q to exist in the locally built image", markerPath)
}
