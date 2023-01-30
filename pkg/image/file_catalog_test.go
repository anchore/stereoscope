//go:build !windows
// +build !windows

package image

import (
	"crypto/sha256"
	"fmt"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-test/deep"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/anchore/stereoscope/pkg/file"
)

const (
	fixturesPath = "test-fixtures"
)

var (
	fixturesGeneratorsPath = path.Join(fixturesPath, "generators")
	tarCachePath           = path.Join(fixturesPath, "tar-cache")
)

func TestFileCatalog_Add(t *testing.T) {
	ref := file.NewFileReference("/somepath")

	metadata := file.Metadata{
		Path:            "a",
		LinkDestination: "c",
		Size:            1,
		UserID:          2,
		GroupID:         3,
		Type:            4,
		IsDir:           true,
		Mode:            5,
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
	catalog.Add(*ref, metadata, layer, nil)

	expected := filetree.IndexEntry{
		Reference: *ref,
		Metadata:  metadata,
	}

	actual, err := catalog.Get(*ref)
	if err != nil {
		t.Fatalf("could not get by ref: %+v", err)
	}

	for d := range deep.Equal(expected, actual) {
		t.Errorf("diff: %+v", d)
	}

	assert.Equal(t, layer, catalog.Layer(*ref))
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
	fixtureFile := getTarFixture(t, "fixture-1")

	// a real path & contents from the fixture
	p := "path/branch/one/file-1.txt"
	ref := file.NewFileReference(file.Path(p))
	expected := "first file\n"

	metadata := file.Metadata{
		Path: p,
	}

	tr, err := file.NewTarIndex(fixtureFile.Name(), nil)
	require.NoError(t, err)

	layer := &Layer{
		layer:          &testLayerContent{},
		indexedContent: tr,
	}

	entries, err := tr.EntriesByName(p)
	require.NoError(t, err)

	require.Len(t, entries, 1)

	opener := func() io.ReadCloser {
		return io.NopCloser(entries[0].Reader)
	}

	catalog := NewFileCatalog()
	catalog.Add(*ref, metadata, layer, opener)

	reader, err := catalog.FileContents(*ref)
	require.NoError(t, err)

	actual, err := io.ReadAll(reader)
	require.NoError(t, err)

	for _, d := range deep.Equal([]byte(expected), actual) {
		t.Errorf("diff: %+v", d)
	}
}

func Test_fileExtensions(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "empty",
			path: "",
		},
		{
			name: "directory",
			path: "/somewhere/to/nowhere/",
		},
		{
			name: "directory with ext",
			path: "/somewhere/to/nowhere.d/",
		},
		{
			name: "single extension",
			path: "/somewhere/to/my.tar",
			want: []string{".tar"},
		},
		{
			name: "multiple extensions",
			path: "/somewhere/to/my.tar.gz",
			want: []string{".gz", ".tar.gz"},
		},
		{
			name: "ignore . prefix",
			path: "/somewhere/to/.my.tar.gz",
			want: []string{".gz", ".tar.gz"},
		},
		{
			name: "ignore more . prefixes",
			path: "/somewhere/to/...my.tar.gz",
			want: []string{".gz", ".tar.gz"},
		},
		{
			name: "ignore . suffixes",
			path: "/somewhere/to/my.tar.gz...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fileExtensions(tt.path))
		})
	}
}

func TestFileCatalog_GetByExtension(t *testing.T) {
	fixtureTarFile := getTarFixture(t, "fixture-2")

	ft := filetree.NewFileTree()
	fileCatalog := NewFileCatalog()
	var size int64

	// we don't need the index itself, just the side effect on the file catalog after indexing
	_, err := file.NewTarIndex(
		fixtureTarFile.Name(),
		layerTarIndexer(ft, fileCatalog, &size, nil, nil),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    []filetree.IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get simple extension",
			input: ".txt",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-1.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-1.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{

					Reference: file.Reference{RealPath: "/path/branch.d/two/file-2.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/two/file-2.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/file-3.txt"},
					Metadata: file.Metadata{
						Path:     "/path/file-3.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get mixed type extension",
			input: ".d",
			want: []filetree.IndexEntry{
				{

					Reference: file.Reference{RealPath: "/path/branch.d"},
					Metadata: file.Metadata{
						Path:  "/path/branch.d",
						Type:  file.TypeDir,
						IsDir: true,
					},
				},
				{

					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.d"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.d",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},

				{

					Reference: file.Reference{RealPath: "/path/common/branch.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/branch.d",
						LinkDestination: "path/branch.d",
						Type:            file.TypeSymlink,
					},
				},
				{

					Reference: file.Reference{RealPath: "/path/common/file-1.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/file-1.d",
						LinkDestination: "path/branch.d/one/file-1.txt",
						Type:            file.TypeSymlink,
					},
				},
			},
		},
		{
			name:  "get long extension",
			input: ".tar.gz",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get short extension",
			input: ".gz",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get non-existent extension",
			input: ".blerg-123",
			want:  []filetree.IndexEntry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileCatalog.GetByExtension(tt.input)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			if d := cmp.Diff(tt.want, actual,
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreUnexported(file.Reference{}),
				cmpopts.IgnoreFields(file.Metadata{}, "Mode", "GroupID", "UserID", "Size"),
			); d != "" {
				t.Errorf("diff: %s", d)
			}
		})
	}
}

func TestFileCatalog_GetByBasename(t *testing.T) {
	fixtureTarFile := getTarFixture(t, "fixture-2")

	ft := filetree.NewFileTree()
	fileCatalog := NewFileCatalog()
	var size int64

	// we don't need the index itself, just the side effect on the file catalog after indexing
	_, err := file.NewTarIndex(
		fixtureTarFile.Name(),
		layerTarIndexer(ft, fileCatalog, &size, nil, nil),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    []filetree.IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file name",
			input: "file-1.txt",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-1.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-1.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get non-existing name",
			input: "file-11.txt",
			want:  []filetree.IndexEntry{},
		},
		{
			name:  "get directory name",
			input: "branch.d",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d"},
					Metadata: file.Metadata{
						Path:  "/path/branch.d",
						Type:  file.TypeDir,
						IsDir: true,
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/common/branch.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/branch.d",
						LinkDestination: "path/branch.d",
						Type:            file.TypeSymlink,
					},
				},
			},
		},
		{
			name:  "get symlink name",
			input: "file-1.d",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/common/file-1.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/file-1.d",
						LinkDestination: "path/branch.d/one/file-1.txt",
						Type:            file.TypeSymlink,
					},
				},
			},
		},
		{
			name:    "get basename with path expression",
			input:   "somewhere/file-1.d",
			wantErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileCatalog.GetByBasename(tt.input)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			if d := cmp.Diff(tt.want, actual,
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreUnexported(file.Reference{}),
				cmpopts.IgnoreFields(file.Metadata{}, "Mode", "GroupID", "UserID", "Size"),
			); d != "" {
				t.Errorf("diff: %s", d)
			}
		})
	}
}

func TestFileCatalog_GetByBasenameGlob(t *testing.T) {
	fixtureTarFile := getTarFixture(t, "fixture-2")

	ft := filetree.NewFileTree()
	fileCatalog := NewFileCatalog()
	var size int64

	// we don't need the index itself, just the side effect on the file catalog after indexing
	_, err := file.NewTarIndex(
		fixtureTarFile.Name(),
		layerTarIndexer(ft, fileCatalog, &size, nil, nil),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    []filetree.IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file name",
			input: "file-1.*",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/common/file-1.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/file-1.d",
						LinkDestination: "path/branch.d/one/file-1.txt",
						Type:            file.TypeSymlink,
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-1.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-1.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get non-existing name",
			input: "blerg-*.txt",
			want:  []filetree.IndexEntry{},
		},
		{
			name:  "get directory name",
			input: "bran*.d",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d"},
					Metadata: file.Metadata{
						Path:  "/path/branch.d",
						Type:  file.TypeDir,
						IsDir: true,
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/common/branch.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/branch.d",
						LinkDestination: "path/branch.d",
						Type:            file.TypeSymlink,
					},
				},
			},
		},
		{
			name:  "get symlink name",
			input: "file?1.d",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/common/file-1.d"},
					Metadata: file.Metadata{
						Path:            "/path/common/file-1.d",
						LinkDestination: "path/branch.d/one/file-1.txt",
						Type:            file.TypeSymlink,
					},
				},
			},
		},
		{
			name:    "get basename with path expression",
			input:   "somewhere/file?1.d",
			wantErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileCatalog.GetByBasenameGlob(tt.input)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			if d := cmp.Diff(tt.want, actual,
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreUnexported(file.Reference{}),
				cmpopts.IgnoreFields(file.Metadata{}, "Mode", "GroupID", "UserID", "Size"),
			); d != "" {
				t.Errorf("diff: %s", d)
			}
		})
	}
}

func TestFileCatalog_GetByMimeType(t *testing.T) {
	fixtureTarFile := getTarFixture(t, "fixture-2")

	ft := filetree.NewFileTree()
	fileCatalog := NewFileCatalog()
	var size int64

	// we don't need the index itself, just the side effect on the file catalog after indexing
	_, err := file.NewTarIndex(
		fixtureTarFile.Name(),
		layerTarIndexer(ft, fileCatalog, &size, nil, nil),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		input   string
		want    []filetree.IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file mimetype",
			input: "text/plain",
			want: []filetree.IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-1.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-1.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.d"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.d",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/two/file-2.txt"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/two/file-2.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/file-3.txt"},
					Metadata: file.Metadata{
						Path:     "/path/file-3.txt",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get non-existing mimetype",
			input: "text/bogus",
			want:  []filetree.IndexEntry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileCatalog.GetByMIMEType(tt.input)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			if d := cmp.Diff(tt.want, actual,
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreUnexported(file.Reference{}),
				cmpopts.IgnoreFields(file.Metadata{}, "Mode", "GroupID", "UserID", "Size"),
			); d != "" {
				t.Errorf("diff: %s", d)
			}
		})
	}
}

func TestFileCatalog_GetBasenames(t *testing.T) {
	fixtureTarFile := getTarFixture(t, "fixture-2")

	ft := filetree.NewFileTree()
	fileCatalog := NewFileCatalog()
	var size int64

	// we don't need the index itself, just the side effect on the file catalog after indexing
	_, err := file.NewTarIndex(
		fixtureTarFile.Name(),
		layerTarIndexer(ft, fileCatalog, &size, nil, nil),
	)
	require.NoError(t, err)

	tests := []struct {
		name string
		want []string
	}{
		{
			name: "go case",
			want: []string{
				".file-4.tar.gz",
				"branch",
				"branch.d",
				"common",
				"file-1.d",
				"file-1.txt",
				"file-2.txt",
				"file-3.txt",
				"file-4",
				"file-4.d",
				"file-4.tar.gz",
				"one",
				"path",
				"two",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := fileCatalog.Basenames()
			assert.ElementsMatchf(t, tt.want, actual, "diff: %s", cmp.Diff(tt.want, actual))
		})
	}
}

func getTarFixture(t *testing.T, name string) *os.File {
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

	t.Cleanup(func() {
		require.NoError(t, fh.Close())
	})

	return fh
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

func fileExtensions(p string) []string {
	var exts []string
	p = strings.TrimSpace(p)

	// ignore oddities
	if strings.HasSuffix(p, ".") {
		return exts
	}

	// ignore directories
	if strings.HasSuffix(p, "/") {
		return exts
	}

	// ignore . which indicate a hidden file
	p = strings.TrimLeft(path.Base(p), ".")
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '.' {
			exts = append(exts, p[i:])
		}
	}
	return exts
}
