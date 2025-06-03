package filetree

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

func Test_searchContext_SearchByPath(t *testing.T) {
	type fields struct {
		tree  *FileTree
		index Index
	}
	type args struct {
		path    string
		options []LinkResolutionOption
	}

	tree := New()
	ref, err := tree.AddFile("/path/to/file.txt")
	require.NoError(t, err)
	require.NotNil(t, ref)

	idx := NewIndex()
	idx.Add(*ref, file.Metadata{MIMEType: "plain/text"})

	defaultFields := fields{
		tree:  tree,
		index: idx,
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *file.Resolution
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "path exists",
			fields: defaultFields,
			args: args{
				path: "/path/to/file.txt",
			},
			want: &file.Resolution{
				RequestPath: "/path/to/file.txt",
				Reference: &file.Reference{
					RealPath: "/path/to/file.txt",
				},
			},
		},
		{
			name:   "path does not exists",
			fields: defaultFields,
			args: args{
				path: "/NOT/path/to/file.txt",
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			i := searchContext{
				tree:  tt.fields.tree,
				index: tt.fields.index,
			}
			got, err := i.SearchByPath(tt.args.path, tt.args.options...)
			tt.wantErr(t, err, fmt.Sprintf("SearchByPath(%v, %v)", tt.args.path, tt.args.options))
			if err != nil {
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(file.Reference{}, "id"),
			}

			if d := cmp.Diff(tt.want, got, opts...); d != "" {
				t.Errorf("SearchByPath() mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func Test_searchContext_SearchByGlob(t *testing.T) {
	type fields struct {
		tree  *FileTree
		index Index
	}
	type args struct {
		glob    string
		options []LinkResolutionOption
	}

	tree := New()
	doubleLinkToPathRef, err := tree.AddSymLink("/double-link-to-path", "/link-to-path")
	require.NoError(t, err)
	require.NotNil(t, doubleLinkToPathRef)

	linkToPathRef, err := tree.AddSymLink("/link-to-path", "/path")
	require.NoError(t, err)
	require.NotNil(t, linkToPathRef)

	linkToFileRef, err := tree.AddSymLink("/link-to-file", "/path/to/file.txt")
	require.NoError(t, err)
	require.NotNil(t, linkToFileRef)

	fileRef, err := tree.AddFile("/path/to/file.txt")
	require.NoError(t, err)
	require.NotNil(t, fileRef)

	toRef, err := tree.AddDir("/path/to")
	require.NoError(t, err)
	require.NotNil(t, toRef)

	idx := NewIndex()
	idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
	idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymLink})
	idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymLink})
	idx.Add(*doubleLinkToPathRef, file.Metadata{Type: file.TypeSymLink})
	idx.Add(*toRef, file.Metadata{Type: file.TypeDirectory})

	defaultFields := fields{
		tree:  tree,
		index: idx,
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []file.Resolution
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "path exists",
			fields: defaultFields,
			args: args{
				glob: "/**/t?/fil?.txt",
			},
			// note: result "/link-to-file" resolves to the file but does not show up since the request path
			// does not match the requirement glob
			want: []file.Resolution{
				{

					RequestPath: "/path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "ancestor access path exists",
			fields: defaultFields,
			args: args{
				// note: this is a glob through a symlink (ancestor). If not using the index, this will work
				// just fine, since we do a full tree search. However, if using the index, this shortcut will
				// dodge any ancestor symlink and will not find the file.
				glob: "**/link-to-path/to/file.txt",
			},
			want: []file.Resolution{
				{
					RequestPath: "/link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "access all children",
			fields: defaultFields,
			args: args{
				glob: "**/path/to/*",
			},
			want: []file.Resolution{
				{
					RequestPath: "/path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "access all children as path",
			fields: defaultFields,
			args: args{
				glob: "/path/to/*",
			},
			want: []file.Resolution{
				{
					RequestPath: "/path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "access via symlink for all children",
			fields: defaultFields,
			args: args{
				glob: "**/link-to-path/to/*",
			},
			want: []file.Resolution{
				{
					RequestPath: "/link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "multi ancestor access path exists",
			fields: defaultFields,
			args: args{
				// note: this is a glob through a symlink (ancestor). If not using the index, this will work
				// just fine, since we do a full tree search. However, if using the index, this shortcut will
				// dodge any ancestor symlink and will not find the file.
				glob: "**/double-link-to-path/to/file.txt",
			},
			want: []file.Resolution{
				{
					RequestPath: "/double-link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "leaf access path exists",
			fields: defaultFields,
			args: args{
				glob: "**/link-to-file",
			},
			want: []file.Resolution{
				{
					RequestPath: "/link-to-file",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
					LinkResolutions: []file.Resolution{
						{
							RequestPath: "/link-to-file",
							Reference: &file.Reference{
								RealPath: "/link-to-file",
							},
						},
					},
				},
			},
		},
		{
			name:   "ancestor access path exists",
			fields: defaultFields,
			args: args{
				// note: this is a glob through a symlink (ancestor). If not using the index, this will work
				// just fine, since we do a full tree search. However, if using the index, this shortcut will
				// dodge any ancestor symlink and will not find the file.
				glob: "**/link-to-path/to/file.txt",
			},
			want: []file.Resolution{
				{
					RequestPath: "/link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "by extension",
			fields: defaultFields,
			args: args{
				// note: this is a glob through a symlink (ancestor). If not using the index, this will work
				// just fine, since we do a full tree search. However, if using the index, this shortcut will
				// dodge any ancestor symlink and will not find the file.
				glob: "**/*.txt",
			},
			want: []file.Resolution{
				{
					RequestPath: "/path/to/file.txt",
					Reference:   &file.Reference{RealPath: "/path/to/file.txt"},
				},
			},
		},
		{
			name:   "path does not exists",
			fields: defaultFields,
			args: args{
				glob: "/NOT/**/file",
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			sc := NewSearchContext(tt.fields.tree, tt.fields.index)
			got, err := sc.SearchByGlob(tt.args.glob, tt.args.options...)
			tt.wantErr(t, err, fmt.Sprintf("SearchByGlob(%v, %v)", tt.args.glob, tt.args.options))
			if err != nil {
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(file.Reference{}, "id"),
			}

			if d := cmp.Diff(tt.want, got, opts...); d != "" {
				t.Errorf("SearchByGlob() mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func Test_searchContext_SearchByMIMEType(t *testing.T) {
	type fields struct {
		tree  *FileTree
		index Index
	}
	type args struct {
		mimeTypes string
	}

	tree := New()
	ref, err := tree.AddFile("/path/to/file.txt")
	require.NoError(t, err)
	require.NotNil(t, ref)

	idx := NewIndex()
	idx.Add(*ref, file.Metadata{MIMEType: "plain/text"})

	defaultFields := fields{
		tree:  tree,
		index: idx,
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []file.Resolution
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "types exists",
			fields: defaultFields,
			args: args{
				mimeTypes: "plain/text",
			},
			want: []file.Resolution{
				{
					RequestPath: "/path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
			},
		},
		{
			name:   "types do not exists",
			fields: defaultFields,
			args: args{
				mimeTypes: "octetstream",
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			i := searchContext{
				tree:  tt.fields.tree,
				index: tt.fields.index,
			}
			got, err := i.SearchByMIMEType(tt.args.mimeTypes)
			tt.wantErr(t, err, fmt.Sprintf("SearchByMIMEType(%v)", tt.args.mimeTypes))
			if err != nil {
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(file.Reference{}, "id"),
			}

			if d := cmp.Diff(tt.want, got, opts...); d != "" {
				t.Errorf("SearchByMIMEType() mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func Test_complexSymlinkPerformance(t *testing.T) {
	tr := New()
	idx := NewIndex()

	var realPaths []string

	numPkgs := 30 // with this few packages, the allPathsToNode behavior would essentially hang

	for num := 0; num < numPkgs; num++ {
		// add a concrete path
		realPath := fmt.Sprintf("/pkgs/lib-%d/package.json", num)
		realPaths = append(realPaths, realPath)
		r, err := tr.AddFile(file.Path(realPath))
		require.NoError(t, err)
		require.NotNil(t, r)
		idx.Add(*r, file.Metadata{Type: file.TypeRegular})

		// add dependencies on all previous packages
		for dep := num + 1; dep < numPkgs-1; dep++ {
			r, err = tr.AddSymLink(file.Path(fmt.Sprintf("/pkgs/lib-%d/libs/lib-%d", num, dep)), file.Path(fmt.Sprintf("/pkgs/lib-%d", dep)))
			require.NoError(t, err)
			require.NotNil(t, r)
			idx.Add(*r, file.Metadata{Type: file.TypeSymLink})
		}
	}

	tests := []struct {
		glob     string
		expected []string
	}{
		{
			glob:     "**/package.json",
			expected: realPaths,
		},
	}

	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			sc := NewSearchContext(tr, idx)
			gotResolutions, err := sc.SearchByGlob(tt.glob, FollowBasenameLinks)
			require.NoError(t, err)
			require.NotNil(t, gotResolutions)
			var got []string
			for _, gotResolution := range gotResolutions {
				got = append(got, string(gotResolution.RealPath))
			}
			require.ElementsMatch(t, tt.expected, got)
		})
	}
}

func Test_nextSlash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		expected int
	}{
		{name: "single begin", input: "/a", start: 0, expected: 0},
		{name: "single after", input: "/ab1", start: 1, expected: 4},
		{name: "single end", input: "/ab1", start: 4, expected: -1},
		{name: "multiple first", input: "/a/b/c", start: 0, expected: 0},
		{name: "multiple mid", input: "/a/b/c", start: 1, expected: 2},
		{name: "multiple last", input: "/a/b/c", start: 3, expected: 4},
		{name: "multiple after", input: "/a/b/c", start: 5, expected: 6},
		{name: "multiple end", input: "/a/b/c", start: 6, expected: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextSlashOrEnd(tt.input, tt.start)
			if got < 0 {
				got = -1
			}
			require.Equal(t, tt.expected, got)
		})
	}
}
