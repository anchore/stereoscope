package containerd

import (
	"fmt"
	"testing"

	"github.com/containerd/platforms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
)

func Test_checkRegistryHostMissing(t *testing.T) {
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
			got := checkRegistryHostMissing(tt.image)
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

func TestValidatePlatform(t *testing.T) {
	isFetchError := func(t require.TestingT, err error, args ...interface{}) {
		var pErr *image.ErrPlatformMismatch
		require.ErrorAs(t, err, &pErr)
	}

	tests := []struct {
		name           string
		expected       *image.Platform
		given          *platforms.Platform
		expectedErrMsg string
		expectedErr    require.ErrorAssertionFunc
	}{
		{
			name:        "nil expected platform",
			expected:    nil,
			given:       &platforms.Platform{OS: "linux", Architecture: "amd64"},
			expectedErr: require.NoError,
		},
		{
			name:           "nil given platform",
			expected:       &image.Platform{OS: "linux", Architecture: "amd64"},
			given:          nil,
			expectedErr:    isFetchError,
			expectedErrMsg: "image has no platform information",
		},
		{
			name:           "OS mismatch",
			expected:       &image.Platform{OS: "linux", Architecture: "amd64"},
			given:          &platforms.Platform{OS: "windows", Architecture: "amd64"},
			expectedErr:    isFetchError,
			expectedErrMsg: `image has unexpected OS "windows", which differs from the user specified PS "linux"`,
		},
		{
			name:           "architecture mismatch",
			expected:       &image.Platform{OS: "linux", Architecture: "amd64"},
			given:          &platforms.Platform{OS: "linux", Architecture: "arm64"},
			expectedErr:    isFetchError,
			expectedErrMsg: `image has unexpected architecture "arm64", which differs from the user specified architecture "amd64"`,
		},
		{
			name:           "variant mismatch",
			expected:       &image.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"},
			given:          &platforms.Platform{OS: "linux", Architecture: "arm64", Variant: "v7"},
			expectedErr:    isFetchError,
			expectedErrMsg: `image has unexpected architecture "v7", which differs from the user specified architecture "v8"`,
		},
		{
			name:        "matching platform",
			expected:    &image.Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			given:       &platforms.Platform{OS: "linux", Architecture: "amd64", Variant: ""},
			expectedErr: require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlatform(tt.expected, tt.given)
			tt.expectedErr(t, err)
			if err != nil {
				assert.ErrorContains(t, err, tt.expectedErrMsg)
			}
		})
	}
}
