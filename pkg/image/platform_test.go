package image

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPlatform(t *testing.T) {
	tests := []struct {
		specifier string
		want      *Platform
		wantErr   assert.ErrorAssertionFunc
	}{
		{
			specifier: "linux",
			want: &Platform{
				OS: "linux",
			},
		},
		{
			specifier: "linux/arm64",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm64",
			},
		},
		{
			specifier: "linux/arm64/v8",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm64",
				Variant:      "", // v8 on arm64is normalized out
			},
		},
		{
			specifier: "linux/arm/v8",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm",
				Variant:      "v8",
			},
		},
		{
			specifier: "arm64",
			want: &Platform{
				OS:           "linux", // default to linux if not provided an OS
				Architecture: "arm64",
			},
		},
		{
			specifier: "arm64/v8",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm64",
				Variant:      "", // v8 on arm64is normalized out
			},
		},
		{
			specifier: "arm/v8",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm",
				Variant:      "v8",
			},
		},
		{
			specifier: "arm",
			want: &Platform{
				OS:           "linux",
				Architecture: "arm",
				Variant:      "v7", // default to v7 if not specified
			},
		},
		{
			specifier: "quindows", // bogus OS
			wantErr:   assert.Error,
		},
		{
			specifier: "windows/aaarm", // bogus arch
			wantErr:   assert.Error,
		},
		{
			specifier: "windows/arm/valpha", // bogus variant, which is allowed
			want: &Platform{
				OS:           "windows",
				Architecture: "arm",
				Variant:      "valpha",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.specifier, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = assert.NoError
			}
			got, err := NewPlatform(tt.specifier)
			if !tt.wantErr(t, err, fmt.Sprintf("NewPlatform(%v)", tt.specifier)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NewPlatform(%v)", tt.specifier)
		})
	}
}
