package file

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			fixture:  fileReader("test-fixtures/mime/mach-binary"),
			expected: "application/x-mach-binary",
		},
		{
			name:     "script",
			fixture:  fileReader("test-fixtures/mime/capture.sh"),
			expected: "text/plain",
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
