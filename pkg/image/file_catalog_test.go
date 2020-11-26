package image

import (
	"testing"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
)

var testFilePaths = []file.Path{
	"/home",
	"/home/dan",
	"/home/alex",
	"/home/alfredo",
	"/home/alfredo/special-file",
}

func TestFileCatalog_HasEntriesForAllFilesInTree(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T, filePaths []file.Path, fileTree *tree.FileTree, catalog *FileCatalog)
		expected bool
	}{
		{
			name: "identical set of files",
			setup: func(t *testing.T, filePaths []file.Path, fileTree *tree.FileTree, catalog *FileCatalog) {
				for _, p := range filePaths {
					f, err := fileTree.AddPathAndMissingAncestors(p)
					if err != nil {
						t.Fatal(err)
					}
					catalog.Add(f, file.Metadata{}, &Layer{})
				}
			},
			expected: true,
		},
		{
			name: "catalog missing one file that tree has",
			setup: func(t *testing.T, filePaths []file.Path, fileTree *tree.FileTree, catalog *FileCatalog) {
				for i, p := range filePaths {
					f, err := fileTree.AddPathAndMissingAncestors(p)
					if err != nil {
						t.Fatal(err)
					}

					if i != 1 { // don't add filePaths[1] to the catalog
						catalog.Add(f, file.Metadata{}, &Layer{})
					}
				}
			},
			expected: false,
		},
		{
			name: "tree missing one file that catalog has",
			setup: func(t *testing.T, filePaths []file.Path, fileTree *tree.FileTree, catalog *FileCatalog) {
				for i, p := range filePaths {
					if i == 1 { // add filePaths[1] to only the catalog, not the tree
						catalog.Add(file.NewFileReference(p), file.Metadata{}, &Layer{})
						return
					}

					f, err := fileTree.AddPathAndMissingAncestors(p)
					if err != nil {
						t.Fatal(err)
					}
					catalog.Add(f, file.Metadata{}, &Layer{})
				}
			},
			expected: true,
		},
		{
			name: "no files added to tree",
			setup: func(t *testing.T, filePaths []file.Path, fileTree *tree.FileTree, catalog *FileCatalog) {
				for _, p := range filePaths {
					f := file.NewFileReference(p)
					catalog.Add(f, file.Metadata{}, &Layer{})
				}
			},
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fileTree := tree.NewFileTree()
			catalog := NewFileCatalog()

			// Add file tree root to catalog
			catalog.Add(*(fileTree.File("/")), file.Metadata{}, &Layer{})

			tc.setup(t, testFilePaths, fileTree, &catalog)

			result := catalog.HasEntriesForAllFilesInTree(*fileTree)

			if tc.expected != result {
				t.Errorf("expected %t but got %t", tc.expected, result)
			}
		})
	}
}
