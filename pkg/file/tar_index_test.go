package file

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIndexedTarReader_GoCase(t *testing.T) {
	fixture, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	reader, err := NewTarIndex(fixture)
	if err != nil {
		t.Fatal("could not get file reader from tar:", err)
	}

	// all contents + header sizes are below the block size, so accounting for padding will be necessary
	expected := map[string]string{
		"path/branch/one/file-1.txt": "first file\n",
		"path/branch/two/file-2.txt": "second file\n",
		"path/file-3.txt":            "third file\n",
	}

	for name, expectedContents := range expected {
		entries, err := reader.EntriesByName(name)
		if err != nil {
			t.Errorf("unable to get %q : %+v", name, err)
			continue
		}

		if len(entries) != 1 {
			t.Fatalf("unexpected length: %d", len(entries))
		}
		entry := entries[0]

		if entry.Header.Name != name {
			t.Errorf("mismatched header name: %q != %q", entry.Header.Name, name)
		}

		actualContents, err := ioutil.ReadAll(entry.Reader)
		if err != nil {
			t.Errorf("could not read from file reader: %+v", err)
			continue
		}

		if string(actualContents) != expectedContents {
			t.Errorf("unexpected contents for name=%q: '%s'", name, string(actualContents))
		}
	}
}

func TestIndexedTarReader_DuplicateEntries(t *testing.T) {
	fixture := duplicateEntryTarballFixture(t)

	reader, err := NewTarIndex(fixture)
	if err != nil {
		t.Fatal("could not get file reader from tar:", err)
	}

	// all contents are below the block size, so accounting for padding will be necessary
	path := "a/file.path"
	expectedContents := []string{"original", "duplicate"}

	entries, err := reader.EntriesByName(path)
	if err != nil {
		t.Errorf("unable to get %q : %+v", path, err)
	}

	if len(entries) != 2 {
		t.Fatalf("unexpected length: %d", len(entries))
	}

	for idx, entry := range entries {
		if entry.Header.Name != path {
			t.Errorf("mismatched header name: %q != %q", entry.Header.Name, path)
		}

		actualContents, err := ioutil.ReadAll(entry.Reader)
		if err != nil {
			t.Errorf("could not read from file reader: %+v", err)
			continue
		}

		if string(actualContents) != expectedContents[idx] {
			t.Errorf("unexpected contents for name=%q: '%s'", path, string(actualContents))
		}
	}

}

func duplicateEntryTarballFixture(t *testing.T) *os.File {
	tempFile, err := ioutil.TempFile("", "stereoscope-dup-tar-entry-fixture-XXXXXX")
	if err != nil {
		t.Fatalf("could not create tempfile: %+v", err)
	}
	t.Cleanup(func() {
		os.Remove(tempFile.Name())
	})

	tarWriter := tar.NewWriter(tempFile)

	addFileToTarWriter(t, "a/file.path", "original", tarWriter)
	addFileToTarWriter(t, "a/file.path", "duplicate", tarWriter)

	tarWriter.Close()
	tempFile.Close()

	fh, err := os.Open(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to open tar: %+v", err)
	}

	return fh
}

func addFileToTarWriter(t *testing.T, path, contents string, tarWriter *tar.Writer) {
	header := &tar.Header{
		Name:    path,
		Size:    int64(len(contents)),
		Mode:    44,
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("failed to write header for file=%q: %+v", path, err)
	}

	_, err := io.Copy(tarWriter, strings.NewReader(contents))
	if err != nil {
		t.Fatalf("failed to write contents for file=%q: %+v", path, err)
	}
}
