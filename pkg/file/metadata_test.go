//go:build !windows
// +build !windows

package file

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-test/deep"
)

func TestFileMetadataFromTar(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

	expected := []Metadata{
		{Path: "/path", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one/file-1.txt", Type: TypeRegular, LinkDestination: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/branch/two", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/two/file-2.txt", Type: TypeRegular, LinkDestination: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/file-3.txt", Type: TypeRegular, LinkDestination: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
	}

	var actual []Metadata
	visitor := func(entry TarFileEntry) error {
		var contents io.Reader
		if strings.HasSuffix(entry.Header.Name, ".txt") {
			contents = strings.NewReader("#!/usr/bin/env bash\necho 'awesome script'")
		}
		actual = append(actual, NewMetadata(entry.Header, contents))
		return nil
	}

	if err := IterateTar(tarReader, visitor); err != nil {
		t.Fatalf("unable to iterate through tar: %+v", err)
	}

	for _, d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %s", d)
	}
}

func TestFileMetadataFromPath(t *testing.T) {

	tests := []struct {
		path             string
		expectedType     Type
		expectedMIMEType string
	}{
		{
			path:             "test-fixtures/symlinks-simple/readme",
			expectedType:     TypeRegular,
			expectedMIMEType: "text/plain",
		},
		{
			path:             "test-fixtures/symlinks-simple/link_to_new_readme",
			expectedType:     TypeSymLink,
			expectedMIMEType: "",
		},
		{
			path:             "test-fixtures/symlinks-simple/link_to_link_to_new_readme",
			expectedType:     TypeSymLink,
			expectedMIMEType: "",
		},
		{
			path:             "test-fixtures/symlinks-simple",
			expectedType:     TypeDirectory,
			expectedMIMEType: "",
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			info, err := os.Lstat(test.path)
			require.NoError(t, err)

			actual := NewMetadataFromPath(test.path, info)
			assert.Equal(t, test.expectedMIMEType, actual.MIMEType, "unexpected MIME type for %s", test.path)
			assert.Equal(t, test.expectedType, actual.Type, "unexpected type for %s", test.path)
		})
	}
}
