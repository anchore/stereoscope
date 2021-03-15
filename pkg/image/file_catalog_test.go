package image

import (
	"crypto/sha256"
	"fmt"
	"github.com/go-test/deep"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
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

	fh, err := os.Open(tarFixturePath)
	if err != nil {
		t.Fatalf("could not open tar fixture '%s'", tarFixturePath)
	}

	return fh, func() {
		fh.Close()
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

func TestFileCatalog_Add(t *testing.T) {
	ref := file.NewFileReference("/somepath")

	metadata := file.Metadata{
		Path:          "a",
		TarHeaderName: "b",
		Linkname:      "c",
		Size:          1,
		UserID:        2,
		GroupID:       3,
		TypeFlag:      4,
		IsDir:         true,
		Mode:          5,
	}

	layer := &Layer{
		layer: nil,
		Metadata: LayerMetadata{
			Index:     1,
			Digest:    "y",
			MediaType: "z",
			Size:      2,
		},
		Tree:         nil,
		SquashedTree: nil,
		fileCatalog:  nil,
	}

	catalog := NewFileCatalog()
	catalog.Add(*ref, metadata, layer)

	expected := FileCatalogEntry{
		File:     *ref,
		Metadata: metadata,
		Layer:    layer,
	}

	actual, err := catalog.Get(*ref)
	if err != nil {
		t.Fatalf("could not get by ref: %+v", err)
	}

	for d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %+v", d)
	}
}

type testLayerContent struct {
}

func (t *testLayerContent) Digest() (v1.Hash, error) {
	panic("not implemented")
}

func (t *testLayerContent) DiffID() (v1.Hash, error) {
	panic("not implemented")
}

func (t *testLayerContent) Compressed() (io.ReadCloser, error) {
	panic("not implemented")
}

func (t *testLayerContent) Uncompressed() (io.ReadCloser, error) {
	panic("not implemented")
}

func (t *testLayerContent) Size() (int64, error) {
	panic("not implemented")
}

func (t *testLayerContent) MediaType() (types.MediaType, error) {
	panic("not implemented")
}

func TestFileCatalog_FileContents(t *testing.T) {
	fixtureFile, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	// a real path & contents from the fixture
	p := "path/branch/one/file-1.txt"
	ref := file.NewFileReference(file.Path(p))
	expected := "first file\n"

	metadata := file.Metadata{
		Path:          p,
		TarHeaderName: p,
	}

	tr, err := file.NewTarIndex(fixtureFile.Name())
	if err != nil {
		t.Fatalf("unable to get indexed reader")
	}
	layer := &Layer{
		layer:          &testLayerContent{},
		indexedContent: tr,
	}

	catalog := NewFileCatalog()
	catalog.Add(*ref, metadata, layer)

	reader, err := catalog.FileContents(*ref)
	if err != nil {
		t.Fatalf("could not get contents by ref: %+v", err)
	}

	actual, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("could not read content reader: %+v", err)
	}

	for _, d := range deep.Equal([]byte(expected), actual) {
		t.Errorf("diff: %+v", d)
	}
}
