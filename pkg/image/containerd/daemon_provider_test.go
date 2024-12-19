package containerd

import (
	"fmt"
	"testing"

	"github.com/containerd/platforms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

func Test_ensureRegistryHostPrefix(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{
			image: "alpine:sometag",
			want:  "docker.io/library/alpine:sometag",
		},
		{
			image: "alpine:latest",
			want:  "docker.io/library/alpine:latest",
		},
		{
			image: "alpine",
			want:  "docker.io/library/alpine",
		},
		{
			image: "registry.place.io/thing:version",
			want:  "registry.place.io/thing:version",
		},
		{
			image: "127.0.0.1/thing:version",
			want:  "127.0.0.1/thing:version",
		},
		{
			image: "127.0.0.1:1234/thing:version",
			want:  "127.0.0.1:1234/thing:version",
		},
		{
			image: "localhost/thing:version",
			want:  "localhost/thing:version",
		},
		{
			image: "localhost:1234/thing:version",
			want:  "localhost:1234/thing:version",
		},
		{
			image: "alpine@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
			want:  "docker.io/library/alpine@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
		},
		{
			image: "alpine:sometag@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
			want:  "docker.io/library/alpine:sometag@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
		},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := ensureRegistryHostPrefix(tt.image)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_exportPlatformComparer(t *testing.T) {
	tests := []struct {
		name     string
		platform *image.Platform
		want     platforms.MatchComparer
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name:     "no platform results in linux/amd64",
			platform: nil,
			want:     platforms.OnlyStrict(platforms.MustParse("linux/amd64")),
			wantErr:  assert.NoError,
		},
		{
			name: "honor provided platform",
			platform: func() *image.Platform {
				p, err := image.NewPlatform("darwin/arm64")
				require.NoError(t, err)
				return p
			}(),
			want:    platforms.OnlyStrict(platforms.MustParse("darwin/arm64")),
			wantErr: assert.NoError,
		},
		{
			// note: platforms.Parse() will still allow for invalid platform values, but not malformed inputs (too many "/")
			name: "bad platform errors",
			platform: &image.Platform{
				Architecture: "bogus/extra",
				OS:           "thing/yup",
				Variant:      "here/nope",
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := exportPlatformComparer(tt.platform)
			if !tt.wantErr(t, err, fmt.Sprintf("exportPlatformComparer(%v)", tt.platform)) {
				return
			}
			assert.Equalf(t, tt.want, got, "exportPlatformComparer(%v)", tt.platform)
		})
	}
}
