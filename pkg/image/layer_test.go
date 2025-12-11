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
