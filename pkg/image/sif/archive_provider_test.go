package sif

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sylabs/sif/v2/pkg/sif"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func TestSingularityImageProvider_Provide(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		userMetadata []image.AdditionalMetadata
		wantErr      error
	}{
		{
			name:    "NoObjects",
			path:    filepath.Join("test-fixtures", "empty.sif"),
			wantErr: sif.ErrNoObjects,
		},
		{
			name: "OK",
			path: filepath.Join("test-fixtures", "one-group.sif"),
		},
		{
			name: "FIFO",
			path: filepath.Join("test-fixtures", "fifo.sif"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewArchiveProvider(file.NewTempDirGenerator(""))

			i, err := p.Provide(context.Background(), tt.path, nil)
			t.Cleanup(func() { _ = i.Cleanup() })

			if got, want := err, tt.wantErr; !errors.Is(got, want) {
				t.Fatalf("got error %v, want %v", got, want)
			}

			if err == nil {
				if err := i.Read(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
