package filetree

import (
	"sort"
	"testing"

	"github.com/go-test/deep"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
)

func TestFileInfoAdapter(t *testing.T) {
	tr := New()
	tr.AddFile("/home/thing.txt")
	tr.AddDir("/home/wagoodman")
	tr.AddSymLink("/home/thing", "./thing.txt")
	// note: link destination does not exist
	tr.AddHardLink("/home/place", "/somewhere-else")

	// child order is not guaranteed... so we must compare by map
	homeFiles := map[string]fileinfoAdapter{
		"/home/thing.txt": {
			VirtualPath: "/home/thing.txt",
			Node: filenode.FileNode{
				RealPath: "/home/thing.txt",
				FileType: file.TypeRegular,
			},
		},
		"/home/wagoodman": {
			VirtualPath: "/home/wagoodman",
			Node: filenode.FileNode{
				RealPath: "/home/wagoodman",
				FileType: file.TypeDirectory,
			},
		},
		"/home/thing": {
			VirtualPath: "/home/thing",
			Node: filenode.FileNode{
				RealPath: "/home/thing",
				FileType: file.TypeSymLink,
				LinkPath: "./thing.txt",
			},
		},
		"/home/place": {
			VirtualPath: "/home/place",
			Node: filenode.FileNode{
				RealPath: "/home/place",
				FileType: file.TypeHardLink,
				LinkPath: "/somewhere-else",
			},
		},
	}

	tests := []struct {
		name                         string
		doNotFollowDeadBasenameLinks bool
		path                         string
		expectedFiles                map[string]fileinfoAdapter
	}{
		{
			// note that since Lstat is used, the osAdapter option for following dead basename links HAS NO EFFECT
			name:                         "follow dead basename links",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home",
			expectedFiles:                homeFiles,
		},
		{
			name:                         "do NOT follow dead basename links",
			doNotFollowDeadBasenameLinks: true,
			path:                         "/home",
			expectedFiles:                homeFiles,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := fileAdapter{
				os: &osAdapter{
					filetree:                     tr,
					doNotFollowDeadBasenameLinks: test.doNotFollowDeadBasenameLinks,
				},
				filetree: tr,
				name:     test.path,
			}

			fileInfos, err := adapter.ReadDir(-1)
			if err != nil {
				t.Fatalf("could not read dir: %+v", err)
			}
			if err = adapter.Close(); err != nil {
				t.Fatalf("close should have no effect")
			}

			var files = make(map[string]fileinfoAdapter)
			for _, fi := range fileInfos {
				f := fi.(*fileinfoAdapter)
				f.Node.Reference = nil
				if _, exists := files[string(f.VirtualPath)]; exists {
					t.Errorf("duplicate entry: %+v", f.VirtualPath)
				} else {
					files[string(f.VirtualPath)] = *f
				}
			}

			if len(files) != len(test.expectedFiles) {
				for _, f := range files {
					t.Logf("   actual: %+v", f)
				}
				t.Fatalf("unexpected number of files: %d != %d", len(files), len(test.expectedFiles))
			}

			for _, d := range deep.Equal(test.expectedFiles, files) {
				t.Errorf("   diff: %+v", d)
			}

		})
	}
}

func TestOsAdapter_PreventInfiniteLoop(t *testing.T) {
	tr := New()
	tr.AddFile("/usr/bin/busybox")
	tr.AddSymLink("/usr/bin/X11", ".")

	tests := []struct {
		name       string
		path       string
		childCount int
	}{
		{
			name:       "children on real path",
			path:       "/usr/bin",
			childCount: 2,
		},
		{
			name:       "first link iteration shows children",
			path:       "/usr/bin/X11",
			childCount: 2,
		},
		{
			name:       "second link iteration DOES NOT show children",
			path:       "/usr/bin/X11/X11",
			childCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &osAdapter{
				filetree:                     tr,
				doNotFollowDeadBasenameLinks: false,
			}
			fileInfos, err := adapter.ReadDir(test.path)
			if err != nil {
				t.Fatalf("could not read dir: %+v", err)
			}

			if len(fileInfos) != test.childCount {
				for _, f := range fileInfos {
					t.Logf("   actual: %+v", f)
				}
				t.Errorf("unexpected number of files: %d != %d", len(fileInfos), test.childCount)
			}

		})
	}
}

func TestFileInfoAdapter_PreventInfiniteLoop(t *testing.T) {
	tr := New()
	tr.AddFile("/usr/bin/busybox")
	tr.AddSymLink("/usr/bin/X11", ".")

	tests := []struct {
		name       string
		path       string
		childCount int
	}{
		{
			name:       "children on real path",
			path:       "/usr/bin",
			childCount: 2,
		},
		{
			name:       "first link iteration shows children",
			path:       "/usr/bin/X11",
			childCount: 2,
		},
		{
			name:       "second link iteration DOES NOT show children",
			path:       "/usr/bin/X11/X11",
			childCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := fileAdapter{
				os: &osAdapter{
					filetree:                     tr,
					doNotFollowDeadBasenameLinks: false,
				},
				filetree: tr,
				name:     test.path,
			}

			fileInfos, err := adapter.ReadDir(-1)
			if err != nil {
				t.Fatalf("could not read dir: %+v", err)
			}
			if err = adapter.Close(); err != nil {
				t.Fatalf("close should have no effect")
			}

			if len(fileInfos) != test.childCount {
				for _, f := range fileInfos {
					t.Logf("   actual: %+v", f)
				}
				t.Errorf("unexpected number of files: %d != %d", len(fileInfos), test.childCount)
			}

		})
	}
}

func TestOSAdapter_ReadDir(t *testing.T) {
	tr := newHelperTree()

	tests := []struct {
		name                         string
		doNotFollowDeadBasenameLinks bool
		path                         string
		expected                     []fileinfoAdapter
		shouldErr                    bool
	}{
		{
			name:                         "ReadDir fetches the filesInfos correctly",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home",
			expected: []fileinfoAdapter{
				{
					VirtualPath: "/home/thing.txt",
					Node:        filenode.FileNode{RealPath: "/home/thing.txt", FileType: file.TypeRegular},
				},

				{
					VirtualPath: "/home/wagoodman",
					Node:        filenode.FileNode{RealPath: "/home/wagoodman", FileType: file.TypeDirectory},
				},
				{
					VirtualPath: "/home/thing",
					Node:        filenode.FileNode{RealPath: "/home/thing", FileType: file.TypeSymLink, LinkPath: "./thing.txt"},
				},
				{
					VirtualPath: "/home/place",
					Node:        filenode.FileNode{RealPath: "/home/place", FileType: file.TypeHardLink, LinkPath: "/somewhere-else"},
				},
			},
			shouldErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := osAdapter{
				filetree:                     tr,
				doNotFollowDeadBasenameLinks: test.doNotFollowDeadBasenameLinks,
			}

			fileInfos, err := adapter.ReadDir(test.path)
			if err != nil {
				t.Fatalf("could not lstat: %+v", err)
			}

			actual := make([]fileinfoAdapter, 0)

			for _, fileInfo := range fileInfos {
				fi := fileInfo.(*fileinfoAdapter)
				fi.Node.Reference = nil
				actual = append(actual, *fi)
			}

			// sort outputs for compare
			for _, fi := range [][]fileinfoAdapter{actual, test.expected} {
				sort.Slice(fi, func(i, j int) bool {
					return fi[i].VirtualPath > fi[j].VirtualPath
				})
			}

			for _, d := range deep.Equal(test.expected, actual) {
				t.Errorf("   diff: %+v", d)
			}
		})
	}

}

func TestOSAdapter_Lstat(t *testing.T) {
	tr := newHelperTree()

	tests := []struct {
		name                         string
		doNotFollowDeadBasenameLinks bool
		path                         string
		expected                     fileinfoAdapter
	}{
		{
			name:                         "dir",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home",
			expected: fileinfoAdapter{
				VirtualPath: "/home",
				Node: filenode.FileNode{
					RealPath: "/home",
					FileType: file.TypeDirectory,
				},
			},
		},
		{
			name:                         "symlink",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home/thing",
			expected: fileinfoAdapter{
				VirtualPath: "/home/thing",
				Node: filenode.FileNode{
					RealPath: "/home/thing",
					FileType: file.TypeSymLink,
					LinkPath: "./thing.txt",
				},
			},
		},
		{
			// NOTE: following dead basename links should have NO EFFECT (this is the definition of Lstat)
			name:                         "follow dead basename links",
			doNotFollowDeadBasenameLinks: false, // <---------------
			path:                         "/home/place",
			expected: fileinfoAdapter{
				VirtualPath: "/home/place",
				Node: filenode.FileNode{
					RealPath: "/home/place",
					FileType: file.TypeHardLink,
					LinkPath: "/somewhere-else",
				},
			},
		},
		{
			// NOTE: following dead basename links should have NO EFFECT (this is the definition of Lstat)
			name:                         "DO NOT follow dead basename links",
			doNotFollowDeadBasenameLinks: true, // <--------------
			path:                         "/home/place",
			expected: fileinfoAdapter{
				VirtualPath: "/home/place",
				Node: filenode.FileNode{
					RealPath: "/home/place",
					FileType: file.TypeHardLink,
					LinkPath: "/somewhere-else",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := osAdapter{
				filetree:                     tr,
				doNotFollowDeadBasenameLinks: test.doNotFollowDeadBasenameLinks,
			}

			fileInfo, err := adapter.Lstat(test.path)
			if err != nil {
				t.Fatalf("could not lstat: %+v", err)
			}

			actual := fileInfo.(*fileinfoAdapter)
			actual.Node.Reference = nil

			for _, d := range deep.Equal(test.expected, *actual) {
				t.Errorf("   diff: %+v", d)
			}

		})
	}

}

func TestOSAdapter_Stat(t *testing.T) {
	tr := newHelperTree()
	tests := []struct {
		name                         string
		doNotFollowDeadBasenameLinks bool
		path                         string
		expected                     fileinfoAdapter
		doesNotExist                 bool
	}{
		{
			name:                         "dir",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home",
			expected: fileinfoAdapter{
				VirtualPath: "/home",
				Node: filenode.FileNode{
					RealPath: "/home",
					FileType: file.TypeDirectory,
				},
			},
		},
		{
			name:                         "symlink",
			doNotFollowDeadBasenameLinks: false,
			path:                         "/home/thing",
			expected: fileinfoAdapter{
				// note the path is the request path (not the path to the resolved file)
				VirtualPath: "/home/thing",
				Node: filenode.FileNode{
					RealPath: "/home/thing.txt",
					FileType: file.TypeRegular,
				},
			},
		},
		{
			name:                         "follow dead basename links",
			doNotFollowDeadBasenameLinks: false, // <---------------
			path:                         "/home/place",
			expected:                     fileinfoAdapter{},
			doesNotExist:                 true,
		},
		{
			name:                         "DO NOT follow dead basename links",
			doNotFollowDeadBasenameLinks: true, // <--------------
			path:                         "/home/place",
			expected: fileinfoAdapter{
				VirtualPath: "/home/place",
				Node: filenode.FileNode{
					RealPath: "/home/place",
					FileType: file.TypeHardLink,
					LinkPath: "/somewhere-else",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := osAdapter{
				filetree:                     tr,
				doNotFollowDeadBasenameLinks: test.doNotFollowDeadBasenameLinks,
			}

			fileInfo, err := adapter.Stat(test.path)
			if err != nil && !test.doesNotExist {
				t.Fatalf("could not stat: %+v", err)
			} else if err == nil && test.doesNotExist {
				t.Fatalf("expected to not exist but does")
			}

			if test.doesNotExist {
				return
			}

			actual := fileInfo.(*fileinfoAdapter)
			actual.Node.Reference = nil

			for _, d := range deep.Equal(test.expected, *actual) {
				t.Errorf("   diff: %+v", d)
			}

		})
	}

}

func newHelperTree() *FileTree {
	tr := New()
	tr.AddFile("/home/thing.txt")
	tr.AddDir("/home/wagoodman")
	tr.AddSymLink("/home/thing", "./thing.txt")
	// note: link destination does not exist
	tr.AddHardLink("/home/place", "/somewhere-else")

	return tr
}
