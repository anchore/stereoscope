package filetree

import (
	"fmt"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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

	tree := NewFileTree()
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
		want    *file.ReferenceAccessVia
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "path exists",
			fields: defaultFields,
			args: args{
				path: "/path/to/file.txt",
			},
			want: &file.ReferenceAccessVia{
				ReferenceAccess: file.ReferenceAccess{
					RequestPath: "/path/to/file.txt",
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

	tree := NewFileTree()
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

	idx := NewIndex()
	idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
	idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymlink})
	idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymlink})
	idx.Add(*doubleLinkToPathRef, file.Metadata{Type: file.TypeSymlink})

	defaultFields := fields{
		tree:  tree,
		index: idx,
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []file.ReferenceAccessVia
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "path exists",
			fields: defaultFields,
			args: args{
				glob: "/**/t?/fil?.txt",
			},
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/double-link-to-path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
					},
				},
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/link-to-file",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
					},
					LeafLinkResolution: []file.ReferenceAccess{
						{
							RequestPath: "/link-to-file",
							Reference: &file.Reference{
								RealPath: "/link-to-file",
							},
						},
					},
				},
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/link-to-path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
					},
				},
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
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
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/link-to-path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
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
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/double-link-to-path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
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
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/link-to-file",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
					},
					LeafLinkResolution: []file.ReferenceAccess{
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
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/link-to-path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
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

	tree := NewFileTree()
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
		want    []file.ReferenceAccessVia
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:   "types exists",
			fields: defaultFields,
			args: args{
				mimeTypes: "plain/text",
			},
			want: []file.ReferenceAccessVia{
				{
					ReferenceAccess: file.ReferenceAccess{
						RequestPath: "/path/to/file.txt",
						Reference: &file.Reference{
							RealPath: "/path/to/file.txt",
						},
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
			name: "dead symlink",
			want: []file.Path{
				"/path/to/file.txt",
			},
			input: func() input {
				tree := NewFileTree()

				deafLinkRef, err := tree.AddSymLink("/link-to-file", "/path/to/dead/file.txt")
				require.NoError(t, err)
				require.NotNil(t, deafLinkRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*deafLinkRef, file.Metadata{Type: file.TypeSymlink})

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
			name:    "symlink triangle cycle",
			wantErr: require.Error,
			input: func() input {
				tree := NewFileTree()

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
				idx.Add(*link1, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*link2, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*link3, file.Metadata{Type: file.TypeSymlink})

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
			name: "single leaf symlink",
			want: []file.Path{
				"/link-to-file",
				"/path/to/file.txt",
			},
			input: func() input {
				tree := NewFileTree()

				linkToFileRef, err := tree.AddSymLink("/link-to-file", "/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, linkToFileRef)

				fileRef, err := tree.AddFile("/path/to/file.txt")
				require.NoError(t, err)
				require.NotNil(t, fileRef)

				idx := NewIndex()
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymlink})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToFileRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*doubleLinkToFileRef, file.Metadata{Type: file.TypeSymlink})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDir})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDir})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*anotherLinkToPathRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*anotherLinkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDir})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToAnotherViaLinkRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*pathToLinkToFileRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDir})
				idx.Add(*dirAnother, file.Metadata{Type: file.TypeDir})

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
				tree := NewFileTree()

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
				idx.Add(*fileRef, file.Metadata{MIMEType: "plain/text", Type: file.TypeReg})
				idx.Add(*linkToAnotherViaLinkRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToPathRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*linkToToRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*pathToLinkToFileRef, file.Metadata{Type: file.TypeSymlink})
				idx.Add(*dirTo, file.Metadata{Type: file.TypeDir})
				idx.Add(*dirAnother, file.Metadata{Type: file.TypeDir})

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
			assert.ElementsMatchf(t, tt.want, got, cmp.Diff(tt.want, got))
		})
	}
}
