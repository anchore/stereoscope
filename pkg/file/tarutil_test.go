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

func getTarFixture(t *testing.T, name string) (*os.File, func()) {
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
