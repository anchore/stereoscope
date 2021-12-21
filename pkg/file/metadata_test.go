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
		{Path: "/path", TarSequence: 0, TarHeaderName: "path/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch", TarSequence: 1, TarHeaderName: "path/branch/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one", TarSequence: 2, TarHeaderName: "path/branch/one/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/one/file-1.txt", TarSequence: 3, TarHeaderName: "path/branch/one/file-1.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/branch/two", TarSequence: 4, TarHeaderName: "path/branch/two/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true, MIMEType: ""},
		{Path: "/path/branch/two/file-2.txt", TarSequence: 5, TarHeaderName: "path/branch/two/file-2.txt", TypeFlag: 48, Linkname: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
		{Path: "/path/file-3.txt", TarSequence: 6, TarHeaderName: "path/file-3.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432, IsDir: false, MIMEType: "text/plain"},
	}

	var actual []Metadata
	visitor := func(entry TarFileEntry) error {
		var contents io.Reader
		if strings.HasSuffix(entry.Header.Name, ".txt") {
			contents = strings.NewReader("#!/usr/bin/env bash\necho 'awesome script'")
		}
		actual = append(actual, NewMetadata(entry.Header, entry.Sequence, contents))
		return nil
	}

	if err := IterateTar(tarReader, visitor); err != nil {
		t.Fatalf("unable to iterate through tar: %+v", err)
	}

	for _, d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %s", d)
	}
}
