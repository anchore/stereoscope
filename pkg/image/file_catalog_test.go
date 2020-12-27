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

var testFilePaths = []file.Path{
	"/home",
	"/home/dan",
	"/home/alex",
	"/home/alfredo",
	"/home/alfredo/special-file",
}

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
		file.Close()
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

func testFileCatalog(t *testing.T) FileCatalog {
	tempDir, err := ioutil.TempDir("", "stereoscope-file-catalog-test")
	if err != nil {
		t.Fatalf("could not create tempfile: %+v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return NewFileCatalog(tempDir)
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

	catalog := testFileCatalog(t)
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
	content io.ReadCloser
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
	return t.content, nil
}

func (t *testLayerContent) Size() (int64, error) {
	panic("not implemented")
}

func (t *testLayerContent) MediaType() (types.MediaType, error) {
	panic("not implemented")
}

func TestFileCatalog_FileContents(t *testing.T) {
	actualReadCloser, cleanup := getTarFixture(t, "fixture-1")
	defer cleanup()

	// a real path & contents from the fixture
	p := "path/branch/one/file-1.txt"
	ref := file.NewFileReference(file.Path(p))
	expected := "first file\n"

	metadata := file.Metadata{
		Path:          p,
		TarHeaderName: p,
	}

	layer := &Layer{
		layer: &testLayerContent{content: actualReadCloser},
	}

	catalog := testFileCatalog(t)
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

func setupMultipleFileContents(t *testing.T, fileSize int64) (FileCatalog, map[file.Reference]string, []file.Reference) {
	// a real path & contents from the fixture
	ref1 := file.NewFileReference("path/branch/one/file-1.txt")
	ref2 := file.NewFileReference("path/branch/two/file-2.txt")
	entries := map[file.Reference]string{
		*ref1: "first file\n",
		*ref2: "second file\n",
	}

	catalog := testFileCatalog(t)

	for ref := range entries {
		metadata := file.Metadata{
			Path:          string(ref.RealPath),
			TarHeaderName: string(ref.RealPath),
			Size:          fileSize,
		}

		// these "layers" cannot share the same readcloser
		actualReadCloser, cleanup := getTarFixture(t, "fixture-1")
		t.Cleanup(cleanup)

		layer := &Layer{
			// note: since this test is using the same tar, it is as if it is a request for two files in the same layer
			layer: &testLayerContent{content: actualReadCloser},
		}

		catalog.Add(ref, metadata, layer)
	}

	var refs = []file.Reference{*ref1, *ref2}

	return catalog, entries, refs
}

func assertMultipleFileContents(t *testing.T, expectedContents map[file.Reference]string, actualReaders map[file.Reference]io.ReadCloser) {
	for ref, actualReader := range actualReaders {
		expectedStr, ok := expectedContents[ref]
		if !ok {
			t.Fatalf("could not find ref: %+v", ref)
		}
		actualBytes, err := ioutil.ReadAll(actualReader)
		if err != nil {
			t.Fatalf("could not read content reader: %+v", err)
		}

		for _, d := range deep.Equal([]byte(expectedStr), actualBytes) {
			t.Errorf("diff: %+v", d)
		}
	}
}

func TestFileCatalog_MultipleFileContents_NoCache(t *testing.T) {
	// note: the file size is below the cache threshold
	catalog, expected, refs := setupMultipleFileContents(t, 20)

	actual, err := catalog.MultipleFileContents(refs...)
	if err != nil {
		t.Fatalf("could not get contents by ref: %+v", err)
	}

	assertMultipleFileContents(t, expected, actual)
}

func TestFileCatalog_MultipleFileContents_WithCache(t *testing.T) {
	// note: the file size is above the cache threshold
	catalog, expected, refs := setupMultipleFileContents(t, 2*cacheFileSizeThreshold)

	actual, err := catalog.MultipleFileContents(refs...)
	if err != nil {
		t.Fatalf("could not get contents by ref: %+v", err)
	}

	if len(catalog.contentsCachePath) != len(refs) {
		t.Fatalf("did not cache results")
	}

	// ensure the cache is there and the contents are what you would expect
	for cacheID, p := range catalog.contentsCachePath {
		fh, err := os.Open(p)
		if err != nil {
			t.Fatalf("could not get cache file=%+v : %+v", cacheID, err)
		}
		cachedBytes, err := ioutil.ReadAll(fh)
		if err != nil {
			t.Fatalf("could not read cache file=%+v : %+v", cacheID, err)
		}

		entry, ok := catalog.catalog[cacheID]
		if !ok {
			t.Fatalf("could not find entry for ID=%+v", cacheID)
		}

		expectedStr, ok := expected[entry.File]
		if !ok {
			t.Fatalf("could not find expected result for ref=%+v", entry.File)
		}

		if expectedStr != string(cachedBytes) {
			t.Errorf("mismatched contents: %q != %q", expectedStr, string(cachedBytes))
		}
	}

	// ensure contents are expected via the API (not verifying manually)
	assertMultipleFileContents(t, expected, actual)
}

//func TestFileCatalog_HasEntriesForAllFilesInTree(t *testing.T) {
//	cases := []struct {
//		name     string
//		setup    func(t *testing.T, filePaths []file.Path, fileTree *filetree.FileTree, catalog *FileCatalog)
//		expected bool
//	}{
//		{
//			name: "identical set of files",
//			setup: func(t *testing.T, filePaths []file.Path, fileTree *filetree.FileTree, catalog *FileCatalog) {
//				for _, p := range filePaths {
//					f, err := fileTree.AddFile(p)
//					if err != nil {
//						t.Fatal(err)
//					}
//					catalog.Add(*f, file.Metadata{}, &Layer{})
//				}
//			},
//			expected: true,
//		},
//		{
//			name: "catalog missing one file that tree has",
//			setup: func(t *testing.T, filePaths []file.Path, fileTree *filetree.FileTree, catalog *FileCatalog) {
//				for i, p := range filePaths {
//					f, err := fileTree.AddFile(p)
//					if err != nil {
//						t.Fatal(err)
//					}
//
//					if i != 1 { // don't add filePaths[1] to the catalog
//						catalog.Add(*f, file.Metadata{}, &Layer{})
//					}
//				}
//			},
//			expected: false,
//		},
//		{
//			name: "tree missing one file that catalog has",
//			setup: func(t *testing.T, filePaths []file.Path, fileTree *filetree.FileTree, catalog *FileCatalog) {
//				for i, p := range filePaths {
//					if i == 1 { // add filePaths[1] to only the catalog, not the tree
//						catalog.Add(*file.NewFileReference(p), file.Metadata{}, &Layer{})
//						return
//					}
//
//					f, err := fileTree.AddFile(p)
//					if err != nil {
//						t.Fatal(err)
//					}
//					catalog.Add(*f, file.Metadata{}, &Layer{})
//				}
//			},
//			expected: true,
//		},
//		{
//			name: "no files added to tree",
//			setup: func(t *testing.T, filePaths []file.Path, fileTree *filetree.FileTree, catalog *FileCatalog) {
//				for _, p := range filePaths {
//					f := file.NewFileReference(p)
//					catalog.Add(*f, file.Metadata{}, &Layer{})
//				}
//			},
//			expected: true,
//		},
//	}
//
//	for _, tc := range cases {
//		t.Run(tc.name, func(t *testing.T) {
//			fileTree := filetree.NewFileTree()
//
//			catalog := testFileCatalog(t)
//
//			// Add file tree root to catalog
//			exists, _, f, _ := fileTree.File("/", false)
//			if !exists {
//				t.Fatalf("nice going, you somehow deleted / [the universe then collapsed on itself]")
//			}
//			catalog.Add(*f, file.Metadata{}, &Layer{})
//
//			tc.setup(t, testFilePaths, fileTree, &catalog)
//
//			result := catalog.HasEntriesForAllFilesInTree(*fileTree)
//
//			if tc.expected != result {
//				t.Errorf("expected %t but got %t", tc.expected, result)
//			}
//		})
//	}
//}
