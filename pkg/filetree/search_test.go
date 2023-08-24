package filetree

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
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
				{

					RequestPath: "/double-link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
				{

					RequestPath: "/link-to-path/to/file.txt",
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
				{
					RequestPath: "/double-link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
				},
				// note: this is NOT expected since the input glob does not match against the request path
				//{
				//	Resolution: file.Resolution{
				//		RequestPath: "/link-to-file",
				//		Reference: &file.Reference{
				//			RealPath: "/path/to/file.txt",
				//		},
				//	},
				//	LinkResolutions: []file.Resolution{
				//		{
				//			RequestPath: "/link-to-file",
				//			Reference:   &file.Reference{RealPath: "/link-to-file"},
				//		},
				//	},
				//},
				{
					RequestPath: "/link-to-path/to/file.txt",
					Reference: &file.Reference{
						RealPath: "/path/to/file.txt",
					},
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

			expected, err := tt.fields.tree.FilesByGlob(tt.args.glob, tt.args.options...)
			require.NoError(t, err)

			if d := cmp.Diff(expected, got, opts...); d != "" {
				t.Errorf("Difference relative to tree results mismatch (-want +got):\n%s", d)
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

func Test_searchContext_allPathsToNode(t *testing.T) {
	type input struct {
		query *filenode.FileNode
		sc    *searchContext
	}

	tests := []struct {
		name    string
		input   input
		want    []file.Path
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "simple dir",
			want: []file.Path{
				"/path/to",
			},
			input: func() input {
				tree := New()

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})

				na, err := tree.node("/path/to", linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equal(t, file.Path("/path/to"), na.FileNode.RealPath)

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "dead symlink",
			want: []file.Path{
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				deafLinkRef, err := tree.AddSymLink("/link-to-file", "/path/to/dead/file.txt")
				require.NoError(t, err)
				require.NotNil(t, deafLinkRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*deafLinkRef, file.Metadata{Type: file.TypeSymLink})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "symlink triangle cycle",
			want: []file.Path{
				"/1",
				"/2",
				"/3",
			},
			input: func() input {
				tree := New()

				link1, err := tree.AddSymLink("/1", "/2")
				require.NoError(t, err)
				require.NotNil(t, link1)

				link2, err := tree.AddSymLink("/2", "/3")
				require.NoError(t, err)
				require.NotNil(t, link2)

				link3, err := tree.AddSymLink("/3", "/1")
				require.NoError(t, err)
				require.NotNil(t, link3)

				idx := NewIndex()
				idx.Add(*link1, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*link2, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*link3, file.Metadata{Type: file.TypeSymLink})

				na, err := tree.node(link1.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, link1.ID(), na.FileNode.Reference.ID(), "query node should be the same as the first link")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			// note: this isn't a real link cycle, but it does look like one while resolving from a leaf to the root
			name: "reverse symlink cycle",
			want: []file.Path{
				"/bin/ttyd",
				"/usr/bin/X11/ttyd",
				"/usr/bin/ttyd",
			},
			input: func() input {
				tree := New()

				usrRef, err := tree.AddDir("/usr")
				require.NoError(t, err)
				require.NotNil(t, usrRef)

				usrBinRef, err := tree.AddDir("/usr/bin")
				require.NoError(t, err)
				require.NotNil(t, usrBinRef)

				ttydRef, err := tree.AddFile("/usr/bin/ttyd")
				require.NoError(t, err)
				require.NotNil(t, ttydRef)

				binLinkRef, err := tree.AddSymLink("/bin", "usr/bin")
				require.NoError(t, err)
				require.NotNil(t, binLinkRef)

				x11LinkRef, err := tree.AddSymLink("/usr/bin/X11", ".")
				require.NoError(t, err)
				require.NotNil(t, x11LinkRef)

				idx := NewIndex()
				idx.Add(*usrRef, file.Metadata{Type: file.TypeDirectory})
				idx.Add(*usrBinRef, file.Metadata{Type: file.TypeDirectory})
				idx.Add(*binLinkRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*x11LinkRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*ttydRef, file.Metadata{Type: file.TypeRegular})

				na, err := tree.node(ttydRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, ttydRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as usr/bin/ttyd binary")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "single leaf symlink",
			want: []file.Path{
				"/link-to-file",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				linkToFileRef, err := tree.AddSymLink("/link-to-file", "/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, linkToFileRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymLink})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "2 deep leaf symlink",
			want: []file.Path{
				"/double-link-to-file",
				"/link-to-file",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				doubleLinkToFileRef, err := tree.AddSymLink("/double-link-to-file", "/link-to-file")
				require.NoError(t, err)
				require.NotNil(t, doubleLinkToFileRef)

				linkToFileRef, err := tree.AddSymLink("/link-to-file", "/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, linkToFileRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*doubleLinkToFileRef, file.Metadata{Type: file.TypeSymLink})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "single ancestor symlink",
			want: []file.Path{
				"/link-to-to/file.txt",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				dirTo, err := tree.AddDir("/path/to")
				require.NoError(t, err)
				require.NotNil(t, dirTo)

				linkToToRef, err := tree.AddSymLink("/link-to-to", "/path/to")
				require.NoError(t, err)
				require.NotNil(t, linkToToRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDirectory})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "2 deep, single sibling ancestor symlink",
			want: []file.Path{
				"/link-to-path/to/file.txt",
				"/link-to-to/file.txt",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				dirTo, err := tree.AddDir("/path/to")
				require.NoError(t, err)
				require.NotNil(t, dirTo)

				linkToPathRef, err := tree.AddSymLink("/link-to-path", "/path")
				require.NoError(t, err)
				require.NotNil(t, linkToPathRef)

				linkToToRef, err := tree.AddSymLink("/link-to-to", "/path/to")
				require.NoError(t, err)
				require.NotNil(t, linkToToRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDirectory})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "2 deep, multiple sibling ancestor symlink",
			want: []file.Path{
				"/another-link-to-path/to/file.txt",
				"/another-link-to-to/file.txt",
				"/link-to-path/to/file.txt",
				"/link-to-to/file.txt",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := New()

				dirTo, err := tree.AddDir("/path/to")
				require.NoError(t, err)
				require.NotNil(t, dirTo)

				linkToPathRef, err := tree.AddSymLink("/link-to-path", "/path")
				require.NoError(t, err)
				require.NotNil(t, linkToPathRef)

				anotherLinkToPathRef, err := tree.AddSymLink("/another-link-to-path", "/path")
				require.NoError(t, err)
				require.NotNil(t, anotherLinkToPathRef)

				linkToToRef, err := tree.AddSymLink("/link-to-to", "/path/to")
				require.NoError(t, err)
				require.NotNil(t, linkToToRef)

				anotherLinkToToRef, err := tree.AddSymLink("/another-link-to-to", "/path/to")
				require.NoError(t, err)
				require.NotNil(t, anotherLinkToToRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*anotherLinkToPathRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*anotherLinkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDirectory})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "2 deep, multiple nested ancestor symlink",
			want: []file.Path{
				"/link-to-path/link-to-another/file.txt",
				"/link-to-path/to/another/file.txt",
				"/link-to-path/to/link-to-file",
				"/link-to-to/another/file.txt",
				"/link-to-to/link-to-file",
				"/path/link-to-another/file.txt",
				"/path/to/another/file.txt",
				"/path/to/link-to-file",
			},
			input: func() input {
				tree := New()

				linkToAnotherViaLinkRef, err := tree.AddSymLink("/path/link-to-another", "/link-to-to/another")
				require.NoError(t, err)
				require.NotNil(t, linkToAnotherViaLinkRef)

				linkToPathRef, err := tree.AddSymLink("/link-to-path", "/path")
				require.NoError(t, err)
				require.NotNil(t, linkToPathRef)

				linkToToRef, err := tree.AddSymLink("/link-to-to", "/path/to")
				require.NoError(t, err)
				require.NotNil(t, linkToToRef)

				pathToLinkToFileRef, err := tree.AddSymLink("/path/to/link-to-file", "/path/to/another/file.txt")
				require.NoError(t, err)
				require.NotNil(t, pathToLinkToFileRef)

				dirTo, err := tree.AddDir("/path/to")
				require.NoError(t, err)
				require.NotNil(t, dirTo)

				dirAnother, err := tree.AddDir("/path/to/another")
				require.NoError(t, err)
				require.NotNil(t, dirAnother)

				fileRef, err := tree.AddFile("/path/to/another/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToAnotherViaLinkRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*pathToLinkToFileRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDirectory})
				idx.Add(*dirAnother, file.Metadata{Type: file.TypeDirectory})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
		{
			name: "relative, 2 deep, multiple nested ancestor symlink",
			want: []file.Path{
				"/link-to-path/link-to-another/file.txt",
				"/link-to-path/to/another/file.txt",
				"/link-to-path/to/link-to-file",
				"/link-to-to/another/file.txt",
				"/link-to-to/link-to-file",
				"/path/link-to-another/file.txt",
				"/path/to/another/file.txt",
				"/path/to/link-to-file",
			},
			input: func() input {
				tree := New()

				linkToAnotherViaLinkRef, err := tree.AddSymLink("/path/link-to-another", "../link-to-to/another")
				require.NoError(t, err)
				require.NotNil(t, linkToAnotherViaLinkRef)

				linkToPathRef, err := tree.AddSymLink("/link-to-path", "./path")
				require.NoError(t, err)
				require.NotNil(t, linkToPathRef)

				linkToToRef, err := tree.AddSymLink("/link-to-to", "./path/to")
				require.NoError(t, err)
				require.NotNil(t, linkToToRef)

				pathToLinkToFileRef, err := tree.AddSymLink("/path/to/link-to-file", "../to/another/file.txt")
				require.NoError(t, err)
				require.NotNil(t, pathToLinkToFileRef)

				dirTo, err := tree.AddDir("/path/to")
				require.NoError(t, err)
				require.NotNil(t, dirTo)

				dirAnother, err := tree.AddDir("/path/to/another")
				require.NoError(t, err)
				require.NotNil(t, dirAnother)

				fileRef, err := tree.AddFile("/path/to/another/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeRegular})
				idx.Add(*linkToAnotherViaLinkRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*pathToLinkToFileRef, file.Metadata{Type: file.TypeSymLink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDirectory})
				idx.Add(*dirAnother, file.Metadata{Type: file.TypeDirectory})

				na, err := tree.node(fileRef.RealPath, linkResolutionStrategy{
					FollowAncestorLinks:          false,
					FollowBasenameLinks:          false,
					DoNotFollowDeadBasenameLinks: false,
				})
				require.NoError(t, err)
				require.NotNil(t, na)
				require.NotNil(t, na.FileNode)
				require.Equalf(t, fileRef.ID(), na.FileNode.Reference.ID(), "query node should be the same as the file node")

				return input{
					query: na.FileNode,
					sc:    NewSearchContext(tree, idx).(*searchContext),
				}
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}

			got, err := tt.input.sc.allPathsToNode(tt.input.query)
			tt.wantErr(t, err, fmt.Sprintf("allPathsToNode(%v)", tt.input.query))
			if err != nil {
				return
			}

			assert.ElementsMatchf(t, tt.want, got, cmp.Diff(tt.want, got), "expected and actual paths should match")
		})
	}
}
