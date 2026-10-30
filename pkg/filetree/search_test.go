package filetree

import (
	"fmt"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
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

func Test_searchContext_SearchByMIMEType_SkipsLinkCycles(t *testing.T) {
	tree := New()
	idx := NewIndex()
	const mimeType = "application/x-executable"

	for _, p := range []file.Path{
		"/usr/bin/a-before",
		"/usr/bin/zz-after",
	} {
		ref, err := tree.AddFile(p)
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType})
	}

	for path, destination := range map[file.Path]file.Path{
		"/usr/bin/xz":    "/usr/bin/xzcat",
		"/usr/bin/xzcat": "/usr/bin/xz",
	} {
		ref, err := tree.AddSymLink(path, destination)
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType, Type: file.TypeSymLink})
	}

	results, err := NewSearchContext(tree, idx).SearchByMIMEType(mimeType)
	require.NoError(t, err)
	require.Len(t, results, 2)

	require.ElementsMatch(t, []string{
		"/usr/bin/a-before",
		"/usr/bin/zz-after",
	}, realPaths(results))
}

// layeredSearchFixture models the same path written by several layers against an image-wide index:
//   - lower: /app is a directory and /app/run.sh is a script
//   - upper: /app is written again and /app/run.sh is overwritten with a binary
//   - relinked: /app/run.sh is replaced with a symlink to /app/real.sh
type layeredSearchFixture struct {
	idx                        Index
	lower, upper               *FileTree
	squashed, squashedRelinked *FileTree
	scriptID, binID, realID    file.ID
}

func newLayeredSearchFixture(t *testing.T) layeredSearchFixture {
	t.Helper()
	idx := NewIndex()

	addDir := func(tr *FileTree, p file.Path) {
		ref, err := tr.AddDir(p)
		require.NoError(t, err)
		idx.Add(*ref, file.Metadata{Path: string(p), Type: file.TypeDirectory})
	}

	lower := New()
	addDir(lower, "/app")
	scriptRef, err := lower.AddFile("/app/run.sh")
	require.NoError(t, err)
	idx.Add(*scriptRef, file.Metadata{Type: file.TypeRegular, MIMEType: "text/x-shellscript"})

	upper := New()
	addDir(upper, "/app")
	binRef, err := upper.AddFile("/app/run.sh")
	require.NoError(t, err)
	idx.Add(*binRef, file.Metadata{Type: file.TypeRegular, MIMEType: "application/x-executable"})

	relinked := New()
	linkRef, err := relinked.AddSymLink("/app/run.sh", "/app/real.sh")
	require.NoError(t, err)
	idx.Add(*linkRef, file.Metadata{Type: file.TypeSymLink})
	realRef, err := relinked.AddFile("/app/real.sh")
	require.NoError(t, err)
	idx.Add(*realRef, file.Metadata{Type: file.TypeRegular, MIMEType: "application/x-executable"})

	squash := func(layers ...*FileTree) *FileTree {
		tr, err := layers[0].Copy()
		require.NoError(t, err)
		for _, l := range layers[1:] {
			require.NoError(t, tr.Merge(l))
		}
		return tr.(*FileTree)
	}

	return layeredSearchFixture{
		idx:              idx,
		lower:            lower,
		upper:            upper,
		squashed:         squash(lower, upper),
		squashedRelinked: squash(lower, relinked),
		scriptID:         scriptRef.ID(),
		binID:            binRef.ID(),
		realID:           realRef.ID(),
	}
}

type searchHit struct {
	RequestPath file.Path
	ID          file.ID
}

func searchHits(refs []file.Resolution) []searchHit {
	var hits []searchHit
	for _, r := range refs {
		hits = append(hits, searchHit{RequestPath: r.RequestPath, ID: r.ID()})
	}
	return hits
}

func Test_searchContext_SearchByMIMEType_layeredPaths(t *testing.T) {
	f := newLayeredSearchFixture(t)

	tests := []struct {
		name     string
		tree     Reader
		mimeType []string
		want     []searchHit
	}{
		{
			name:     "squashed: shadowed lower layer type does not match",
			tree:     f.squashed,
			mimeType: []string{"text/x-shellscript"},
		},
		{
			name:     "squashed: live top layer type matches",
			tree:     f.squashed,
			mimeType: []string{"application/x-executable"},
			want:     []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			// the stale lower layer entry is returned by the index first, the live one must still be found
			name:     "squashed: stale entry ahead of live entry for the same path",
			tree:     f.squashed,
			mimeType: []string{"text/x-shellscript", "application/x-executable"},
			want:     []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			name:     "squashed: repeated type does not duplicate results",
			tree:     f.squashed,
			mimeType: []string{"application/x-executable", "application/x-executable"},
			want:     []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			name:     "lower layer: its own type matches",
			tree:     f.lower,
			mimeType: []string{"text/x-shellscript"},
			want:     []searchHit{{"/app/run.sh", f.scriptID}},
		},
		{
			name:     "upper layer: lower layer type does not match",
			tree:     f.upper,
			mimeType: []string{"text/x-shellscript"},
		},
		{
			// the stale entry's path is now a symlink, it must not resolve through to the link target
			name:     "squashed relinked: shadowed type does not match through the new symlink",
			tree:     f.squashedRelinked,
			mimeType: []string{"text/x-shellscript"},
		},
		{
			name:     "squashed relinked: link target matches only at its own path",
			tree:     f.squashedRelinked,
			mimeType: []string{"application/x-executable"},
			want:     []searchHit{{"/app/real.sh", f.realID}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSearchContext(tt.tree, f.idx).SearchByMIMEType(tt.mimeType...)
			require.NoError(t, err)
			require.Equal(t, tt.want, searchHits(got))
		})
	}
}

func Test_searchContext_SearchByGlob_layeredPaths(t *testing.T) {
	f := newLayeredSearchFixture(t)

	tests := []struct {
		name string
		tree Reader
		glob string
		want []searchHit
	}{
		{
			name: "squashed: basename returns the live file once",
			tree: f.squashed,
			glob: "**/run.sh",
			want: []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			name: "squashed: basename glob returns the live file once",
			tree: f.squashed,
			glob: "**/run*",
			want: []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			name: "squashed: extension returns the live file once",
			tree: f.squashed,
			glob: "**/*.sh",
			want: []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			// /app is indexed once per layer that wrote it, its children must still be listed once
			name: "squashed: parent directory returns the live child once",
			tree: f.squashed,
			glob: "**/app/*",
			want: []searchHit{{"/app/run.sh", f.binID}},
		},
		{
			// only the symlink's own entry should produce the link resolution, not the stale script entry
			name: "squashed relinked: basename resolves the live symlink once",
			tree: f.squashedRelinked,
			glob: "**/run.sh",
			want: []searchHit{{"/app/run.sh", f.realID}},
		},
		{
			name: "squashed relinked: extension returns the symlink and its target once each",
			tree: f.squashedRelinked,
			glob: "**/*.sh",
			want: []searchHit{{"/app/real.sh", f.realID}, {"/app/run.sh", f.realID}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSearchContext(tt.tree, f.idx).SearchByGlob(tt.glob)
			require.NoError(t, err)
			require.Equal(t, tt.want, searchHits(got))
		})
	}
}

func Test_complexSymlinkPerformance(t *testing.T) {
	tr := New()
	idx := NewIndex()

	var pkgPaths []string

	numPkgs := 30 // with this few packages, the allPathsToNode behavior would essentially hang

	for num := range numPkgs {
		// add a concrete path
		realPath := fmt.Sprintf("/pkgs/lib-%d/package.json", num)
		pkgPaths = append(pkgPaths, realPath)
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
			expected: pkgPaths,
		},
	}

	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			sc := NewSearchContext(tr, idx)
			gotResolutions, err := sc.SearchByGlob(tt.glob, FollowBasenameLinks)
			require.NoError(t, err)
			require.NotNil(t, gotResolutions)
			require.ElementsMatch(t, tt.expected, realPaths(gotResolutions))
		})
	}
}

func Test_nextSegment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		start    int
		expected int
	}{
		{name: "empty", input: "", start: 0, expected: -1},
		{name: "empty after", input: "", start: 1, expected: -1},
		{name: "no slash", input: "abc", start: 0, expected: 3},
		{name: "no slash after", input: "abc", start: 3, expected: -1},
		{name: "end slash", input: "abc/", start: 0, expected: 3},
		{name: "end slash after", input: "abc/", start: 4, expected: -1},
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
			got := nextSegment(tt.input, tt.start)
			if got < 0 {
				got = -1
			}
			require.Equal(t, tt.expected, got)
		})
	}
}

// cycleSearchFixture builds a tree+index with a two-node symlink cycle among regular indexed files.
func cycleSearchFixture(t *testing.T, dir string, mimeType string) (*FileTree, Index) {
	t.Helper()
	tree := New()
	idx := NewIndex()

	for _, d := range file.Path(dir).AllPaths() {
		if d == "/" {
			continue
		}
		ref, err := tree.AddDir(d)
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{Type: file.TypeDirectory})
	}

	for _, name := range []string{"a-before", "zz-after"} {
		ref, err := tree.AddFile(file.Path(dir + "/" + name))
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType})
	}

	for from, to := range map[file.Path]file.Path{
		file.Path(dir + "/xz"):    file.Path(dir + "/xzcat"),
		file.Path(dir + "/xzcat"): file.Path(dir + "/xz"),
	} {
		ref, err := tree.AddSymLink(from, to)
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType, Type: file.TypeSymLink})
	}

	return tree, idx
}

// this is the path the dpkg cataloger takes when building package-file relationships, and the first error
// reported in the original issue.
func Test_searchContext_SearchByPath_SkipsLinkCycles(t *testing.T) {
	tree, idx := cycleSearchFixture(t, "/usr/bin", "application/x-executable")

	ref, err := NewSearchContext(tree, idx).SearchByPath("/usr/bin/xz")
	require.NoError(t, err)
	require.Nil(t, ref)

	// the healthy neighbors must still resolve
	ref, err = NewSearchContext(tree, idx).SearchByPath("/usr/bin/a-before")
	require.NoError(t, err)
	require.NotNil(t, ref)
}

// the sub-directory search basis (e.g. **/var/lib/dpkg/status.d/*) resolves entries by listing the parent,
// which is a separate path from the MIME-type search.
func Test_searchContext_SearchByGlob_SubDirectory_SkipsLinkCycles(t *testing.T) {
	tree, idx := cycleSearchFixture(t, "/var/lib/dpkg/status.d", "text/plain")

	results, err := NewSearchContext(tree, idx).SearchByGlob("**/status.d/*")
	require.NoError(t, err)

	require.ElementsMatch(t, []string{
		"/var/lib/dpkg/status.d/a-before",
		"/var/lib/dpkg/status.d/zz-after",
	}, realPaths(results))
}

func Test_searchContext_SearchByMIMEType_SkipsExcessiveLinkDepth(t *testing.T) {
	const mimeType = "application/x-executable"
	tree := New()
	idx := NewIndex()

	for _, name := range []string{"a-before", "zz-after"} {
		ref, err := tree.AddFile(file.Path("/usr/bin/" + name))
		require.NoError(t, err)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType})
	}
	for i := 0; i < maxLinkResolutionDepth+20; i++ {
		ref, err := tree.AddSymLink(
			file.Path(fmt.Sprintf("/usr/bin/l%04d", i)),
			file.Path(fmt.Sprintf("/usr/bin/l%04d", i+1)),
		)
		require.NoError(t, err)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType, Type: file.TypeSymLink})
	}

	results, err := NewSearchContext(tree, idx).SearchByMIMEType(mimeType)
	require.NoError(t, err)

	require.Subset(t, realPaths(results), []string{"/usr/bin/a-before", "/usr/bin/zz-after"})
}

// a malformed link anywhere in the image must not cost us the backward-reference index, which is what
// lets searches find files through symlinked parent directories. This is a differential test on purpose:
// asserting "no error" would not have caught the index silently emptying itself.
func Test_searchContext_LinkCycleDoesNotDropUnrelatedResults(t *testing.T) {
	const mimeType = "text/plain"

	// buildLinkResolutionIndex reaches the tree twice, and a malformed link can enter through either. Both are
	// covered here because they are separate call sites and fixing one silently leaves the other.
	const (
		noCycle = iota
		// a link that IS in the tree, reached while walking the index entries
		cycleViaTreeLink
		// an entry that is in the INDEX but not in this tree, reached while filtering entries down to this
		// tree. An index spans every layer while a search context is one squashed tree, so this is ordinary.
		cycleViaIndexOnlyEntry
	)

	build := func(t *testing.T, mode int) Searcher {
		t.Helper()
		tree := New()
		idx := NewIndex()

		ref, err := tree.AddFile("/usr/bin/legit")
		require.NoError(t, err)
		idx.Add(*ref, file.Metadata{MIMEType: mimeType})

		switch mode {
		case cycleViaTreeLink:
			// an unrelated malformed link. note: index entries come back sorted by file.ID, which is a
			// process-global counter, so these have to be CREATED before the healthy link below; otherwise the
			// healthy backward reference is already registered by the time the bad link truncates the index and
			// nothing is observable.
			for _, l := range []struct{ from, to file.Path }{{"/x", "/y"}, {"/y", "/x"}, {"/through", "/x/foo"}} {
				ref, err = tree.AddSymLink(l.from, l.to)
				require.NoError(t, err)
				idx.Add(*ref, file.Metadata{Type: file.TypeSymLink})
			}
		case cycleViaIndexOnlyEntry:
			for _, l := range []struct{ from, to file.Path }{{"/x", "/y"}, {"/y", "/x"}} {
				_, err = tree.AddSymLink(l.from, l.to)
				require.NoError(t, err)
			}
			// deliberately NOT added to the tree, so filtering it falls through to ancestor resolution and
			// trips the cycle at /x. Ordering does not matter here: this aborts before any backward reference
			// is registered, so the whole index is lost rather than a suffix of it.
			ghost := file.NewFileReference("/x/ghost")
			idx.Add(*ghost, file.Metadata{Type: file.TypeSymLink})
		}

		// a healthy directory link: /bin/legit is a legitimate second path to the same file
		ref, err = tree.AddSymLink("/bin", "/usr/bin")
		require.NoError(t, err)
		idx.Add(*ref, file.Metadata{Type: file.TypeSymLink})

		return NewSearchContext(tree, idx)
	}

	clean, err := build(t, noCycle).SearchByGlob("/bin/leg*")
	require.NoError(t, err)
	require.Equal(t, []string{"/usr/bin/legit"}, realPaths(clean))

	for name, mode := range map[string]int{
		"cycle via a link in the tree":  cycleViaTreeLink,
		"cycle via an index-only entry": cycleViaIndexOnlyEntry,
	} {
		t.Run(name, func(t *testing.T) {
			withCycle, err := build(t, mode).SearchByGlob("/bin/leg*")
			require.NoError(t, err)

			// the cycle is unrelated to this glob, so the results must be identical
			require.Equal(t, realPaths(clean), realPaths(withCycle))
		})
	}
}

func Test_searchContext_SearchByGlob_altAndClassBasenames(t *testing.T) {
	// basename globs with alternations and character classes reach the basename index (see parseGlobBasename)
	// while escaped globs skip it (see parseGlob), so the indexed search must agree with the authoritative tree
	// walk rather than silently returning less
	paths := []string{
		"/usr/lib/foo.jar",
		"/usr/lib/bar.war",
		"/usr/bin/python2",
		"/usr/bin/python3",
		"/opt/app/go.mod",
		"/opt/app/go.sum",
		"/usr/lib/aaab",
		"/usr/lib/foo*",
		"/usr/lib/foo*bar",
		"/srv/foobaz/x",
		"/a/b{",
		"/opt/foobar",
		"/opt/x1,y",
		"/opt/z",
	}

	tree := New()
	idx := NewIndex()

	// layers with a "./" tar entry (and every squashfs layer) index the root directory itself
	root, err := tree.AddDir("/")
	require.NoError(t, err)
	idx.Add(*root, file.Metadata{Path: "/", Type: file.TypeDirectory})

	for _, p := range paths {
		ref, err := tree.AddFile(file.Path(p))
		require.NoError(t, err)
		require.NotNil(t, ref)
		idx.Add(*ref, file.Metadata{Path: p, Type: file.TypeRegular})
	}

	tests := []struct {
		name    string
		glob    string
		want    []string
		wantErr bool
	}{
		{
			name: "alternation in the basename",
			glob: "**/*.{jar,war}",
			want: []string{"/usr/lib/bar.war", "/usr/lib/foo.jar"},
		},
		{
			name: "alternation suffixing the basename",
			glob: "**/python{2,3}",
			want: []string{"/usr/bin/python2", "/usr/bin/python3"},
		},
		{
			name: "character class in the basename",
			glob: "**/python[23]",
			want: []string{"/usr/bin/python2", "/usr/bin/python3"},
		},
		{
			name: "character class followed by a wildcard",
			glob: "**/go.[ms]*",
			want: []string{"/opt/app/go.mod", "/opt/app/go.sum"},
		},
		{
			name: "plain wildcard basename still works",
			glob: "**/python*",
			want: []string{"/usr/bin/python2", "/usr/bin/python3"},
		},
		{
			name: "negated class that would match the root",
			glob: "**/[!a-z]*",
			want: nil,
		},
		{
			name: "alternation that matches nothing",
			glob: "**/*.{deb,rpm}",
			want: nil,
		},
		{
			name: "wildcard that must backtrack",
			glob: "**/*aab",
			want: []string{"/usr/lib/aaab"},
		},
		{
			name: "escaped wildcard",
			glob: `**/foo\*`,
			want: []string{"/usr/lib/foo*"},
		},
		{
			name: "escaped brace in an alternation spanning a directory",
			glob: `**/{a/b\{}`,
			want: []string{"/a/b{"},
		},
		{
			name: "escaped literal basename",
			glob: `**/foo\bar`,
			want: []string{"/opt/foobar"},
		},
		{
			name: "escaped parent of a wildcard basename",
			glob: `**/foo\baz/*`,
			want: []string{"/srv/foobaz/x"},
		},
		{
			name: "escaped parent of a recursive basename",
			glob: `**/foo\baz/**`,
			want: []string{"/srv/foobaz/x"},
		},
		{
			name: "escaped asterisk followed by a wildcard",
			glob: `**/foo\**`,
			want: []string{"/usr/lib/foo*", "/usr/lib/foo*bar"},
		},
		{
			name: "escaped comma in a simple list",
			glob: `**/{x*\,y,z}`,
			want: []string{"/opt/x1,y", "/opt/z"},
		},
		{
			name: "simple list with a wildcard section",
			glob: "**/{python*,go.mod}",
			want: []string{"/opt/app/go.mod", "/usr/bin/python2", "/usr/bin/python3"},
		},
		{
			name: "overlapping alternation sections",
			glob: "**/{go.mod,go.*}",
			want: []string{"/opt/app/go.mod", "/opt/app/go.sum"},
		},
		{
			name:    "invalid basename glob falls back to the tree walk",
			glob:    "**/foo][",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexed, indexedErr := NewSearchContext(tree, idx).SearchByGlob(tt.glob)

			// the tree walk takes no part in the index lookup, so it is the reference answer
			walked, walkedErr := searchContext{tree: tree}.SearchByGlob(tt.glob)

			if tt.wantErr {
				require.ErrorIs(t, indexedErr, doublestar.ErrBadPattern, "indexed search")
				require.ErrorIs(t, walkedErr, doublestar.ErrBadPattern, "tree walk")
				return
			}
			require.NoError(t, indexedErr)
			require.NoError(t, walkedErr)

			assert.Equal(t, tt.want, realPaths(indexed), "indexed search")
			assert.Equal(t, tt.want, realPaths(walked), "tree walk")
		})
	}
}

func realPaths(refs []file.Resolution) []string {
	var paths []string
	for _, r := range refs {
		paths = append(paths, string(r.RealPath))
	}
	return paths
}

func Test_searchContext_nilIndex(t *testing.T) {
	tree := New()
	_, err := tree.AddFile("/path/to/file.txt")
	require.NoError(t, err)
	_, err = tree.AddSymLink("/link-to-file.txt", "/path/to/file.txt")
	require.NoError(t, err)

	sc := NewSearchContext(tree, nil)

	// both the file and the link match, and both resolve to the real file
	got, err := sc.SearchByGlob("**/*.txt")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, ref := range got {
		require.Equal(t, file.Path("/path/to/file.txt"), ref.RealPath)
	}

	_, err = sc.SearchByMIMEType("text/plain")
	require.ErrorContains(t, err, "no index available")
}
