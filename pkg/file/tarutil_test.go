//go:build !windows
// +build !windows

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

	"github.com/stretchr/testify/assert"
)

const (
	fixturesPath = "test-fixtures"
)

var (
	fixturesGeneratorsPath = path.Join(fixturesPath, "generators")
	tarCachePath           = path.Join(fixturesPath, "tar-cache")
)

func TestReaderFromTar_GoCase(t *testing.T) {
	tarReader := getTarFixture(t, "fixture-1")

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
	tarReader := getTarFixture(t, "fixture-1")

	_, err := ReaderFromTar(tarReader, "nOn-ExIsTaNt-paTh")
	if err == nil {
		t.Error("expected an error but did not find one")
	}
}

func TestMetadataFromTar(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		expected Metadata
	}{
		{
			name:    "path/branch/two/file-2.txt",
			fixture: "fixture-1",
			expected: Metadata{
				Path:          "/path/branch/two/file-2.txt",
				TarHeaderName: "path/branch/two/file-2.txt",
				TarSequence:   5,
				Linkname:      "",
				Size:          12,
				UserID:        1337,
				GroupID:       5432,
				TypeFlag:      0x30,
				IsDir:         false,
				Mode:          0x1ed,
				MIMEType:      "application/octet-stream",
			},
		},
		{
			name:    "path/branch/two/",
			fixture: "fixture-1",
			expected: Metadata{
				Path:          "/path/branch/two",
				TarHeaderName: "path/branch/two/",
				TarSequence:   4,
				Linkname:      "",
				Size:          0,
				UserID:        1337,
				GroupID:       5432,
				TypeFlag:      0x35,
				IsDir:         true,
				Mode:          0x800001ed,
				MIMEType:      "",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := getTarFixture(t, "fixture-1")
			metadata, err := MetadataFromTar(f, test.name)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, metadata)
		})
	}
}

func getTarFixture(t testing.TB, name string) *os.File {
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

	t.Cleanup(func() {
		file.Close()
	})

	return file
}

func fixtureVersion(t testing.TB, path string) string {
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

func fileExists(t testing.TB, filename string) bool {
	t.Helper()
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	} else if err != nil {
		t.Fatal(err)
	}
	return !info.IsDir()
}
