package query

import (
	"fmt"
	"io"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// FileContentsByPath is a common helper function for resolving the file contents for a path from the file
// catalog relative to the given tree.
func FileContentsByPath(ft *filetree.FileTree, fileIndex *file.Index, path file.Path) (io.ReadCloser, error) {
	exists, fileReference, err := ft.File(path, filetree.FollowBasenameLinks)
	if err != nil {
		return nil, err
	}
	if !exists && fileReference == nil {
		return nil, fmt.Errorf("could not find file path in Tree: %s", path)
	}

	reader, err := fileIndex.FileContents(*fileReference)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

// FilesByMIMEType is a common helper function for resolving file references for a MIME type from the file
// catalog relative to the given tree.
func FilesByMIMEType(ft *filetree.FileTree, fileIndex *file.Index, mType string) ([]file.Reference, error) {
	fileEntries, err := fileIndex.GetByMIMEType(mType)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by MIME type (%q): %w", mType, err)
	}

	// since this query is related to the contents of the path, this should be a strict file ID match
	return filterCatalogFilesRelativesToTree(ft, fileEntries, true, filetree.FollowBasenameLinks)
}

// FilesByExtension is a common helper function for resolving file references for a file extension from the file
// catalog relative to the given tree.
func FilesByExtension(ft *filetree.FileTree, fileIndex *file.Index, extension string) ([]file.Reference, error) {
	fileEntries, err := fileIndex.GetByExtension(extension)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by extension (%q): %w", extension, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, false, filetree.FollowBasenameLinks)
}

// FilesByBasename is a common helper function for resolving file references for a file basename
// catalog relative to the given tree.
func FilesByBasename(ft *filetree.FileTree, fileIndex *file.Index, basename string) ([]file.Reference, error) {
	fileEntries, err := fileIndex.GetByBasename(basename)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by basename (%q): %w", basename, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, false, filetree.FollowBasenameLinks)
}

// FilesByBasenameGlob is a common helper function for resolving file references for a file basename glob pattern
// catalog relative to the given tree.
func FilesByBasenameGlob(ft *filetree.FileTree, fileIndex *file.Index, basenameGlob string) ([]file.Reference, error) {
	fileEntries, err := fileIndex.GetByBasenameGlob(basenameGlob)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by basename glob (%q): %w", basenameGlob, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, false, filetree.FollowBasenameLinks)
}

func filterCatalogFilesRelativesToTree(ft *filetree.FileTree, fileEntries []file.IndexEntry, strictFileID bool, linkResolutionOpts ...filetree.LinkResolutionOption) ([]file.Reference, error) {
	var refs []file.Reference
	for _, entry := range fileEntries {
		_, ref, err := ft.File(entry.Reference.RealPath, linkResolutionOpts...)
		if err != nil {
			return nil, fmt.Errorf("unable to get ref for path=%q: %w", entry.Reference.RealPath, err)
		}

		if ref == nil {
			continue
		}

		if strictFileID && ref.ID() != entry.Reference.ID() {
			continue
		}

		// we know this entry exists in the tree, keep track of the reference for this file
		refs = append(refs, *ref)
	}
	return refs, nil
}
