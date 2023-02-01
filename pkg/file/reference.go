package file

import (
	"fmt"
	"sort"

	"github.com/scylladb/go-set/strset"
)

// ReferenceAccess represents the fetching of a possibly non-existent file, and how it was accessed.
type ReferenceAccess struct {
	RequestPath Path
	*Reference
}

// ReferenceAccessVia represents a possibly non-existent file, and how it was accessed, including all symlink and hardlink resolution.
type ReferenceAccessVia struct {
	ReferenceAccess
	LeafLinkResolution []ReferenceAccess
}

func (f *ReferenceAccessVia) HasReference() bool {
	if f == nil {
		return false
	}
	return f.Reference != nil
}

func (f *ReferenceAccessVia) AllPaths() []Path {
	set := strset.New()
	set.Add(string(f.RequestPath))
	if f.Reference != nil {
		set.Add(string(f.Reference.RealPath))
	}
	for _, p := range f.LeafLinkResolution {
		set.Add(string(p.RequestPath))
		if p.Reference != nil {
			set.Add(string(p.Reference.RealPath))
		}
	}

	paths := set.List()
	sort.Strings(paths)

	var results []Path
	for _, p := range paths {
		results = append(results, Path(p))
	}
	return results
}

// RequestResolutionPath represents the traversal through the filesystem to access to current reference, including all symlink and hardlink resolution.
func (f *ReferenceAccessVia) RequestResolutionPath() []Path {
	var paths []Path
	var firstPath Path
	var lastLinkResolutionIsDead bool

	if string(f.RequestPath) != "" {
		firstPath = f.RequestPath
		paths = append(paths, f.RequestPath)
	}
	for i, p := range f.LeafLinkResolution {
		if i == 0 && p.RequestPath == f.RequestPath {
			// ignore link resolution that starts with the same user requested path
			continue
		}
		if firstPath == "" {
			firstPath = p.RequestPath
		}

		paths = append(paths, p.RequestPath)

		if i == len(f.LeafLinkResolution)-1 {
			// we've reached the final link resolution
			if p.Reference == nil {
				lastLinkResolutionIsDead = true
			}
		}
	}
	if f.HasReference() && firstPath != f.Reference.RealPath && !lastLinkResolutionIsDead {
		// we've reached the final reference that was resolved
		// we should only do this if there was a link resolution
		paths = append(paths, f.Reference.RealPath)
	}
	return paths
}

// ResolutionReferences represents the traversal through the filesystem to access to current reference, including all symlink and hardlink resolution.
func (f *ReferenceAccessVia) ResolutionReferences() []Reference {
	var refs []Reference
	var lastLinkResolutionIsDead bool

	for i, p := range f.LeafLinkResolution {
		if p.Reference != nil {
			refs = append(refs, *p.Reference)
		}
		if i == len(f.LeafLinkResolution)-1 {
			// we've reached the final link resolution
			if p.Reference == nil {
				lastLinkResolutionIsDead = true
			}
		}
	}
	if f.Reference != nil && !lastLinkResolutionIsDead {
		refs = append(refs, *f.Reference)
	}
	return refs
}

// Reference represents a unique file. This is useful when path is not good enough (i.e. you have the same file path for two files in two different container image layers, and you need to be able to distinguish them apart)
type Reference struct {
	id       ID
	RealPath Path // file path with NO symlinks or hardlinks in constituent paths
}

// NewFileReferenceVia shows how a reference was accessed.
func NewFileReferenceVia(path Path, ref *Reference, leafs []ReferenceAccess) *ReferenceAccessVia {
	return &ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: path,
			Reference:   ref,
		},
		LeafLinkResolution: leafs,
	}
}

// NewFileReference creates a new unique file reference for the given path.
func NewFileReference(path Path) *Reference {
	nextID++
	return &Reference{
		RealPath: path,
		id:       ID(nextID),
	}
}

// ID returns the unique ID for this file reference.
func (f *Reference) ID() ID {
	return f.id
}

// String returns a string representation of the path with a unique ID.
func (f *Reference) String() string {
	if f == nil {
		return "[nil]"
	}
	return fmt.Sprintf("[%v] real=%q", f.id, f.RealPath)
}
