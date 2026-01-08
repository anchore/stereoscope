package image

import (
	"errors"
	"io"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
)

type mockLayer struct {
	mediaType v1Types.MediaType
	err       error
}

func (m mockLayer) Digest() (v1.Hash, error) {
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       "aaaaaaaaaa1234",
	}, nil
}

func (m mockLayer) DiffID() (v1.Hash, error) {
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       "aaaaaaaaaa1234",
	}, nil
}

func (m mockLayer) Compressed() (io.ReadCloser, error) {
	panic("implement me")
}

func (m mockLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m mockLayer) Size() (int64, error) {
	return 0, nil
}

func (m mockLayer) MediaType() (v1Types.MediaType, error) {
	return m.mediaType, m.err
}

var _ v1.Layer = &mockLayer{}

func fakeLayer(mediaType v1Types.MediaType, err error) v1.Layer {
	return mockLayer{
		mediaType: mediaType,
		err:       err,
	}
}

func TestRead(t *testing.T) {
	tests := []struct {
		name            string
		mediaType       v1Types.MediaType
		mediaTypeErr    error
		wantErrContents string
	}{
		{
			name:            "unsupported media type",
			mediaType:       "garbage",
			mediaTypeErr:    nil,
			wantErrContents: "unknown layer media type: garbage",
		},
		{
			name:            "unsupported media type: helm chart",
			mediaType:       "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
			wantErrContents: "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
		},
		{
			name:            "err on media type returned",
			mediaTypeErr:    errors.New("no media type for you"),
			wantErrContents: "no media type for you",
		},
		{
			name:      "support OCI layer",
			mediaType: v1Types.OCILayer,
		},
		{
			name:      "support OCI uncompressed layer",
			mediaType: v1Types.OCIUncompressedLayer,
		},
		{
			name:      "support OCI restricted layer",
			mediaType: v1Types.OCIRestrictedLayer,
		},
		{
			name:      "support OCI uncompressed restricted layer",
			mediaType: v1Types.OCIUncompressedRestrictedLayer,
		},
		{
			name:      "support OCI zstd layer",
			mediaType: v1Types.OCILayerZStd,
		},
		{
			name:      "support docker tar.gz layer",
			mediaType: v1Types.DockerLayer,
		},
		{
			name:      "support docker foreign layer",
			mediaType: v1Types.DockerForeignLayer,
		},
		{
			name:      "support docker uncompressed layer",
			mediaType: v1Types.DockerUncompressedLayer,
		},
		{
			name:      "support docker tar.zstd layer",
			mediaType: BuildKitZstdCompressedLayer,
		},
		{
			name:      "support docker tar+zstd layer",
			mediaType: BuildKitZstdCompressedLayerAlt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := Layer{layer: fakeLayer(tt.mediaType, tt.mediaTypeErr)}
			catalog := NewFileCatalog()
			err := layer.Read(catalog, 0, t.TempDir())
			if tt.wantErrContents != "" {
				require.ErrorContains(t, err, tt.wantErrContents)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsSupportedLayerMediaType(t *testing.T) {
	tests := []struct {
		name      string
		mediaType v1Types.MediaType
		want      bool
	}{
		{
			name:      "OCI layer",
			mediaType: v1Types.OCILayer,
			want:      true,
		},
		{
			name:      "OCI uncompressed layer",
			mediaType: v1Types.OCIUncompressedLayer,
			want:      true,
		},
		{
			name:      "OCI restricted layer",
			mediaType: v1Types.OCIRestrictedLayer,
			want:      true,
		},
		{
			name:      "OCI uncompressed restricted layer",
			mediaType: v1Types.OCIUncompressedRestrictedLayer,
			want:      true,
		},
		{
			name:      "OCI zstd layer",
			mediaType: v1Types.OCILayerZStd,
			want:      true,
		},
		{
			name:      "Docker layer",
			mediaType: v1Types.DockerLayer,
			want:      true,
		},
		{
			name:      "Docker foreign layer",
			mediaType: v1Types.DockerForeignLayer,
			want:      true,
		},
		{
			name:      "Docker uncompressed layer",
			mediaType: v1Types.DockerUncompressedLayer,
			want:      true,
		},
		{
			name:      "BuildKit zstd layer",
			mediaType: BuildKitZstdCompressedLayer,
			want:      true,
		},
		{
			name:      "BuildKit zstd layer alt",
			mediaType: BuildKitZstdCompressedLayerAlt,
			want:      true,
		},
		{
			name:      "Singularity squashfs layer",
			mediaType: SingularitySquashFSLayer,
			want:      true,
		},
		{
			name:      "unsupported garbage type",
			mediaType: "garbage",
			want:      false,
		},
		{
			name:      "unsupported helm chart",
			mediaType: "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSupportedLayerMediaType(tt.mediaType)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestValidateLayerMediaTypes(t *testing.T) {
	tests := []struct {
		name            string
		layers          []v1.Layer
		wantErrContents string
	}{
		{
			name: "all supported types",
			layers: []v1.Layer{
				fakeLayer(v1Types.OCILayer, nil),
				fakeLayer(v1Types.DockerLayer, nil),
			},
		},
		{
			name: "one unsupported type",
			layers: []v1.Layer{
				fakeLayer(v1Types.OCILayer, nil),
				fakeLayer("garbage", nil),
			},
			wantErrContents: "unsupported layer media type(s): layer 1: garbage",
		},
		{
			name: "multiple unsupported types",
			layers: []v1.Layer{
				fakeLayer("garbage", nil),
				fakeLayer(v1Types.OCILayer, nil),
				fakeLayer("also-garbage", nil),
			},
			wantErrContents: "unsupported layer media type(s): layer 0: garbage, layer 2: also-garbage",
		},
		{
			name: "media type error",
			layers: []v1.Layer{
				fakeLayer("", errors.New("no media type")),
			},
			wantErrContents: "unable to get media type for layer 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLayerMediaTypes(tt.layers)
			if tt.wantErrContents != "" {
				require.ErrorContains(t, err, tt.wantErrContents)
				return
			}
			require.NoError(t, err)
		})
	}
}
