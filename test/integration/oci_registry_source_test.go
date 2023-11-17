package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
)

func TestOciRegistrySourceMetadata(t *testing.T) {
	rawManifest := `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "size": 1509,
      "digest": "sha256:a24bb4013296f61e89ba57005a7b3e52274d8edd3ae2077d04395f806b63d83e"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 2797541,
         "digest": "sha256:df20fa9351a15782c64e6dddb2d4a6f50bf6d3688060a34c4014b0d9a752eb4c"
      }
   ]
}`
	digest := "sha256:a15790640a6690aa1730c38cf0a440e2aa44aaca9b0e8931a9f2b0d7cc90fd65"
	imgStr := "anchore/test_images"
	ref := fmt.Sprintf("%s@%s", imgStr, digest)

	img, err := stereoscope.GetImage(context.TODO(), "registry:"+ref)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})

	require.NoError(t, img.Read())

	assert.Len(t, img.Metadata.RepoDigests, 1)
	assert.Equal(t, "index.docker.io/"+ref, img.Metadata.RepoDigests[0])
	assert.Equal(t, []byte(rawManifest), img.Metadata.RawManifest)
}

func TestOciRegistry_Proxy(t *testing.T) {
	// note: invalid proxy configuration
	t.Setenv("https_proxy", "http://0.0.0.0:1234")

	// a valid image...
	digest := "sha256:a15790640a6690aa1730c38cf0a440e2aa44aaca9b0e8931a9f2b0d7cc90fd65"
	imgStr := "anchore/test_images"
	ref := fmt.Sprintf("%s@%s", imgStr, digest)

	// note: this should FAIL!
	img, err := stereoscope.GetImage(context.TODO(), "registry:"+ref)
	require.ErrorContains(t, err, "proxyconnect tcp: dial tcp 0.0.0.0:1234: connect: connection refused")

	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})
}
