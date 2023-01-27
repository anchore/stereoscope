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
	exists, refVia, err := ft.File(path, filetree.FollowBasenameLinks)
	if err != nil {
		return nil, err
	}
	if !exists && refVia == nil || refVia.Reference == nil {
		return nil, fmt.Errorf("could not find file path in Tree: %s", path)
	}

	reader, err := fileCatalog.FileContents(*refVia.Reference)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

// fetchFileContentsByPath is a common helper function for resolving file references for a MIME type from the file
// catalog relative to the given tree.
func fetchFilesByMIMEType(ft *filetree.FileTree, fileCatalog *FileCatalog, mType string) ([]file.ReferenceAccessVia, error) {
	fileEntries, err := fileCatalog.GetByMIMEType(mType)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by MIME type (%q): %w", mType, err)
	}

	// since this query is related to the contents of the path, this should be a strict file ID match
	return filterCatalogFilesRelativesToTree(ft, fileEntries, filetree.FollowBasenameLinks)
}

// fetchFilesByExtension is a common helper function for resolving file references for a file extension from the file
// catalog relative to the given tree.
func fetchFilesByExtension(ft *filetree.FileTree, fileCatalog *FileCatalog, extension string) ([]file.ReferenceAccessVia, error) {
	fileEntries, err := fileCatalog.GetByExtension(extension)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by extension (%q): %w", extension, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, filetree.FollowBasenameLinks)
}

// fetchFilesByBasename is a common helper function for resolving file references for a file basename
// catalog relative to the given tree.
func fetchFilesByBasename(ft *filetree.FileTree, fileCatalog *FileCatalog, basename string) ([]file.ReferenceAccessVia, error) {
	fileEntries, err := fileCatalog.GetByBasename(basename)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by basename (%q): %w", basename, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, filetree.FollowBasenameLinks)
}

// fetchFilesByBasenameGlob is a common helper function for resolving file references for a file basename glob pattern
// catalog relative to the given tree.
func fetchFilesByBasenameGlob(ft *filetree.FileTree, fileCatalog *FileCatalog, basenameGlobs ...string) ([]file.ReferenceAccessVia, error) {
	fileEntries, err := fileCatalog.GetByBasenameGlob(basenameGlobs...)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch file references by basename glob (%q): %w", basenameGlobs, err)
	}

	return filterCatalogFilesRelativesToTree(ft, fileEntries, filetree.FollowBasenameLinks)
}

func filterCatalogFilesRelativesToTree(ft *filetree.FileTree, fileEntries []FileCatalogEntry, linkResolutionOpts ...filetree.LinkResolutionOption) ([]file.ReferenceAccessVia, error) {
	var refs []file.ReferenceAccessVia
allFileEntries:
	for _, entry := range fileEntries {
		_, ref, err := ft.File(entry.File.RealPath, linkResolutionOpts...)
		if err != nil {
			return nil, fmt.Errorf("unable to get ref for path=%q: %w", entry.File.RealPath, err)
		}

		// TODO: alex think if this is correct
		// if !ref.HasReference() {
		if ref == nil {
			continue
		}

		for _, accessRef := range ref.ResolutionReferences() {
			if accessRef.ID() == entry.File.ID() {
				// we know this entry exists in the tree, keep track of the reference for this file
				refs = append(refs, *ref)
				continue allFileEntries
			}
		}

		// we did not find a matching file ID in the tree, so drop this entry
	}
	return refs, nil
}
