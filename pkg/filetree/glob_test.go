package filetree

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/go-test/deep"
)

func TestFileInfoAdapter(t *testing.T) {
	tr := NewFileTree()
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
				FileType: file.TypeReg,
			},
		},
		"/home/wagoodman": {
			VirtualPath: "/home/wagoodman",
			Node: filenode.FileNode{
				RealPath: "/home/wagoodman",
				FileType: file.TypeDir,
			},
		},
		"/home/thing": {
			VirtualPath: "/home/thing",
			Node: filenode.FileNode{
				RealPath: "/home/thing",
				FileType: file.TypeSymlink,
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

			fileInfos, err := adapter.Readdir(-1)
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

func TestOSAdapter_Lstat(t *testing.T) {
	tr := NewFileTree()
	tr.AddFile("/home/thing.txt")
	tr.AddDir("/home/wagoodman")
	tr.AddSymLink("/home/thing", "./thing.txt")
	// note: link destination does not exist
	tr.AddHardLink("/home/place", "/somewhere-else")

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
					FileType: file.TypeDir,
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
					FileType: file.TypeSymlink,
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
	tr := NewFileTree()
	tr.AddFile("/home/thing.txt")
	tr.AddDir("/home/wagoodman")
	tr.AddSymLink("/home/thing", "./thing.txt")
	// note: link destination does not exist
	tr.AddHardLink("/home/place", "/somewhere-else")

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
					FileType: file.TypeDir,
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
					FileType: file.TypeReg,
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
