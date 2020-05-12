package image

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
)

func fetchFileContents(filetree *tree.FileTree, fileCatalog *FileCatalog, path file.Path) (string, error) {
	fileReference := filetree.File(path)
	if fileReference == nil {
		return "", fmt.Errorf("could not find file path in Tree: %s", path)
	}

	content, err := fileCatalog.FileContents(*fileReference)
	if err != nil {
		return "", err
	}
	return content, nil
}

func fetchMultipleFileContents(filetree *tree.FileTree, fileCatalog *FileCatalog, paths ...file.Path) (map[file.Reference]string, error) {
	fileReferences := make([]file.Reference, len(paths))
	for idx, path := range paths {
		fileReference := filetree.File(path)
		if fileReference == nil {
			return nil, fmt.Errorf("could not find file path in Tree: %s", path)
		}
		fileReferences[idx] = *fileReference
	}

	content, err := fileCatalog.MultipleFileContents(fileReferences...)
	if err != nil {
		return nil, err
	}
	return content, nil
}
