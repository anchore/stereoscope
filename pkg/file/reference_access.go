package file

import (
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

type ReferenceAccessVias []ReferenceAccessVia

// NewFileReferenceVia create a new ReferenceAccessVia for the given request path, showing the resolved reference (or
// nil if it does not exist), and the link resolution of the basename of the request path transitively.
func NewFileReferenceVia(path Path, ref *Reference, leafs []ReferenceAccess) *ReferenceAccessVia {
	return &ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: path,
			Reference:   ref,
		},
		LeafLinkResolution: leafs,
	}
}

func (f ReferenceAccessVias) Len() int {
	return len(f)
}

func (f ReferenceAccessVias) Less(i, j int) bool {
	ith := f[i]
	jth := f[j]

	ithIsReal := ith.Reference != nil && ith.Reference.RealPath == ith.RequestPath
	jthIsReal := jth.Reference != nil && jth.Reference.RealPath == jth.RequestPath

	switch {
	case ithIsReal && !jthIsReal:
		return true
	case !ithIsReal && jthIsReal:
		return false
	}

	return ith.RequestPath < jth.RequestPath
}

func (f ReferenceAccessVias) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
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

func (f *ReferenceAccessVia) AllRequestPaths() []Path {
	set := strset.New()
	set.Add(string(f.RequestPath))
	for _, p := range f.LeafLinkResolution {
		set.Add(string(p.RequestPath))
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
