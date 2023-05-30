package integration

import (
	"context"
	"fmt"
	"runtime"
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

func TestOciRegistryArchHandling(t *testing.T) {
	// possible platforms are at
	// https://github.com/docker/cli/blob/1f6a1a438c4ae426e446f17848114e58072af2bb/cli/command/manifest/util.go#L22
	tests := []struct {
		name     string
		wantErr  bool
		userStr  string
		options  []stereoscope.Option
		wantArch string
	}{
		{
			name:    "arch requested, arch available",
			userStr: "registry:alpine:3.18.0",
			options: []stereoscope.Option{
				stereoscope.WithPlatform("linux/amd64"),
			},
		},
		{
			name:    "arch requested, arch unavailable",
			userStr: "registry:alpine:3.18.0",
			wantErr: true,
			options: []stereoscope.Option{
				stereoscope.WithPlatform("linux/mips64"),
			},
		},
		{
			name:     "multi-arch index, no arch requested, host arch available",
			userStr:  "registry:alpine:3.18.0",
			wantArch: runtime.GOARCH,
		},
		{
			name:    "multi-arch index, no arch requested, host arch unavailable",
			wantErr: true,
			userStr: fmt.Sprintf("registry:%s", getMultiArchImageNotContainingHostArch()),
			// TODO: this is a really hard one
			// maybe make some test images?
		},
		{
			name:    "single arch index",
			wantErr: false,
			userStr: fmt.Sprintf("registry:%s", getSingleArchImageNotMatchingHostArch()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := stereoscope.GetImage(context.TODO(), tt.userStr, tt.options...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantArch != "" {
				assert.Equal(t, tt.wantArch, img.Metadata.Architecture)
			}
			if img != nil {
				t.Logf("%s", img.Metadata.Architecture)
			}
		})
	}
}

func getSingleArchImageNotMatchingHostArch() string {
	if runtime.GOARCH != "amd64" {
		return "rancher/busybox:1.31.1"
	}
	return ""
}

func getMultiArchImageNotContainingHostArch() string {
	return ""
}
