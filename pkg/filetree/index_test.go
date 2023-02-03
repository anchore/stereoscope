//go:build !windows
// +build !windows

package filetree

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
)

func commonIndexFixture(t *testing.T) Index {
	t.Helper()

	tree := New()
	idx := NewIndex()

	addDir := func(path file.Path) {
		ref, err := tree.AddDir(path)
		require.NoError(t, err, "failed to add DIR reference to index")
		require.NotNil(t, ref, "failed to add DIR reference to index (nil ref")
		idx.Add(*ref, file.Metadata{Path: string(path), Type: file.TypeDir, IsDir: true})
	}

	addFile := func(path file.Path) {
		ref, err := tree.AddFile(path)
		require.NoError(t, err, "failed to add FILE reference to index")
		require.NotNil(t, ref, "failed to add FILE reference to index (nil ref")
		idx.Add(*ref, file.Metadata{Path: string(path), Type: file.TypeReg, MIMEType: "text/plain"})
	}

	addLink := func(from, to file.Path) {
		ref, err := tree.AddSymLink(from, to)
		require.NoError(t, err, "failed to add LINK reference to index")
		require.NotNil(t, ref, "failed to add LINK reference to index (nil ref")
		idx.Add(*ref, file.Metadata{Path: string(from), LinkDestination: string(to), Type: file.TypeSymlink})
	}

	//  mkdir -p path/branch.d/one
	//  mkdir -p path/branch.d/two
	//  mkdir -p path/common

	// note: we need to add all paths explicitly to the index
	addDir("/path")
	addDir("/path/branch.d")
	addDir("/path/branch.d/one")
	addDir("/path/branch.d/two")
	addDir("/path/common")

	//  echo "first file" > path/branch.d/one/file-1.txt
	//  echo "forth file" > path/branch.d/one/file-4.d
	//  echo "multi ext file" > path/branch.d/one/file-4.tar.gz
	//  echo "hidden file" > path/branch.d/one/.file-4.tar.gz

	addFile("/path/branch.d/one/file-1.txt")
	addFile("/path/branch.d/one/file-4.d")
	addFile("/path/branch.d/one/file-4.tar.gz")
	addFile("/path/branch.d/one/.file-4.tar.gz")

	//  ln -s path/branch.d path/common/branch.d
	//  ln -s path/branch.d path/common/branch
	//  ln -s path/branch.d/one/file-4.d path/common/file-4
	//  ln -s path/branch.d/one/file-1.txt path/common/file-1.d

	addLink("/path/common/branch.d", "path/branch.d")
	addLink("/path/common/branch", "path/branch.d")
	addLink("/path/common/file-4", "path/branch.d/one/file-4.d")
	addLink("/path/common/file-1.d", "path/branch.d/one/file-1.txt")

	//  echo "second file" > path/branch.d/two/file-2.txt
	//  echo "third file" > path/file-3.txt

	addFile("/path/branch.d/two/file-2.txt")
	addFile("/path/file-3.txt")

	return idx
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
	fileIndex := commonIndexFixture(t)

	tests := []struct {
		name    string
		input   string
		want    []IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get simple extension",
			input: ".txt",
			want: []IndexEntry{
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
			want: []IndexEntry{
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
			want: []IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get short extension",
			input: ".gz",
			want: []IndexEntry{
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
				{
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
						Type:     file.TypeReg,
						MIMEType: "text/plain",
					},
				},
			},
		},
		{
			name:  "get non-existent extension",
			input: ".blerg-123",
			want:  []IndexEntry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileIndex.GetByExtension(tt.input)
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
	fileIndex := commonIndexFixture(t)

	tests := []struct {
		name    string
		input   string
		want    []IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file name",
			input: "file-1.txt",
			want: []IndexEntry{
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
			want:  []IndexEntry{},
		},
		{
			name:  "get directory name",
			input: "branch.d",
			want: []IndexEntry{
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
			want: []IndexEntry{
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
			actual, err := fileIndex.GetByBasename(tt.input)
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
	fileIndex := commonIndexFixture(t)

	tests := []struct {
		name    string
		input   string
		want    []IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file name",
			input: "file-1.*",
			want: []IndexEntry{
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
			want:  []IndexEntry{},
		},
		{
			name:  "get directory name",
			input: "bran*.d",
			want: []IndexEntry{
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
			want: []IndexEntry{
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
			actual, err := fileIndex.GetByBasenameGlob(tt.input)
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
	fileIndex := commonIndexFixture(t)

	tests := []struct {
		name    string
		input   string
		want    []IndexEntry
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "get existing file mimetype",
			input: "text/plain",
			want: []IndexEntry{
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
					Reference: file.Reference{RealPath: "/path/branch.d/one/.file-4.tar.gz"},
					Metadata: file.Metadata{
						Path:     "/path/branch.d/one/.file-4.tar.gz",
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
			want:  []IndexEntry{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			actual, err := fileIndex.GetByMIMEType(tt.input)
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
	fileIndex := commonIndexFixture(t)

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
			actual := fileIndex.Basenames()
			assert.ElementsMatchf(t, tt.want, actual, "diff: %s", cmp.Diff(tt.want, actual))
		})
	}
}
