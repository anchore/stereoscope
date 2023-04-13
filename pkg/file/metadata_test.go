//go:build !windows
// +build !windows

package file

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type expected struct {
	Path            string
	Type            Type
	LinkDestination string
	Size            int64
	Mode            os.FileMode
	UserID          int
	GroupID         int
	IsDir           bool
	MIMEType        string
	ModTime         time.Time
}

func (ex expected) assertEqual(t *testing.T, m Metadata) {
	assert.Equal(t, ex.Path, m.Path)
	assert.Equal(t, ex.Type, m.Type)
	assert.Equal(t, ex.LinkDestination, m.LinkDestination)
	assert.Equal(t, ex.Size, m.Size())
	assert.Equal(t, ex.Mode, m.Mode())
	assert.Equal(t, ex.UserID, m.UserID)
	assert.Equal(t, ex.GroupID, m.GroupID)
	assert.Equal(t, ex.IsDir, m.IsDir())
	assert.Equal(t, ex.MIMEType, m.MIMEType)
	assert.Equal(t, ex.ModTime, m.ModTime())
}

func TestFileMetadataFromTar(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

	expected := []expected{
		{Path: "/path", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: "", ModTime: time.Time{}},
		{Path: "/path/branch", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: "", ModTime: time.Time{}},
		{Path: "/path/branch/one", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: "", ModTime: time.Time{}},
		{Path: "/path/branch/one/file-1.txt", Type: TypeRegular, LinkDestination: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain", ModTime: time.Time{}},
		{Path: "/path/branch/two", Type: TypeDirectory, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: "", ModTime: time.Time{}},
		{Path: "/path/branch/two/file-2.txt", Type: TypeRegular, LinkDestination: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain", ModTime: time.Time{}},
		{Path: "/path/file-3.txt", Type: TypeRegular, LinkDestination: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain", ModTime: time.Time{}},
	}

	var actual []Metadata
	visitor := func(entry TarFileEntry) error {
		var contents io.Reader
		if strings.HasSuffix(entry.Header.Name, ".txt") {
			contents = strings.NewReader("#!/usr/bin/env bash\necho 'awesome script'")
		}

		entry.Header.ModTime = time.Time{}

		actual = append(actual, NewMetadata(entry.Header, contents))
		return nil
	}

	if err := IterateTar(tarReader, visitor); err != nil {
		t.Fatalf("unable to iterate through tar: %+v", err)
	}

	assert.Equal(t, len(expected), len(actual))
	for i, ex := range expected {
		ex.assertEqual(t, actual[i])
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
