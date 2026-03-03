package sif

import (
	"context"
	"errors"
	"testing"

	"github.com/sylabs/sif/v2/pkg/sif"

	"github.com/anchore/stereoscope/internal/testutil"
	"github.com/anchore/stereoscope/pkg/file"
)

func TestSingularityImageProvider_Provide(t *testing.T) {
	tests := []struct {
		name        string
		fixturePath string
		wantErr     error
	}{
		{
			name:        "NoObjects",
			fixturePath: "empty.sif",
			wantErr:     sif.ErrNoObjects,
		},
		{
			name:        "OK",
			fixturePath: "one-group.sif",
		},
		{
			name:        "FIFO",
			fixturePath: "fifo.sif",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := testutil.GetFixturePath(t, tt.fixturePath)
			p := NewArchiveProvider(file.NewTempDirGenerator(""), path)

			i, err := p.Provide(context.Background())
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
