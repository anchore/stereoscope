package imagetest

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_writeFileAtomic(t *testing.T) {
	tests := []struct {
		name     string
		writeErr error
		want     string
	}{
		{
			name: "replaces existing file once the write completes",
			want: "new contents",
		},
		{
			name:     "failed write leaves the existing file untouched",
			writeErr: errors.New("docker save failed"),
			want:     "old contents",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.tar")
			require.NoError(t, os.WriteFile(path, []byte("old contents"), 0o644))

			err := writeFileAtomic(path, func(w io.Writer) error {
				// this is the window where another process may read the fixture: it must still see the complete
				// old file, not a truncated one (which is what writing in place with os.Create gave it)
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "old contents", string(got))

				if tt.writeErr != nil {
					return tt.writeErr
				}
				_, err = io.WriteString(w, "new contents")
				return err
			})
			if tt.writeErr != nil {
				require.ErrorIs(t, err, tt.writeErr)
			} else {
				require.NoError(t, err)
			}

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))

			// no temp files are left behind in either case
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 1)
		})
	}
}
