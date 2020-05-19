package file

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
)

const (
	fixturesPath = "test-fixtures"
)

var (
	fixturesGeneratorsPath = path.Join(fixturesPath, "generators")
	tarCachePath           = path.Join(fixturesPath, "tar-cache")
)

func getTarFixture(t *testing.T, name string) (io.ReadCloser, func()) {
	generatorScriptName := name + ".sh"
	generatorScriptPath := path.Join(fixturesGeneratorsPath, generatorScriptName)
	if !fileExists(t, generatorScriptPath) {
		t.Fatalf("no tar generator script for fixture '%s'", generatorScriptPath)
	}

	version := fixtureVersion(t, generatorScriptPath)
	tarName := name + ":" + version + ".tar"
	tarFixturePath := path.Join(tarCachePath, tarName)

	if !fileExists(t, tarFixturePath) {
		t.Logf("Creating tar fixture: %s", tarFixturePath)

		fullPath, err := filepath.Abs(tarFixturePath)
		if err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command("./"+generatorScriptName, fullPath)
		cmd.Env = os.Environ()
		cmd.Dir = fixturesGeneratorsPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		err = cmd.Run()
		if err != nil {
			panic(err)
		}
	}

	file, err := os.Open(tarFixturePath)
	if err != nil {
		t.Fatalf("could not open tar fixture '%s'", tarFixturePath)
	}

	return file, func() {
		err := file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureVersion(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

func fileExists(t *testing.T, filename string) bool {
	t.Helper()
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		t.Fatal(err)
	}
	return !info.IsDir()
}

func TestReaderFromTar_GoCase(t *testing.T) {
	tarReader, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	fileReader, err := ReaderFromTar(tarReader, "path/branch/two/file-2.txt")
	if err != nil {
		t.Fatal("could not get file reader from tar:", err)
	}

	contents, err := ioutil.ReadAll(fileReader)
	if err != nil {
		t.Fatal("could not read from file reader:", err)
	}

	if string(contents) != "second file\n" {
		t.Errorf("unexpected contents: '%s'", string(contents))
	}
}

func TestReaderFromTar_MissingFile(t *testing.T) {
	tarReader, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	_, err := ReaderFromTar(tarReader, "nOn-ExIsTaNt-paTh")
	if err == nil {
		t.Error("expected an error but did not find one")
	}
}

func TestEnumerateFileMetadataFromTar_GoCase(t *testing.T) {
	tarReader, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	expected := []Metadata{
		{Path: "/path", TarHeaderName: "path/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true},
		{Path: "/path/branch", TarHeaderName: "path/branch/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true},
		{Path: "/path/branch/one", TarHeaderName: "path/branch/one/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o700, UserID: 1337, GroupID: 5432, IsDir: true},
		{Path: "/path/branch/one/file-1.txt", TarHeaderName: "path/branch/one/file-1.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o700, UserID: 1337, GroupID: 5432, IsDir: false},
		{Path: "/path/branch/two", TarHeaderName: "path/branch/two/", TypeFlag: 53, Linkname: "", Size: 0, Mode: os.ModeDir | 0o755, UserID: 1337, GroupID: 5432, IsDir: true},
		{Path: "/path/branch/two/file-2.txt", TarHeaderName: "path/branch/two/file-2.txt", TypeFlag: 48, Linkname: "", Size: 12, Mode: 0o755, UserID: 1337, GroupID: 5432, IsDir: false},
		{Path: "/path/file-3.txt", TarHeaderName: "path/file-3.txt", TypeFlag: 48, Linkname: "", Size: 11, Mode: 0o664, UserID: 1337, GroupID: 5432, IsDir: false},
	}

	idx := 0
	for metadata := range EnumerateFileMetadataFromTar(tarReader) {
		t.Log("Path:", metadata.Path)
		if len(expected) <= idx {
			t.Fatal("more metadata files than expected!")
		}
		if metadata != expected[idx] {
			t.Logf("Mode: actual:%d expected:%d", metadata.Mode, expected[idx].Mode)
			t.Errorf("unexpected file metadata:\n\texpected: %+v\n\tgot     : %+v\n", expected[idx], metadata)

		}
		idx++
	}
}

func TestContentsFromTar_GoCase(t *testing.T) {
	tarReader, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	first := Reference{
		id:   ID(1),
		Path: "path/branch/one/file-1.txt",
	}

	second := Reference{
		id:   ID(2),
		Path: "path/branch/two/file-2.txt",
	}

	third := Reference{
		id:   ID(3),
		Path: "path/file-3.txt",
	}

	expected := map[Reference]string{
		first:  "first file\n",
		second: "second file\n",
		third:  "third file\n",
	}

	request := map[string]Reference{
		string(first.Path):  first,
		string(second.Path): second,
		string(third.Path):  third,
	}

	actual, err := ContentsFromTar(tarReader, request)
	if err != nil {
		t.Fatal("could not read from file reader:", err)
	}

	if len(expected) != len(actual) {
		t.Fatalf("mismatched result lengths: %d!=%d", len(expected), len(actual))
	}

	for expectedRef, expectedContents := range expected {
		actualContents, ok := actual[expectedRef]
		if !ok {
			t.Errorf("could not find key: %+v", expectedRef)
		}
		if actualContents != expectedContents {
			t.Errorf("mismatched contents for key: %+v\n\texpected: %+v\n\tgot     : %+v\n", expectedRef, expectedContents, actualContents)
		}
	}
}

func TestContentsFromTar_MissingFile(t *testing.T) {
	tarReader, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	ref := Reference{
		id:   ID(99),
		Path: "nOn-ExIsTaNt-paTh",
	}

	request := map[string]Reference{
		string(ref.Path): ref,
	}

	_, err := ContentsFromTar(tarReader, request)
	if err == nil {
		t.Error("expected an error but did not find one")
	}
}
