package image

import (
	"fmt"
	"io"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// fetchFileContentsByPath is a common helper function for resolving the file contents for a path from the file
// catalog relative to the given tree.
func fetchFileContentsByPath(ft *filetree.FileTree, fileCatalog *FileCatalog, path file.Path) (io.ReadCloser, error) {
	exists, fileReference, err := ft.File(path, filetree.FollowBasenameLinks)
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

// fetchFileContentsByPath is a common helper function for resolving file references for a MIME type from the file
// catalog relative to the given tree.
func fetchFilesByMIMEType(ft *filetree.FileTree, fileCatalog *FileCatalog, mType string) ([]file.Reference, error) {
	fileEntries, err := fileCatalog.GetByMIMEType(mType)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by MIME type: %w", err)
	}

	var refs []file.Reference
	for _, entry := range fileEntries {
		_, ref, err := ft.File(entry.File.RealPath, filetree.FollowBasenameLinks)
		if err != nil {
			return nil, fmt.Errorf("unable to get ref for path=%q: %w", entry.File.RealPath, err)
		}

		// we know this entry exists in the tree, keep track of the reference for this file
		if ref != nil && ref.ID() == entry.File.ID() {
			refs = append(refs, *ref)
		}
	}
	return refs, nil
}
