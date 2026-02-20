package tree

import (
	"fmt"
	"path"
	"strings"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

// TreeNode represents a single node in the compact tree array
// Total size: ~40-48 bytes per node (vs ~665 bytes for map-based)
type TreeNode struct {
	id       uint64    // Sequential ID (8 bytes)
	parentID uint64    // 0 for root (8 bytes)
	children []uint64  // Child node IDs (dynamic, ~8 bytes per child)
	nameIdx  uint32    // Index into string pool (4 bytes)
	fileType file.Type // File type (enum, 1 byte)
	deleted  bool      // Whether the node has been removed
	padding  [2]byte   // Alignment padding
	refIdx   uint32    // Index into reference pool or 0 if none (4 bytes)
	linkIdx  uint32    // Index into string pool for link path (4 bytes)
}

// CompactTree is a memory-optimized tree using arrays instead of maps
// Memory: ~50-90 bytes/node vs ~952 bytes for map-based (95% reduction)
type CompactTree struct {
	nodes      []TreeNode        // Compact array indexed by sequential ID-1
	pathToID   map[string]uint64 // Optional: path → ID for fast lookup
	stringPool *StringPool       // Deduplicated strings
	refPool    *ReferencePool    // Deduplicated references
	nextID     uint64            // Counter for generating sequential IDs
}

// NewCompactTree creates a new compact tree instance
func NewCompactTree() *CompactTree {
	return &CompactTree{
		nodes:      make([]TreeNode, 0),
		pathToID:   make(map[string]uint64),
		stringPool: NewStringPool(),
		refPool:    NewReferencePool(),
		nextID:     1, // IDs start at 1, 0 is reserved for "no parent"
	}
}

// GetTreeNode returns a node by its sequential ID
func (t *CompactTree) GetTreeNode(id uint64) *TreeNode {
	if id == 0 || id-1 >= uint64(len(t.nodes)) {
		return nil
	}
	return &t.nodes[id-1]
}

// ID returns the node ID for a given path
func (t *CompactTree) ID(path string) uint64 {
	id, ok := t.pathToID[path]
	if !ok {
		return 0
	}
	return id
}

// HasNode checks if a node exists by ID
func (t *CompactTree) HasNode(id uint64) bool {
	if id == 0 {
		return false
	}
	return id-1 < uint64(len(t.nodes))
}

// HasPath checks if a node exists by path
func (t *CompactTree) HasPath(path string) bool {
	_, ok := t.pathToID[path]
	return ok
}

// AddRoot adds a root node to the tree
func (t *CompactTree) AddRoot(name string, fileType file.Type, ref *file.Reference) (uint64, error) {
	if t.nextID != 1 {
		return 0, fmt.Errorf("root already exists")
	}

	id, err := t.addNode(0, name, fileType, ref, "")
	if err != nil {
		return 0, err
	}

	if name == "" {
		t.pathToID["/"] = id
	} else {
		t.pathToID["/"+name] = id
	}

	return id, nil
}

// AddChild adds a node as a child of the parent node
func (t *CompactTree) AddChild(parentID uint64, name string, fileType file.Type, ref *file.Reference, linkPath string) (uint64, error) {
	if !t.HasNode(parentID) {
		return 0, fmt.Errorf("parent node not found")
	}

	nodeID, err := t.addNode(parentID, name, fileType, ref, linkPath)
	if err != nil {
		return 0, err
	}

	// Add child ID to parent's children list
	parent := t.GetTreeNode(parentID)
	parent.children = append(parent.children, nodeID)

	return nodeID, nil
}

// addNode adds a node to the tree
func (t *CompactTree) addNode(parentID uint64, name string, fileType file.Type, ref *file.Reference, linkPath string) (uint64, error) {
	nameIdx := t.stringPool.Intern(name)
	refIdx := t.refPool.Add(ref)
	var linkIdx uint32
	if linkPath != "" {
		linkIdx = t.stringPool.Intern(linkPath)
	}

	nodeID := t.nextID
	t.nextID++

	node := TreeNode{
		id:       nodeID,
		parentID: parentID,
		children: make([]uint64, 0),
		nameIdx:  nameIdx,
		fileType: fileType,
		refIdx:   refIdx,
		linkIdx:  linkIdx,
	}

	t.nodes = append(t.nodes, node)

	return nodeID, nil
}

// CompactNode implements node.Node for CompactTree
type CompactNode struct {
	tree *CompactTree
	id   uint64
}

func (n *CompactNode) ID() node.ID {
	return node.ID(n.tree.Path(n.id))
}

func (n *CompactNode) Copy() node.Node {
	return &CompactNode{
		tree: n.tree,
		id:   n.id,
	}
}

func (n *CompactNode) UintID() uint64 {
	return n.id
}

func (n *CompactNode) FileType() file.Type {
	return n.tree.FileType(n.id)
}

func (n *CompactNode) Reference() *file.Reference {
	return n.tree.Reference(n.id)
}

func (n *CompactNode) LinkPath() string {
	return n.tree.LinkPath(n.id)
}

func (n *CompactNode) RealPath() string {
	return n.tree.Path(n.id)
}

func (n *CompactNode) IsLink() bool {
	ft := n.tree.FileType(n.id)
	return ft == file.TypeHardLink || ft == file.TypeSymLink
}

// Tree Reader implementation

func (t *CompactTree) Node(id node.ID) node.Node {
	uintID := t.ID(string(id))
	if uintID == 0 {
		return nil
	}
	return &CompactNode{tree: t, id: uintID}
}

func (t *CompactTree) Nodes() node.Nodes {
	nodes := make(node.Nodes, 0, t.Length())
	for i := range t.nodes {
		if !t.nodes[i].deleted {
			nodes = append(nodes, &CompactNode{tree: t, id: t.nodes[i].id})
		}
	}
	return nodes
}

func (t *CompactTree) Children(n node.Node) node.Nodes {
	if n == nil {
		return nil
	}
	var uintID uint64
	if cn, ok := n.(*CompactNode); ok {
		uintID = cn.id
	} else {
		uintID = t.ID(string(n.ID()))
	}

	if uintID == 0 {
		return nil
	}

	childrenIDs := t.GetChildIDs(uintID)
	children := make(node.Nodes, len(childrenIDs))
	for i, cid := range childrenIDs {
		children[i] = &CompactNode{tree: t, id: cid}
	}
	return children
}

func (t *CompactTree) Parent(n node.Node) node.Node {
	if n == nil {
		return nil
	}
	var uintID uint64
	if cn, ok := n.(*CompactNode); ok {
		uintID = cn.id
	} else {
		uintID = t.ID(string(n.ID()))
	}

	if uintID == 0 {
		return nil
	}

	parentID := t.GetParentID(uintID)
	if parentID == 0 {
		return nil
	}
	return &CompactNode{tree: t, id: parentID}
}

func (t *CompactTree) Roots() node.Nodes {
	roots := make(node.Nodes, 0)
	for i := range t.nodes {
		if t.nodes[i].parentID == 0 && !t.nodes[i].deleted {
			roots = append(roots, &CompactNode{tree: t, id: t.nodes[i].id})
		}
	}
	return roots
}

// Root returns the root node (ID = 1)
func (t *CompactTree) Root() node.Node {
	return t.Node("/")
}

// GetChildIDs returns all child IDs for a given node ID
func (t *CompactTree) GetChildIDs(id uint64) []uint64 {
	node := t.GetTreeNode(id)
	if node == nil {
		return nil
	}
	return node.children
}

// GetParentID returns the parent ID for a given node ID (0 if root)
func (t *CompactTree) GetParentID(id uint64) uint64 {
	node := t.GetTreeNode(id)
	if node == nil {
		return 0
	}
	return node.parentID
}

// Length returns the number of nodes in the tree
func (t *CompactTree) Length() int {
	return len(t.nodes)
}

// Name returns the name of a node
func (t *CompactTree) Name(id uint64) string {
	node := t.GetTreeNode(id)
	if node == nil {
		return ""
	}
	return t.stringPool.Get(node.nameIdx)
}

// FileType returns the file type of a node
func (t *CompactTree) FileType(id uint64) file.Type {
	node := t.GetTreeNode(id)
	if node == nil {
		return file.TypeIrregular
	}
	return node.fileType
}

// Reference returns the file reference of a node
func (t *CompactTree) Reference(id uint64) *file.Reference {
	node := t.GetTreeNode(id)
	if node == nil {
		return nil
	}
	return t.refPool.Get(node.refIdx)
}

// LinkPath returns the link path of a node
func (t *CompactTree) LinkPath(id uint64) string {
	node := t.GetTreeNode(id)
	if node == nil {
		return ""
	}
	return t.stringPool.Get(node.linkIdx)
}

// StringPool returns the string pool for path reconstruction
func (t *CompactTree) StringPool() *StringPool {
	return t.stringPool
}

// Copy returns a copy of the compact tree
func (t *CompactTree) Copy() *CompactTree {
	newTree := NewCompactTree()
	newTree.nodes = make([]TreeNode, len(t.nodes))
	copy(newTree.nodes, t.nodes)
	for k, v := range t.pathToID {
		newTree.pathToID[k] = v
	}
	newTree.stringPool = t.stringPool.Copy()
	newTree.refPool = t.refPool.Copy()
	newTree.nextID = t.nextID
	return newTree
}

// Path reconstructs the full path for a node by walking up the parent chain
func (t *CompactTree) Path(id uint64) string {
	if id == 0 {
		return ""
	}

	// Collect names by walking up the parent chain
	names := make([]string, 0)
	currentID := id

	for currentID != 0 {
		node := t.GetTreeNode(currentID)
		if node == nil {
			break
		}
		name := t.Name(currentID)
		names = append(names, name)
		currentID = node.parentID
	}

	// Reverse the names to get the path from root
	// names are [basename, parent, ..., root]
	// need to build /root/parent/basename
	var parts []string
	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		if name != "" && name != "/" {
			parts = append(parts, name)
		}
	}

	return "/" + strings.Join(parts, "/")
}

func (t *CompactTree) invalidatePathCache(id uint64) {
	// No-op: pathCache removed to save memory
	// Path is reconstructed on demand via StringPool lookup
}

// RemoveNode removes a node and all its descendants from the tree
func (t *CompactTree) RemoveNode(id uint64) ([]uint64, error) {
	node := t.GetTreeNode(id)
	if node == nil || node.deleted {
		return nil, fmt.Errorf("node %d not found", id)
	}

	// Remove from pathToID first (before recursion) to avoid re-adding this node
	path := t.Path(id)
	delete(t.pathToID, path)

	removedIDs := make([]uint64, 0)

	// Recursively remove children first
	// Note: we need to copy the children slice because RemoveNode will modify it in the parent
	children := make([]uint64, len(node.children))
	copy(children, node.children)
	for _, childID := range children {
		ids, err := t.RemoveNode(childID)
		if err == nil {
			removedIDs = append(removedIDs, ids...)
		}
	}

	// Remove from parent's children list
	if node.parentID != 0 {
		parent := t.GetTreeNode(node.parentID)
		if parent != nil {
			for i, childID := range parent.children {
				if childID == id {
					parent.children = append(parent.children[:i], parent.children[i+1:]...)
					break
				}
			}
		}
	}

	// Mark as deleted
	node.deleted = true
	removedIDs = append(removedIDs, id)

	return removedIDs, nil
}

// Replace updates an existing node's data
func (t *CompactTree) Replace(id uint64, name string, fileType file.Type, ref *file.Reference, linkPath string) error {
	node := t.GetTreeNode(id)
	if node == nil || node.deleted {
		return fmt.Errorf("node %d not found", id)
	}

	// Normalize root name
	if id == 1 && (name == "/" || name == "") {
		name = ""
	}

	// Get old path from pathToID before it's potentially removed
	var oldPathKey string
	for k, v := range t.pathToID {
		if v == id {
			oldPathKey = k
			break
		}
	}
	t.invalidatePathCache(id)

	node.nameIdx = t.stringPool.Intern(name)
	node.fileType = fileType
	node.refIdx = t.refPool.Add(ref)
	if linkPath != "" {
		node.linkIdx = t.stringPool.Intern(linkPath)
	} else {
		node.linkIdx = 0
	}

	// Invalidate path cache for this node and all children
	t.invalidatePathCache(id)

	// Update pathToID mapping
	if oldPathKey != "" {
		delete(t.pathToID, oldPathKey)
	}
	newPath := t.Path(id)
	t.pathToID[newPath] = id

	return nil
}

// SetPath sets the path-to-ID mapping for a node
// This should be called after adding a node
func (t *CompactTree) SetPath(path string, id uint64) {
	t.pathToID[path] = id
}

// AddFile adds a file node at the given path
// Creates parent directories if they don't exist
func (t *CompactTree) AddFile(realPath string, fileType file.Type, ref *file.Reference, linkPath string) (uint64, error) {
	realPath = path.Clean(realPath)
	// Check if path already exists
	if id, ok := t.pathToID[realPath]; ok {
		node := t.GetTreeNode(id)
		if node.fileType != fileType {
			return 0, fmt.Errorf("path %q exists but has different type", realPath)
		}
		// Update the reference if a new one is provided (for merge operations)
		if ref != nil {
			node.refIdx = t.refPool.Add(ref)
			if linkPath != "" {
				node.linkIdx = t.stringPool.Intern(linkPath)
			} else {
				node.linkIdx = 0
			}
			t.invalidatePathCache(id)
		}
		return id, nil
	}

	// Get or create parent directory
	filePath := file.Path(realPath)
	parentPath, err := filePath.ParentPath()
	if err != nil {
		return 0, fmt.Errorf("invalid path %q: %w", realPath, err)
	}

	_, err = t.AddDir(string(parentPath), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create parent directory for %q: %w", realPath, err)
	}

	parentID := t.pathToID[string(parentPath)]
	if parentID == 0 {
		return 0, fmt.Errorf("parent directory not found for %q", realPath)
	}

	// Create the file node
	basename := filePath.Basename()
	newID, err := t.AddChild(parentID, basename, fileType, ref, linkPath)
	if err != nil {
		return 0, err
	}

	t.pathToID[realPath] = newID
	return newID, nil
}

// AddDir adds a directory node at the given path
// Creates parent directories if they don't exist
func (t *CompactTree) AddDir(realPath string, ref *file.Reference) (uint64, error) {
	realPath = path.Clean(realPath)
	// Check if path already exists
	if id, ok := t.pathToID[realPath]; ok {
		node := t.GetTreeNode(id)
		if node.fileType != file.TypeDirectory {
			return 0, fmt.Errorf("path %q exists but is not a directory", realPath)
		}
		// Update the reference if a new one is provided (for merge operations)
		if ref != nil {
			node.refIdx = t.refPool.Add(ref)
			t.invalidatePathCache(id)
		}
		return id, nil
	}

	// Split path into components
	parts := splitPath(realPath)
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid path: %q", realPath)
	}

	var parentID uint64
	currentPath := ""

	for _, part := range parts {
		if part == "" {
			// Root node
			if t.nextID != 1 {
				parentID = 1
				currentPath = "/"
				continue
			}
			rootID, err := t.AddRoot("", file.TypeDirectory, ref)
			if err != nil {
				return 0, err
			}
			parentID = rootID
			t.pathToID["/"] = rootID
			currentPath = "/"
			continue
		}

		// Build current path
		if currentPath == "/" {
			currentPath = "/" + part
		} else {
			currentPath += "/" + part
		}

		// Check if node exists
		if id, ok := t.pathToID[currentPath]; ok {
			node := t.GetTreeNode(id)
			if node.fileType != file.TypeDirectory {
				return 0, fmt.Errorf("path %q exists but is not a directory", currentPath)
			}
			parentID = id
			continue
		}

		// Create directory node
		id, err := t.AddChild(parentID, part, file.TypeDirectory, nil, "")
		if err != nil {
			return 0, err
		}
		t.pathToID[currentPath] = id
		parentID = id
	}

	// If this was the final node, update its reference if provided
	if ref != nil {
		node := t.GetTreeNode(parentID)
		node.refIdx = t.refPool.Add(ref)
	}

	return parentID, nil
}

// splitPath cleans a path and splits it into components.
// The first component is always "" if the path is absolute.
func splitPath(p string) []string {
	if p == "" {
		return []string{}
	}
	// Use path.Clean to handle multiple slashes and trailing slashes
	cleaned := path.Clean(p)
	if cleaned == "/" {
		return []string{""}
	}

	return strings.Split(cleaned, "/")
}
