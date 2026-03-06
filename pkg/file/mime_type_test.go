package file

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/testutil"
)

func Test_MIMEType(t *testing.T) {

	fileReader := func(path string) io.Reader {
		f, err := os.Open(path)
		require.NoError(t, err)
		return f
	}

	tests := []struct {
		name     string
		fixture  io.Reader
		expected string
	}{
		{
			name:     "binary",
			fixture:  fileReader(testutil.GetFixturePath(t, "mime", "mach-binary")),
			expected: "application/x-mach-binary",
		},
		{
			name:     "script",
			fixture:  fileReader(testutil.GetFixturePath(t, "mime", "capture.sh")),
			expected: "text/x-shellscript",
		},
		{
			name:     "no contents",
			fixture:  strings.NewReader(""),
			expected: "",
		},
		{
			name:     "no reader",
			fixture:  nil,
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, MIMEType(test.fixture))
		})
	}
}
