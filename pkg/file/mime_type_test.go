package file

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func Test_MIMEType(t *testing.T) {

	tests := []struct {
		fixture  string
		expected string
	}{
		{
			// darwin binary
			fixture:  "test-fixtures/mime/mach-binary",
			expected: "application/x-mach-binary",
		},
		{
			// script
			fixture:  "test-fixtures/mime/capture.sh",
			expected: "text/plain",
		},
		{
			// no contents
			fixture:  "",
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			var f *os.File
			var err error
			if test.fixture != "" {
				f, err = os.Open(test.fixture)
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, MIMEType(f))
		})
	}
}
