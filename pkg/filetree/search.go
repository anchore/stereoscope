package filetree

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/bmatcuk/doublestar/v4"
)

// Searcher is a facade for searching a file tree with optional indexing support.
type Searcher interface {
	SearchByPath(path string, options ...LinkResolutionOption) (*file.ReferenceAccessVia, error)
	SearchByGlob(patterns string, options ...LinkResolutionOption) ([]file.ReferenceAccessVia, error)
	SearchByMIMEType(mimeTypes ...string) ([]file.ReferenceAccessVia, error)
}

type searchContext struct {
	tree  Reader // this is the tree which all index search results are filtered against
	index Index  // this index is relative to one or more trees, not just necessarily one
}

func NewSearchContext(tree Reader, index Index) Searcher {
	return &searchContext{
		tree:  tree,
		index: index,
	}
}

func (i searchContext) SearchByPath(path string, options ...LinkResolutionOption) (*file.ReferenceAccessVia, error) {
	// TODO: one day this could leverage indexes outside of the tree, but today this is not implemented
	options = append(options, FollowBasenameLinks)
	_, ref, err := i.tree.File(file.Path(path), options...)
	return ref, err
}

func (i searchContext) SearchByMIMEType(mimeTypes ...string) ([]file.ReferenceAccessVia, error) {
	var fileEntries []IndexEntry

	for _, mType := range mimeTypes {
		entries, err := i.index.GetByMIMEType(mType)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch file references by MIME type (%q): %w", mType, err)
		}
		fileEntries = append(fileEntries, entries...)
	}

	return i.filterIndexEntriesRelativeToTree(fileEntries)
}

func (i searchContext) SearchByGlob(pattern string, options ...LinkResolutionOption) ([]file.ReferenceAccessVia, error) {
	if i.index == nil {
		options = append(options, FollowBasenameLinks)
		return i.tree.FilesByGlob(pattern, options...)
	}

	return i.searchByGlob(parseGlob(pattern), options...)
}

func (i searchContext) searchByGlob(request searchRequest, options ...LinkResolutionOption) ([]file.ReferenceAccessVia, error) {
	switch request.searchBasis {
	case searchByPath:
		options = append(options, FollowBasenameLinks)
		ref, err := i.SearchByPath(request.value, options...)
		if err != nil {
			return nil, err
		}
		if ref == nil {
			return nil, nil
		}
		return []file.ReferenceAccessVia{*ref}, nil
	case searchByBasename:
		indexes, err := i.index.GetByBasename(request.value)
		if err != nil {
			return nil, fmt.Errorf("unable to search by basename=%q: %w", request.value, err)
		}
		refs, err := i.filterIndexEntries(request.requirement, indexes)
		if err != nil {
			return nil, err
		}
		return refs, nil
	case searchByBasenameGlob:
		indexes, err := i.index.GetByBasenameGlob(request.value)
		if err != nil {
			return nil, fmt.Errorf("unable to search by basename-glob=%q: %w", request.value, err)
		}
		refs, err := i.filterIndexEntries(request.requirement, indexes)
		if err != nil {
			return nil, err
		}
		return refs, nil
	case searchByExtension:
		indexes, err := i.index.GetByExtension(request.value)
		if err != nil {
			return nil, fmt.Errorf("unable to search by extension=%q: %w", request.value, err)
		}
		refs, err := i.filterIndexEntries(request.requirement, indexes)
		if err != nil {
			return nil, err
		}
		return refs, nil
	case searchByGlob:
		options = append(options, FollowBasenameLinks)
		return i.tree.FilesByGlob(request.value, options...)
	}

	return nil, fmt.Errorf("invalid search request: %+v", request.searchBasis)
}

func (i searchContext) filterIndexEntries(requirement string, entries []IndexEntry) ([]file.ReferenceAccessVia, error) {
	refs, err := i.filterIndexEntriesRelativeToTree(entries)
	if err != nil {
		return nil, err
	}

	var results []file.ReferenceAccessVia
	for _, ref := range refs {
		if requirement != "" {
			var foundMatchingRequirement bool
			for _, p := range ref.AllPaths() {
				matched, err := doublestar.Match(requirement, string(p))
				if err != nil {
					return nil, fmt.Errorf("unable to match glob pattern=%q to path=%q: %w", requirement, p, err)
				}
				if matched {
					foundMatchingRequirement = true
					break
				}
			}
			if !foundMatchingRequirement {
				continue
			}
		}
		results = append(results, ref)
	}

	return results, nil
}

func (i searchContext) filterIndexEntriesRelativeToTree(fileEntries []IndexEntry) ([]file.ReferenceAccessVia, error) {
	var refs []file.ReferenceAccessVia
allFileEntries:
	for _, entry := range fileEntries {
		_, ref, err := i.tree.File(entry.Reference.RealPath, FollowBasenameLinks)
		if err != nil {
			return nil, fmt.Errorf("unable to get ref for path=%q: %w", entry.Reference.RealPath, err)
		}

		if !ref.HasReference() {
			continue
		}

		for _, accessRef := range ref.ResolutionReferences() {
			if accessRef.ID() == entry.Reference.ID() {
				// we know this entry exists in the tree, keep track of the reference for this file
				refs = append(refs, *ref)
				continue allFileEntries
			}
		}

		// we did not find a matching file ID in the tree, so drop this entry
	}
	return refs, nil
}
