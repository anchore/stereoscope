package sif

import (
	"errors"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sylabs/sif/v2/pkg/sif"
)

func Test_newSIFImage(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantErr    error
		wantArch   string
		wantDiffID v1.Hash
	}{
		{
			name:    "NoObjects",
			path:    filepath.Join("test-fixtures", "empty.sif"),
			wantErr: sif.ErrNoObjects,
		},
		{
			name:     "OK",
			path:     filepath.Join("test-fixtures", "one-group.sif"),
			wantArch: "386",
			wantDiffID: v1.Hash{
				Algorithm: "sha256",
				Hex:       "9f9c4e5e131934969b4ac8f495691c70b8c6c8e3f489c2c9ab5f1af82bce0604",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im, err := newSIFImage(tt.path)

			if got, want := err, tt.wantErr; !errors.Is(got, want) {
				t.Fatalf("got error %v, want %v", got, want)
			}

			if im != nil {
				if got, want := tt.path, im.path; got != want {
					t.Errorf("got path %v, want %v", got, want)
				}

				if got, want := tt.wantArch, im.arch; got != want {
					t.Errorf("got arch %v, want %v", got, want)
				}

				if _, ok := im.diffIDs[tt.wantDiffID]; !ok {
					t.Errorf("diffID %v not found", tt.wantDiffID)
				}
			}
		})
	}
}
