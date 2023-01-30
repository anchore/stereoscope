//go:build !windows
// +build !windows

package file

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-test/deep"
)

func TestFileMetadataFromTar(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

	expected := []Metadata{
		{Path: "/path", Type: TypeDir, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch", Type: TypeDir, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one", Type: TypeDir, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one/file-1.txt", Type: TypeReg, LinkDestination: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/branch/two", Type: TypeDir, LinkDestination: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/two/file-2.txt", Type: TypeReg, LinkDestination: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/file-3.txt", Type: TypeReg, LinkDestination: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
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
