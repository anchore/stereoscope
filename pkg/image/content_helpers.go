package image

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
)

// fetchFileContentsByPath is a common helper function for resolving the file contents for a path from the file
// catalog relative to the given tree.
func fetchFileContentsByPath(ft *tree.FileTree, fileCatalog *FileCatalog, path file.Path) (io.ReadCloser, error) {
	exists, _, fileReference, err := ft.File(path, true)
	if err != nil {
		return nil, err
	}
	if !exists && fileReference == nil {
		return nil, fmt.Errorf("could not find file path in Tree: %s", path)
	}

	reader, err := fileCatalog.FileContents(*fileReference)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

// fetchMultipleFileContentsByPath is a common helper function for resolving the file contents for all paths from the
// file catalog relative to the given tree. If any one path does not exist in the given tree then an error is returned.
func fetchMultipleFileContentsByPath(ft *tree.FileTree, fileCatalog *FileCatalog, paths ...file.Path) (map[file.Reference]io.ReadCloser, error) {
	fileReferences := make([]file.Reference, len(paths))
	for idx, p := range paths {
		exists, _, fileReference, err := ft.File(p, true)
		if err != nil {
			return nil, err
		}
		if !exists && fileReference == nil {
			return nil, fmt.Errorf("could not find file path in Tree: %s", p)
		}

		fileReferences[idx] = *fileReference
	}

	readers, err := fileCatalog.MultipleFileContents(fileReferences...)
	if err != nil {
		return nil, err
	}
	return readers, nil
}
