package file

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func getFixture(t *testing.T, filepath string) []byte {
	fh, err := os.Open(filepath)
	require.NoError(t, err)
	defer fh.Close()
	expectedContents, err := io.ReadAll(fh)
	require.NoError(t, err)

	return expectedContents
}
