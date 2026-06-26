//go:build containers_image_openpgp

package integration

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// TestContainersStorageSource is an optional, opt-in integration test for the containers-storage image source. It is
// only compiled with the `containers_image_openpgp` build tag (the same tag that enables the provider) and is skipped
// unless buildah is available, so it does not introduce a required CI dependency on rootless storage / buildah.
//
// Run it explicitly with:
//
//	go test -tags containers_image_openpgp -run TestContainersStorageSource ./test/integration
//
// It builds a tiny image into the current user's containers-storage store (rootless for non-root users, rootful for
// root) and verifies that stereoscope resolves the locally built image both explicitly and implicitly, and that it
// observes a marker file that is unique to the local image (so we know it did not pull from a registry).
func TestContainersStorageSource(t *testing.T) {
	if _, err := exec.LookPath("buildah"); err != nil {
		t.Skip("buildah not available; skipping containers-storage integration test")
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

	assertMarker := func(t *testing.T, userInput string) {
		t.Helper()
		img, err := stereoscope.GetImage(context.Background(), userInput)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, img.Cleanup())
		})

		_, ref, err := img.SquashedTree().File(file.Path(markerPath), filetree.FollowBasenameLinks)
		require.NoError(t, err)
		require.NotNil(t, ref, "expected marker file %q to exist in the locally built image", markerPath)
	}

	t.Run("explicit containers-storage scheme", func(t *testing.T) {
		assertMarker(t, "containers-storage:"+imageRef)
	})

	t.Run("implicit resolution before registry", func(t *testing.T) {
		// note: localhost/... does not exist in any registry, so a successful resolution proves the image was
		// found in the local containers-storage store before any registry fallback.
		assertMarker(t, imageRef)
	})
}
