package file

import (
	"github.com/go-test/deep"
	"os"
	"testing"
)

func TestFileMetadata_GoCase(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

	expected := []Metadata{
		{Path: "/path", TarSequence: 0, TarHeaderName: "path/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432},
		{Path: "/path/branch", TarSequence: 1, TarHeaderName: "path/branch/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432},
		{Path: "/path/branch/one", TarSequence: 2, TarHeaderName: "path/branch/one/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432},
		{Path: "/path/branch/one/file-1.txt", TarSequence: 3, TarHeaderName: "path/branch/one/file-1.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432},
		{Path: "/path/branch/two", TarSequence: 4, TarHeaderName: "path/branch/two/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432},
		{Path: "/path/branch/two/file-2.txt", TarSequence: 5, TarHeaderName: "path/branch/two/file-2.txt", TypeFlag: 48, Linkname: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432},
		{Path: "/path/file-3.txt", TarSequence: 6, TarHeaderName: "path/file-3.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432},
	}

	var actual []Metadata
	visitor := func(entry TarFileEntry) error {
		actual = append(actual, NewMetadata(entry.Header, entry.Sequence))
		return nil
	}

	if err := IterateTar(tarReader, visitor); err != nil {
		t.Fatalf("unable to iterate through tar: %+v", err)
	}

	for _, d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %s", d)
	}
}

func TestFileMetadata_Xattr(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-xattr")

	type testValue struct {
		path  string
		xattr map[string]string
	}

	expected := []testValue{
		{
			path: "/file-1.txt",
			xattr: map[string]string{
				"user.comment":        "very cool",
				"com.anchore.version": "3.0",
			},
		},
	}

	var actual []testValue
	visitor := func(entry TarFileEntry) error {
		metadata := NewMetadata(entry.Header, entry.Sequence)
		actual = append(actual, testValue{
			path:  metadata.Path,
			xattr: metadata.PAXRecords,
		})
		return nil
	}

	if err := IterateTar(tarReader, visitor); err != nil {
		t.Fatalf("unable to iterate through tar: %+v", err)
	}

	for _, d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %s", d)
	}
}
