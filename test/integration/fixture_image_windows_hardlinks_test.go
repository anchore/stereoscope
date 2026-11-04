package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/internal/testutil"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

const windowsHardlinkFixture = "image-windows-nanoserver-sac2016"

// Windows layers link to files in the layers below them: the update layer of nanoserver:sac2016 links
// WinSxS names to System32 files that only the base layer carries, and Windows resolves such a link
// against the parent layers when it imports the layer. So the link name serves the target's bytes, and
// must be cataloged as that file: same size, MIME type and contents, findable by MIME type, and
// attributed to the layer that holds the link.
func TestImageHardLinks_WindowsLinkIntoLowerLayer(t *testing.T) {
	const (
		link   = "/Files/Windows/WinSxS/Temp/PendingDeletes/b2766aa8b35dd40102020000502e542e.SSShim.dll"
		target = "/Files/Windows/System32/SSShim.dll"
	)

	// one fetch for both modes: a second Read reuses the layers already unpacked in the cache
	img := windowsFixtureImage(t)
	for _, lazy := range []bool{false, true} {
		t.Run(fmt.Sprintf("lazy=%v", lazy), func(t *testing.T) {
			if lazy {
				require.NoError(t, img.Read(image.WithLazyLayerSquashes(context.Background())))
			}

			targetMeta, targetLayer := squashedMetadata(t, img, target)
			require.Equal(t, file.TypeRegular, targetMeta.Type)
			require.Equal(t, 0, targetLayer, "the target is only in the base layer")

			linkMeta, linkLayer := squashedMetadata(t, img, link)
			assert.Equal(t, file.TypeRegular, linkMeta.Type, "a hardlink is cataloged as the file it names")
			assert.Equal(t, int64(133472), linkMeta.Size())
			assert.Equal(t, targetMeta.Size(), linkMeta.Size())
			assert.NotEmpty(t, linkMeta.MIMEType)
			assert.Equal(t, targetMeta.MIMEType, linkMeta.MIMEType)
			assert.Equal(t, 1, linkLayer, "the link belongs to the layer that holds it")

			assert.Equal(t, squashedDigest(t, img, target), squashedDigest(t, img, link), "the link serves the target's bytes")

			refs, err := img.FilesByMIMETypeFromSquash(targetMeta.MIMEType)
			require.NoError(t, err)
			var found bool
			for _, ref := range refs {
				if ref.RealPath == link {
					found = true
					break
				}
			}
			assert.True(t, found, "MIME search reaches the link name")

			assert.Empty(t, img.SquashedTree().AllFiles(file.TypeHardLink), "every link resolves against the layer below it")
		})
	}
}

// windowsFixtureImage reads the pinned Windows image straight from its registry. Its layers are foreign layers
// that Microsoft serves from its own URLs, which the registry source follows.
func windowsFixtureImage(t *testing.T) *image.Image {
	t.Helper()
	refBytes, err := os.ReadFile(testutil.GetFixturePath(t, windowsHardlinkFixture, "reference"))
	require.NoError(t, err)
	ref := strings.TrimSpace(string(refBytes))

	img, err := stereoscope.GetImageFromSource(context.Background(), ref, image.OciRegistrySource, stereoscope.WithPlatform("windows/amd64"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, img.Cleanup()) })
	return img
}

func squashedMetadata(t *testing.T, img *image.Image, p string) (file.Metadata, int) {
	t.Helper()
	exists, res, err := img.SquashedTree().File(file.Path(p))
	require.NoError(t, err)
	require.True(t, exists, "%s is in the squash", p)
	require.NotNil(t, res.Reference)

	entry, err := img.FileCatalog.Get(*res.Reference)
	require.NoError(t, err)
	layer := img.FileCatalog.Layer(*res.Reference)
	require.NotNil(t, layer)
	return entry.Metadata, int(layer.Metadata.Index)
}

func squashedDigest(t *testing.T, img *image.Image, p string) string {
	t.Helper()
	r, err := img.OpenPathFromSquash(file.Path(p))
	require.NoError(t, err)
	defer r.Close()
	h := sha256.New()
	_, err = io.Copy(h, r)
	require.NoError(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}
