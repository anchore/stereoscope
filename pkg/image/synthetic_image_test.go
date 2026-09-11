package image

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// Helpers for building an image out of hand-written tar layers, so a test can express an exact
// sequence of tar headers without needing docker. Lets tests cover layouts a real build will not
// produce, which is where the interesting hardlink edge cases live.

// tarEntry is a single tar header and the data section that follows it, if any.
type tarEntry struct {
	path     string
	typeFlag byte
	linkPath string
	contents string
	// mode is the header's permission bits, defaulting to 0o644 when left unset. Set it only when a
	// test needs to tell the mode of one entry apart from another's.
	mode int64
}

func layerFromTarEntries(t *testing.T, entries ...tarEntry) v1.Layer {
	t.Helper()

	tarPath := filepath.Join(t.TempDir(), "layer.tar")
	fh, err := os.Create(tarPath)
	require.NoError(t, err)

	tw := tar.NewWriter(fh)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:     entry.path,
			Typeflag: entry.typeFlag,
			Linkname: entry.linkPath,
			Mode:     mode,
			Size:     int64(len(entry.contents)),
		}))
		if entry.contents != "" {
			_, err := io.WriteString(tw, entry.contents)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, fh.Close())

	layer, err := tarball.LayerFromFile(tarPath)
	require.NoError(t, err)

	return layer
}

func readImageFromLayers(t *testing.T, layers ...v1.Layer) *Image {
	t.Helper()

	img, err := tryReadImageFromLayers(t, layers...)
	require.NoError(t, err)

	return img
}

// tryReadImageFromLayers is readImageFromLayers for tests that need to assert on the read error
// itself, rather than have it fail the test.
func tryReadImageFromLayers(t *testing.T, layers ...v1.Layer) (*Image, error) {
	t.Helper()

	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	img := New(v1Img, file.NewTempDirGenerator("image-test"), t.TempDir())
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})

	return img, img.Read(context.Background())
}

// readImageFromLayersWithConfig is readImageFromLayers with the image config rewritten before the
// read, for tests that need the config to disagree with the layers it describes.
func readImageFromLayersWithConfig(t *testing.T, override func(*v1.ConfigFile), layers ...v1.Layer) *Image {
	t.Helper()

	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	require.NoError(t, err)

	img := New(configOverrideImage{Image: v1Img, override: override}, file.NewTempDirGenerator("image-test"), t.TempDir())
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})
	require.NoError(t, img.Read(context.Background()))

	return img
}

// configOverrideImage is a v1.Image that reports a modified config file, so a test can express the
// configs an untrusted image can carry but a real build will never produce.
type configOverrideImage struct {
	v1.Image
	override func(*v1.ConfigFile)
}

func (i configOverrideImage) ConfigFile() (*v1.ConfigFile, error) {
	cfg, err := i.Image.ConfigFile()
	if err != nil {
		return nil, err
	}
	cfg = cfg.DeepCopy()
	i.override(cfg)
	return cfg, nil
}
