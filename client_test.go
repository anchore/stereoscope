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

func TestGetImageFromSource_imageSquashOnlyIsScopedToTheCall(t *testing.T) {
	img, err := random.Image(1024, 3)
	require.NoError(t, err)

	tag, err := name.NewTag("localhost/squash-only:latest")
	require.NoError(t, err)

	archive := filepath.Join(t.TempDir(), "image.tar")
	require.NoError(t, tarball.WriteToFile(archive, tag, img))

	ctx := context.Background()
	get := func(options ...Option) *image.Image {
		got, err := GetImageFromSource(ctx, archive, image.DockerTarballSource, options...)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, got.Cleanup()) })
		require.Len(t, got.Layers, 3)
		return got
	}

	squashOnly := get(WithImageSquashOnly())
	require.Nil(t, squashOnly.Layers[0].SquashedTree)
	require.NotNil(t, squashOnly.Layers[2].SquashedTree)

	// the same ctx without the option still gets every layer squash
	full := get()
	require.NotNil(t, full.Layers[0].SquashedTree)
}
