//go:build !windows
// +build !windows

package file

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/scylladb/go-set/strset"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	contents, err := io.ReadAll(fileReader)
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
				Path:            "/path/branch/two/file-2.txt",
				LinkDestination: "",
				UserID:          1337,
				GroupID:         5432,
				Type:            TypeRegular,
				MIMEType:        "application/octet-stream",
				FileInfo: ManualInfo{
					NameValue:    "file-2.txt",
					SizeValue:    12,
					ModeValue:    0x1ed,
					ModTimeValue: time.Date(2019, time.September, 16, 0, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:    "path/branch/two/",
			fixture: "fixture-1",
			expected: Metadata{
				Path:            "/path/branch/two",
				LinkDestination: "",
				UserID:          1337,
				GroupID:         5432,
				Type:            TypeDirectory,
				MIMEType:        "",
				FileInfo: ManualInfo{
					NameValue:    "two",
					SizeValue:    0,
					ModeValue:    0x800001ed,
					ModTimeValue: time.Date(2019, time.September, 16, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := getTarFixture(t, test.fixture)
			metadata, err := MetadataFromTar(f, test.name)
			assert.NoError(t, err)
			assertMetadataEqual(t, test.expected, metadata)
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

func Test_tarVisitor_visit(t *testing.T) {
	assertNoFilesInRoot := func(t testing.TB, fs afero.Fs) {
		t.Helper()

		allowableFiles := strset.New("tmp")

		// list all files in root
		files, err := afero.ReadDir(fs, "/")
		require.NoError(t, err)

		for _, f := range files {
			assert.True(t, allowableFiles.Has(f.Name()), "unexpected file in root: %s", f.Name())
		}
	}

	assertPaths := func(expectedFiles []string, expectedDirs []string) func(t testing.TB, fs afero.Fs) {
		return func(t testing.TB, fs afero.Fs) {
			t.Helper()

			sort.Strings(expectedFiles)
			haveFiles := strset.New()
			haveDirs := strset.New()
			err := afero.Walk(fs, "/", func(path string, info os.FileInfo, err error) error {
				require.NoError(t, err)
				if info.IsDir() {
					haveDirs.Add(path)
				} else {
					haveFiles.Add(path)
				}
				return nil
			})

			haveFilesList := haveFiles.List()
			sort.Strings(haveFilesList)

			haveDirsList := haveDirs.List()
			sort.Strings(haveDirsList)

			require.NoError(t, err)

			if d := cmp.Diff(expectedFiles, haveFilesList); d != "" {
				t.Errorf("unexpected files (-want +got):\n%s", d)
			}

			if d := cmp.Diff(expectedDirs, haveDirsList); d != "" {
				t.Errorf("unexpected dirs (-want +got):\n%s", d)
			}

		}
	}

	tests := []struct {
		name     string
		entry    TarFileEntry
		wantErr  require.ErrorAssertionFunc
		assertFs []func(t testing.TB, fs afero.Fs)
	}{
		{
			name: "regular file is written",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeReg,
					Name:     "file.txt",
					Linkname: "",
					Size:     2,
				},
				Reader: strings.NewReader("hi"),
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{"/tmp/file.txt"},
					[]string{"/", "/tmp"},
				),
			},
		},
		{
			name: "regular file with possible path traversal errors out",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeReg,
					Name:     "../file.txt",
					Linkname: "",
					Size:     2,
				},
				Reader: strings.NewReader("hi"),
			},
			wantErr: require.Error,
		},
		{
			name: "directory is created",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeDir,
					Name:     "dir",
					Linkname: "",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/", "/tmp", "/tmp/dir"},
				),
			},
		},
		{
			name: "symlink is ignored",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeSymlink,
					Name:     "symlink",
					Linkname: "./../to-location",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/"},
				),
			},
		},
		{
			name: "hardlink is ignored",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeLink,
					Name:     "link",
					Linkname: "./../to-location",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/"},
				),
			},
		},
		{
			name: "device is ignored",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeChar,
					Name:     "device",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/"},
				),
			},
		},
		{
			name: "block device is ignored",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeBlock,
					Name:     "device",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/"},
				),
			},
		},
		{
			name: "pipe is ignored",
			entry: TarFileEntry{
				Sequence: 0,
				Header: tar.Header{
					Typeflag: tar.TypeFifo,
					Name:     "pipe",
				},
			},
			assertFs: []func(t testing.TB, fs afero.Fs){
				assertPaths(
					[]string{},
					[]string{"/"},
				),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			v := tarVisitor{
				fs:          afero.NewMemMapFs(),
				destination: "/tmp",
			}
			err := v.visit(tt.entry)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			for _, fn := range tt.assertFs {
				fn(t, v.fs)
			}

			// even if the test has no other assertions, check that the root is empty
			assertNoFilesInRoot(t, v.fs)
		})
	}
}
