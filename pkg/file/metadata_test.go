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

func assertMetadataEqual(t *testing.T, expected, actual Metadata) {
	if !assert.True(t, expected.Equal(actual)) {
		assert.Equal(t, expected.RealPath, actual.RealPath, "mismatched path")
		assert.Equal(t, expected.Type, actual.Type, "mismatched type")
		assert.Equal(t, expected.LinkDestination, actual.LinkDestination, "mismatched link destination")
		assert.Equal(t, expected.Name(), actual.Name(), "mismatched name")
		assert.Equal(t, expected.Size(), actual.Size(), "mismatched size")
		assert.Equal(t, expected.Mode(), actual.Mode(), "mismatched mode")
		assert.Equal(t, expected.UserID, actual.UserID, "mismatched user id")
		assert.Equal(t, expected.GroupID, actual.GroupID, "mismatched group id")
		assert.Equal(t, expected.IsDir(), actual.IsDir(), "mismatched is dir")
		assert.Equal(t, expected.MIMEType, actual.MIMEType, "mismatched mime type")
		exMod := expected.FileInfo.ModTime()
		acMod := actual.FileInfo.ModTime()
		if !assert.True(t, exMod.UTC().Equal(acMod.UTC()), "mismatched mod time (UTC)") {
			assert.Equal(t, exMod, acMod, "mod time details")
		}
	}
}

func TestFileMetadataFromTar(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

	ex := []Metadata{
		{
			RealPath:        "/path",
			Type:            TypeDirectory,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "",
			FileInfo: ManualInfo{
				NameValue:    "path",
				SizeValue:    0,
				ModeValue:    os.ModeDir | 0o755,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/branch",
			Type:            TypeDirectory,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "",
			FileInfo: ManualInfo{
				NameValue:    "branch",
				SizeValue:    0,
				ModeValue:    os.ModeDir | 0o755,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/branch/one",
			Type:            TypeDirectory,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "",
			FileInfo: ManualInfo{
				NameValue:    "one",
				SizeValue:    0,
				ModeValue:    os.ModeDir | 0o700,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/branch/one/file-1.txt",
			Type:            TypeRegular,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "text/plain",
			FileInfo: ManualInfo{
				NameValue:    "file-1.txt",
				SizeValue:    11,
				ModeValue:    0o700,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/branch/two",
			Type:            TypeDirectory,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "",
			FileInfo: ManualInfo{
				NameValue:    "two",
				SizeValue:    0,
				ModeValue:    os.ModeDir | 0o755,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/branch/two/file-2.txt",
			Type:            TypeRegular,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "text/plain",
			FileInfo: ManualInfo{
				NameValue:    "file-2.txt",
				SizeValue:    12,
				ModeValue:    0o755,
				ModTimeValue: time.Time{},
			},
		},
		{
			RealPath:        "/path/file-3.txt",
			Type:            TypeRegular,
			LinkDestination: "",
			UserID:          1337,
			GroupID:         5432,
			MIMEType:        "text/plain",
			FileInfo: ManualInfo{
				NameValue:    "file-3.txt",
				SizeValue:    11,
				ModeValue:    0o664,
				ModTimeValue: time.Time{},
			},
		},
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

	assert.Equal(t, len(ex), len(actual))
	for i, e := range ex {
		assertMetadataEqual(t, e, actual[i])
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
