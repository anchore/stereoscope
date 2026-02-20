package filetree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

// nodeAccess represents a request into the tree for a specific path and the resulting node, which may have a different path.
type nodeAccess struct {
	RequestPath        file.Path
	Node               node.Node // note: it is important that nodeAccess does not implement node.Node (then it can be added to the tree directly)
	LeafLinkResolution []nodeAccess
}

func (na *nodeAccess) HasFileNode() bool {
	if na == nil {
		return false
	}
	return na.Node != nil
}

// AsFileNode converts the underlying node to *filenode.FileNode
// This is needed for backward compatibility with the walker visitor signature
func (na *nodeAccess) AsFileNode() *filenode.FileNode {
	if !na.HasFileNode() {
		return nil
	}

	if fn, ok := na.Node.(*filenode.FileNode); ok {
		return fn
	}

	// Convert CompactNode to FileNode
	return &filenode.FileNode{
		RealPath:  getNodeRealPath(na.Node),
		FileType:  getNodeFileType(na.Node),
		LinkPath:  getNodeLinkPath(na.Node),
		Reference: getNodeReference(na.Node),
	}
}

func (na *nodeAccess) FileResolution() *file.Resolution {
	if !na.HasFileNode() {
		return nil
	}
	return file.NewResolution(
		na.RequestPath,
		getNodeReference(na.Node),
		newResolutions(na.LeafLinkResolution),
	)
}

func (na *nodeAccess) References() []file.Reference {
	if !na.HasFileNode() {
		return nil
	}
	var refs []file.Reference

	ref := getNodeReference(na.Node)
	if ref != nil {
		refs = append(refs, *ref)
	}

	for _, l := range na.LeafLinkResolution {
		if l.HasFileNode() {
			ref := getNodeReference(l.Node)
			if ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	return refs
}

func (na *nodeAccess) FileType() file.Type {
	if !na.HasFileNode() {
		return file.TypeIrregular
	}
	return getNodeFileType(na.Node)
}

func (na *nodeAccess) RealPath() file.Path {
	if !na.HasFileNode() {
		return ""
	}
	return getNodeRealPath(na.Node)
}

func (na *nodeAccess) LinkPath() file.Path {
	if !na.HasFileNode() {
		return ""
	}
	return getNodeLinkPath(na.Node)
}

func (na *nodeAccess) IsLink() bool {
	if !na.HasFileNode() {
		return false
	}
	return isNodeLink(na.Node)
}
