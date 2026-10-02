package stereoscope

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

// a smoke test of the full fetch path (provider, layer cache, indexing) that needs no daemon or prebuilt
// fixtures, so it can run on every OS we ship binaries for. The Windows CI job exists mainly to run this:
// a cache filename Windows rejects (#705) passes everywhere else.
func TestGetImageFromSource_dockerArchiveSmoke(t *testing.T) {
	img, err := random.Image(1024, 3)
	require.NoError(t, err)

	tag, err := name.NewTag("localhost/smoke:latest")
	require.NoError(t, err)

	archive := filepath.Join(t.TempDir(), "image.tar")
	require.NoError(t, tarball.WriteToFile(archive, tag, img))

	got, err := GetImageFromSource(context.Background(), archive, image.DockerTarballSource)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, got.Cleanup()) })

	require.Len(t, got.Layers, 3)
}
