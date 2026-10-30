package stereoscope

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/filetree"
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

func TestGetImageFromSource_lazyLayerSquashes(t *testing.T) {
	img, err := random.Image(1024, 3)
	require.NoError(t, err)

	tag, err := name.NewTag("localhost/lazy-squash:latest")
	require.NoError(t, err)

	archive := filepath.Join(t.TempDir(), "image.tar")
	require.NoError(t, tarball.WriteToFile(archive, tag, img))

	get := func(ctx context.Context, options ...Option) *image.Image {
		got, err := GetImageFromSource(ctx, archive, image.DockerTarballSource, options...)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, got.Cleanup()) })
		require.Len(t, got.Layers, 3)
		return got
	}

	ctx := context.Background()
	lazy := get(ctx, WithLazyLayerSquashes())
	// the option is scoped to its call, the same ctx without it is still eager
	eager := get(ctx)
	ctxLazy := get(image.WithLazyLayerSquashes(ctx))

	// the image squash is built during the read either way, lower layers only when lazy was not asked for
	require.IsType(t, &filetree.FileTree{}, lazy.Layers[2].SquashedTree)
	require.IsType(t, &filetree.FileTree{}, eager.Layers[0].SquashedTree)
	for _, l := range []*image.Image{lazy, ctxLazy} {
		_, isTree := l.Layers[0].SquashedTree.(*filetree.FileTree)
		require.False(t, isTree, "lower layer squash was built during a lazy read")
	}

	for idx := range eager.Layers {
		require.ElementsMatchf(t,
			eager.Layers[idx].SquashedTree.AllRealPaths(),
			lazy.Layers[idx].SquashedTree.AllRealPaths(),
			"layer %d squash", idx)
	}
}
