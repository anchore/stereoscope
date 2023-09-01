package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProviderFromDaemon_imageParsing(t *testing.T) {
	tests := []struct {
		image   string
		want    string
		wantErr require.ErrorAssertionFunc
	}{
		{
			image: "alpine:sometag",
			want:  "alpine:sometag",
		},
		{
			image: "alpine:latest",
			want:  "alpine:latest",
		},
		{
			image: "alpine",
			want:  "alpine:latest",
		},
		{
			image: "registry.place.io/thing:version",
			want:  "registry.place.io/thing:version",
		},
		{
			image: "alpine@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
			want:  "alpine@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
		},
		{
			image: "alpine:sometag@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
			want:  "alpine:sometag@sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209",
		},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got, err := NewProviderFromDaemon(tt.image, nil, nil, nil, "")
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.imageStr)
		})
	}
}

func Test_checkRegistryHostMissing(t *testing.T) {
	tests := []struct {
		image   string
		want    string
		wantErr require.ErrorAssertionFunc
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
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got := checkRegistryHostMissing(tt.image)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}
