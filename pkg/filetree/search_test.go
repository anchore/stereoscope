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

	var paths []string
	for _, result := range results {
		paths = append(paths, string(result.RealPath))
	}
	require.ElementsMatch(t, []string{
		"/usr/bin/a-before",
		"/usr/bin/zz-after",
	}, paths)
}

func Test_complexSymlinkPerformance(t *testing.T) {
	tr := New()
	idx := NewIndex()

	var realPaths []string

	numPkgs := 30 // with this few packages, the allPathsToNode behavior would essentially hang

	for num := range numPkgs {
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

	var paths []string
	for _, result := range results {
		paths = append(paths, string(result.RealPath))
	}
	require.ElementsMatch(t, []string{
		"/var/lib/dpkg/status.d/a-before",
		"/var/lib/dpkg/status.d/zz-after",
	}, paths)
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

	var paths []string
	for _, result := range results {
		paths = append(paths, string(result.RealPath))
	}
	require.Subset(t, paths, []string{"/usr/bin/a-before", "/usr/bin/zz-after"})
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

	paths := func(t *testing.T, refs []file.Resolution) []string {
		t.Helper()
		var out []string
		for _, r := range refs {
			out = append(out, string(r.RealPath))
		}
		return out
	}

	clean, err := build(t, noCycle).SearchByGlob("/bin/leg*")
	require.NoError(t, err)
	require.Equal(t, []string{"/usr/bin/legit"}, paths(t, clean))

	for name, mode := range map[string]int{
		"cycle via a link in the tree":  cycleViaTreeLink,
		"cycle via an index-only entry": cycleViaIndexOnlyEntry,
	} {
		t.Run(name, func(t *testing.T) {
			withCycle, err := build(t, mode).SearchByGlob("/bin/leg*")
			require.NoError(t, err)

			// the cycle is unrelated to this glob, so the results must be identical
			require.Equal(t, paths(t, clean), paths(t, withCycle))
		})
	}
}
