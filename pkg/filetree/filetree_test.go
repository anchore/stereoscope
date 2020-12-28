package filetree

import (
	"errors"
	"fmt"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"testing"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/pkg/file"
)

func TestFileTree_AddPath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home")
	fileNode, err := tr.AddFile(path)
	if err != nil {
		t.Fatalf("could not add path: %+v", err)
	}

	_, _, f, _ := tr.File(path)
	if f != fileNode {
		t.Fatal("expected pointer to the newly created fileNode")
	}
}

func TestFileTree_AddPathAndMissingAncestors(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	fileNode, err := tr.AddFile(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	_, _, f, _ := tr.File(path)
	if f != fileNode {
		t.Fatal("expected pointer to the newly created fileNode")
	}

	parent := file.Path("/home/wagoodman")
	child := file.Path("/home/wagoodman/awesome")

	_, n, err := tr.node(parent, linkResolutionStrategy{})
	if err != nil {
		t.Fatalf("could not get parent node: %+v", err)
	}
	children := tr.tree.Children(n)

	if len(children) != 1 {
		t.Fatal("unexpected child count", len(children))
	}

	if children[0].ID() != filenode.IdByPath(child) {
		t.Fatal("unexpected child", children[0])
	}
}

func TestFileTree_RemovePath(t *testing.T) {
	tr := NewFileTree()
	path := file.Path("/home/wagoodman/awesome/file.txt")
	_, err := tr.AddFile(path)
	if err != nil {
		t.Fatal("could not add path", err)
	}

	err = tr.RemovePath("/home/wagoodman/awesome")
	if err != nil {
		t.Fatal("could not remote path", err)
	}

	if len(tr.tree.Nodes()) != 3 {
		t.Fatal("unexpected node count", len(tr.tree.Nodes()), tr.tree.Nodes())
	}

	_, _, f, _ := tr.File(path)
	if f != nil {
		t.Fatal("expected file to be missing")
	}

	err = tr.RemovePath("/")
	if !errors.Is(err, ErrRemovingRoot) {
		t.Fatalf("should not be able to remove root path, but the call returned err: %v", err)
	}
}

func TestFileTree_FilesByGlob(t *testing.T) {
	tr := NewFileTree()

	paths := []string{
		"/home/wagoodman/awesome/file.txt",
		"/home/wagoodman/file.txt",
		"/home/wagoodman/b-file.txt",
		"/home/wagoodman/some/deeply/nested/spot/file.txt",
		"/home/a-file.txt",
		"/home/nothing.txt",
		"/home/dir",
		"/place/example.gif",
		"/sym-linked-dest/another/a-.gif",
		"/hard-linked-dest/something/b-.gif",
	}

	for _, p := range paths {
		_, err := tr.AddFile(file.Path(p))
		if err != nil {
			t.Fatalf("failed to add path ('%s'): %+v", p, err)
		}
	}

	// absolute symlink
	_, err := tr.AddSymLink("/home/elsewhere/symlink", "/sym-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	// relative symlink
	_, err = tr.AddSymLink("/home/again/symlink", "../../../sym-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	// dead symlink (dir)
	_, err = tr.AddSymLink("/home/again/deadsymlink", "../ijustdontexist")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	// dead symlink (to txt)
	_, err = tr.AddSymLink("/home/again/dead.jpg", "../ialsojustdontexist")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	// hardlink
	_, err = tr.AddHardLink("/home/elsewhere/hardlink", "/hard-linked-dest")
	if err != nil {
		t.Fatalf("could not setup link: %+v", err)
	}

	tests := []struct {
		pattern  string
		options  []LinkResolutionOption
		expected []string
		err      bool
	}{
		///////////////////////
		// symlinked paths
		{
			// parent is an absolute & relative symlink
			pattern: "**/a-.gif",
			expected: []string{
				"/home/elsewhere/symlink/another/a-.gif",
				"/home/again/symlink/another/a-.gif",
				"/sym-linked-dest/another/a-.gif",
			},
		},
		{
			// parent is an absolute & relative symlink
			pattern: "**/symlink/another/a-.gif",
			expected: []string{
				"/home/elsewhere/symlink/another/a-.gif",
				"/home/again/symlink/another/a-.gif",
			},
		},
		{
			// symlink with dead basename (follow)
			pattern:  "**/dead.jpg",
			expected: []string{},
		},
		{
			// symlink with dead basename (do not follow)
			pattern: "**/dead.jpg",
			options: []LinkResolutionOption{DoNotFollowDeadBasenameLinks},
			expected: []string{
				"/home/again/dead.jpg",
			},
		},
		///////////////////////
		// hardlinked paths
		{
			// parent is a hardlink
			pattern: "**/b-.gif",
			expected: []string{
				"/home/elsewhere/hardlink/something/b-.gif",
				"/hard-linked-dest/something/b-.gif",
			},
		},
		{
			// parent is a hardlink
			pattern: "**/hardlink/something/b-.gif",
			expected: []string{
				"/home/elsewhere/hardlink/something/b-.gif",
			},
		},
		///////////////////////
		// mixed links
		{
			// parent is a hardlink or symlink
			pattern: "**/elsewhere/**/?-.gif",
			expected: []string{
				"/home/elsewhere/symlink/another/a-.gif",
				"/home/elsewhere/hardlink/something/b-.gif",
			},
		},
		////////////////////////
		// real paths
		{
			pattern: "/home/wagoodman/**/file.txt",
			expected: []string{
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
			},
		},
		{
			pattern: "/home/wagoodman/**",
			expected: []string{
				// note: this will only find files, not dirs
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			pattern:  "file.txt",
			expected: []string{},
		},
		{
			pattern:  "*file.txt",
			expected: []string{},
		},
		{
			pattern: "**/*file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/a-file.txt",
			},
		},
		{
			pattern: "*/example.gif",
			expected: []string{
				"/place/example.gif",
			},
		},
		{
			pattern: "/**/file.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
			},
		},
		{
			pattern: "/**/*-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			pattern: "/**/?-file.txt",
			expected: []string{
				"/home/a-file.txt",
				"/home/wagoodman/b-file.txt",
			},
		},
		{
			pattern: "**/a-file.txt",
			expected: []string{
				"/home/a-file.txt",
			},
		},
		{
			pattern: "/**/*.txt",
			expected: []string{
				"/home/wagoodman/awesome/file.txt",
				"/home/wagoodman/file.txt",
				"/home/wagoodman/b-file.txt",
				"/home/wagoodman/some/deeply/nested/spot/file.txt",
				"/home/a-file.txt",
				"/home/nothing.txt",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			//t.Log("PATTERN: ", test.pattern)
			actual, err := tr.FilesByGlob(test.pattern, test.options...)
			if err != nil && !test.err {
				t.Fatal("failed to search by glob:", err)
			} else if err == nil && test.err {
				t.Fatalf("expected an error but did not get one")
			} else if err != nil && test.err {
				// we expected an error, nothing else matters
				return
			}

			actualSet := internal.NewStringSet()
			expectedSet := internal.NewStringSet()

			for _, r := range actual {
				actualSet.Add(string(r.MatchPath))
			}

			for _, e := range test.expected {
				expectedSet.Add(e)
				if !actualSet.Contains(e) {
					t.Errorf("missing search hit: %s", e)
				}
			}

			for _, r := range actual {
				if !expectedSet.Contains(string(r.MatchPath)) {
					t.Errorf("extra search hit: %+v", r)
				}
			}

		})
	}

}

func TestFileTree_Merge(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddFile("/home/wagoodman/awesome/file-1.txt")

	tr2 := NewFileTree()
	tr2.AddFile("/home/wagoodman/awesome/file-2.txt")

	if err := tr1.merge(tr2); err != nil {
		t.Fatalf("error on merge : %+v", err)
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome/file-1.txt", "/home/wagoodman/awesome/file-2.txt"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}
}

func TestFileTree_Merge_Overwrite(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddFile("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	newRef, _ := tr2.AddFile("/home/wagoodman/awesome/file.txt")

	if err := tr1.merge(tr2); err != nil {
		t.Fatalf("error on merge : %+v", err)
	}

	_, _, f, _ := tr1.File("/home/wagoodman/awesome/file.txt")
	if f.ID() != newRef.ID() {
		t.Fatalf("did not overwrite paths on merge")
	}

}

func TestFileTree_Merge_OpaqueWhiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddFile("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddFile("/home/wagoodman/.wh..wh..opq")

	if err := tr1.merge(tr2); err != nil {
		t.Fatalf("error on merge : %+v", err)
	}

	for _, p := range []file.Path{"/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("expected path to be deleted: %s", p)
		}
	}

}

func TestFileTree_Merge_OpaqueWhiteout_NoLowerDirectory(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddFile("/home")

	tr2 := NewFileTree()
	tr2.AddFile("/home/luhring/.wh..wh..opq")

	if err := tr1.merge(tr2); err != nil {
		t.Fatalf("error on merge : %+v", err)
	}

	for _, p := range []file.Path{"/home/luhring", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}
}

func TestFileTree_Merge_Whiteout(t *testing.T) {
	tr1 := NewFileTree()
	tr1.AddFile("/home/wagoodman/awesome/file.txt")

	tr2 := NewFileTree()
	tr2.AddFile("/home/wagoodman/awesome/.wh.file.txt")

	if err := tr1.merge(tr2); err != nil {
		t.Fatalf("error on merge : %+v", err)
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome", "/home/wagoodman", "/home"} {
		if !tr1.HasPath(p) {
			t.Errorf("missing expected path: %s", p)
		}
	}

	for _, p := range []file.Path{"/home/wagoodman/awesome/file.txt"} {
		if tr1.HasPath(p) {
			t.Errorf("expected path to be deleted: %s", p)
		}
	}

}

func TestFileTree_File_Symlink(t *testing.T) {

	tests := []struct {
		name                 string
		buildLinkSource      file.Path // ln -s <SOURCE> DEST
		buildLinkDest        file.Path // ln -s SOURCE <DEST>
		buildRealPath        file.Path // a real file that should exist (or not if "")
		linkOptions          []LinkResolutionOption
		requestPath          file.Path // the path to check against
		expectedExists       bool      // if the request path should exist or not
		expectedResolvedPath file.Path // the expected path for a request result
		expectedErr          bool      // if an error is expected from the request
		expectedRealRef      bool      // if the resolved reference should match the built reference from "buildRealPath"
	}{
		///////////////
		{
			name:                 "request base is ABSOLUTE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "/another/place",
			buildRealPath:        "/another/place",
			linkOptions:          []LinkResolutionOption{FollowBasenameLinks},
			requestPath:          "/home",
			expectedExists:       true,
			expectedResolvedPath: "/another/place",
			// /another/place is the "real" reference that we followed, so we should expect the IDs to match upon lookup
			expectedRealRef: true,
		},
		{
			name:                 "request base is ABSOLUTE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "/another/place",
			buildRealPath:        "/another/place",
			linkOptions:          []LinkResolutionOption{},
			requestPath:          "/home",
			expectedExists:       true,
			expectedResolvedPath: "/home",
			// /home is just a symlink, not the real file (which is at /another/place)
			expectedRealRef: false,
		},

		///////////////
		{
			name:                 "request parent is ABSOLUTE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "/another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{FollowBasenameLinks}, // a nop for this case (note the expected path and ref)
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},
		{
			name:                 "request parent is ABSOLUTE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "/another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{}, // a nop for this case (note the expected path and ref)
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},

		///////////////
		{
			name:                 "request base is RELATIVE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "../../another/place",
			buildRealPath:        "/another/place",
			linkOptions:          []LinkResolutionOption{FollowBasenameLinks},
			requestPath:          "/home",
			expectedExists:       true,
			expectedResolvedPath: "/another/place",
			expectedRealRef:      true,
		},
		{
			name:            "request base is RELATIVE symlink",
			buildLinkSource: "/home",
			buildLinkDest:   "../../another/place/wagoodman",
			buildRealPath:   "/another/place/wagoodman",
			linkOptions:     []LinkResolutionOption{},
			requestPath:     "/home",
			expectedExists:  true,
			// note that since the request matches the link source and we are NOT following, we get the link ref back
			expectedResolvedPath: "/home",
			expectedRealRef:      false,
		},
		///////////////
		{
			name:                 "request parent is RELATIVE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "../../another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{FollowBasenameLinks}, // this is a nop since the parent is a link
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},
		{
			name:                 "request parent is RELATIVE symlink",
			buildLinkSource:      "/home",
			buildLinkDest:        "../../another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{}, // this is a nop since the parent is a link
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},
		///////////////
		{
			name:            "request base is DEAD symlink",
			buildLinkSource: "/home",
			buildLinkDest:   "/mwahaha/i/go/to/nowhere",
			linkOptions:     []LinkResolutionOption{},
			requestPath:     "/home",
			// since we did not follow, the paths should exist to the symlink file
			expectedResolvedPath: "/home",
			expectedExists:       true,
		},
		{
			name:            "request base is DEAD symlink",
			buildLinkSource: "/home",
			buildLinkDest:   "/mwahaha/i/go/to/nowhere",
			linkOptions:     []LinkResolutionOption{FollowBasenameLinks},
			requestPath:     "/home",
			// we are following the path, which goes to nowhere.... the first failed path is resolved and returned
			expectedResolvedPath: "/mwahaha",
			expectedExists:       false,
		},
		{
			name:            "request base is DEAD symlink (which we don't follow)",
			buildLinkSource: "/home",
			buildLinkDest:   "/mwahaha/i/go/to/nowhere",
			linkOptions:     []LinkResolutionOption{FollowBasenameLinks, DoNotFollowDeadBasenameLinks},
			requestPath:     "/home",
			// we are following the path, which goes to nowhere.... the first failed path is resolved and returned
			expectedResolvedPath: "/home",
			expectedExists:       true,
		},
		///////////////
		// trying to resolve to above root
		{
			name:                 "request parent is RELATIVE symlink to ABOVE root",
			buildLinkSource:      "/home",
			buildLinkDest:        "../../../../../../../../../../../../another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{FollowBasenameLinks}, // this is a nop since the parent is a link
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},
		{
			name:                 "request parent is RELATIVE symlink to ABOVE root",
			buildLinkSource:      "/home",
			buildLinkDest:        "../../../../../../../../../../../../another/place",
			buildRealPath:        "/another/place/wagoodman",
			linkOptions:          []LinkResolutionOption{}, // this is a nop since the parent is a link
			requestPath:          "/home/wagoodman",
			expectedExists:       true,
			expectedResolvedPath: "/another/place/wagoodman",
			expectedRealRef:      true,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s (follow=%+v)", test.name, test.linkOptions), func(t *testing.T) {
			tr := NewFileTree()
			_, err := tr.AddSymLink(test.buildLinkSource, test.buildLinkDest)
			if err != nil {
				t.Fatalf("unexpected an error on add link: %+v", err)
			}

			var realRef *file.Reference
			if test.buildRealPath != "" {
				realRef, _ = tr.AddFile(test.buildRealPath)
			}

			exists, p, ref, err := tr.File(test.requestPath, test.linkOptions...)
			if err != nil && !test.expectedErr {
				t.Fatalf("unexpected error: %+v", err)
			} else if err == nil && test.expectedErr {
				t.Fatalf("expected error but got none")
			}

			if test.expectedErr {
				// don't validate beyond an expected error...
				return
			}

			// validate exists...
			if exists && !test.expectedExists {
				t.Fatalf("expected path to NOT exist, but does")
			} else if !exists && test.expectedExists {
				t.Fatalf("expected path to exist, but does NOT")
			}

			// validate path...
			if p != test.expectedResolvedPath {
				t.Fatalf("unexpected path difference: %+v != %v", p, test.expectedResolvedPath)
			}

			// validate ref...
			if realRef != nil {
				if ref.ID() == realRef.ID() && !test.expectedRealRef {
					t.Errorf("refs should not be the same: resolve(%+v) == reaal(%+v)", ref, realRef)
				} else if ref.ID() != realRef.ID() && test.expectedRealRef {
					t.Errorf("refs should be the same: resolve(%+v) != real(%+v)", ref, realRef)
				}
			} else {
				if test.expectedRealRef {
					t.Fatalf("expected to test a real reference, but could not")
				}
			}
		})
	}
}

func TestFileTree_ResolveFile_MultipleIndirections(t *testing.T) {
	tr := NewFileTree()
	// first indirection
	_, err := tr.AddSymLink("/home", "/another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	// second indirection
	_, err = tr.AddSymLink("/another/place", "/someother/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	// concrete file
	realHome, _ := tr.AddFile("/someother/place/wagoodman")

	// the test.... do we resolve through multiple indirections?
	exists, resolvedPath, resolvedHome, err := tr.File("/home/wagoodman", FollowBasenameLinks)
	if err != nil {
		t.Fatalf("should not have gotten an error on resolving a file: %+v", err)
	}
	if !exists {
		t.Fatalf("expected path does not exist: %+v", resolvedPath)
	}

	// we are expecting the resolution for /home/wagoodman to result in /someother/place/wagoodman
	if resolvedPath != "/someother/place/wagoodman" {
		t.Fatalf("path resolution through link failed: %+v", resolvedPath)
	}

	if resolvedHome == nil {
		t.Fatalf("expected a ref but got none")
	}

	if resolvedHome.ID() != realHome.ID() {
		t.Errorf("failed to resolve to home symlink ref: %+v != %+v", resolvedHome.ID(), realHome.ID())
	}
}

func TestFileTree_ResolveFile_CycleDetection(t *testing.T) {
	tr := NewFileTree()
	// first indirection
	_, err := tr.AddSymLink("/home", "/another/place")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	// second indirection
	_, err = tr.AddSymLink("/another/place", "/home")
	if err != nil {
		t.Fatalf("unexpected an error on add link: %+v", err)
	}

	// the test.... do we stop when a cycle is detected?
	exists, resolvedPath, _, err := tr.File("/home/wagoodman", FollowBasenameLinks)
	if err != ErrLinkCycleDetected {
		t.Fatalf("should have gotten an error on resolving a file")
	}

	if resolvedPath != "/home" {
		t.Errorf("cycle path should be hinted in resolved path, given %q", resolvedPath)
	}

	if exists {
		t.Errorf("resolution should not exist in cycle")
	}

}
