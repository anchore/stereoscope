package filenode

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

var pathTrie *PathTrie

func init() {
	pathTrie = NewPathTrie()
}

type FileNode struct {
	//RealPath  file.Path // all constituent paths cannot have links (the base may be a link however)
	//realPath file.Path
	trieNodeID int64
	FileType   file.Type
	LinkPath   file.Path // a relative or absolute path to another file
	Reference  *file.Reference
}

func NewDir(p file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		//realPath: p,
		trieNodeID: pathTrie.Insert(string(p)),
		FileType:   file.TypeDirectory,
		Reference:  ref,
	}
}

func NewFile(p file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		//realPath: p,
		trieNodeID: pathTrie.Insert(string(p)),
		FileType:   file.TypeRegular,
		Reference:  ref,
	}
}

func NewSymLink(p, linkPath file.Path, ref *file.Reference) *FileNode {
	return &FileNode{
		//realPath: p,
		trieNodeID: pathTrie.Insert(string(p)),
		FileType:   file.TypeSymLink,
		LinkPath:   linkPath,
		Reference:  ref,
	}
}

func NewHardLink(p, linkPath file.Path, ref *file.Reference) *FileNode {
	// hard link MUST be interpreted as an absolute path
	linkPath = file.Path(path.Clean(file.DirSeparator + string(linkPath)))
	return &FileNode{
		//realPath: p,
		trieNodeID: pathTrie.Insert(string(p)),
		FileType:   file.TypeHardLink,
		LinkPath:   linkPath,
		Reference:  ref,
	}
}

func (n *FileNode) ID() node.ID {
	return IDByPath(n.RealPath())
}

func (n *FileNode) RealPath() file.Path {
	//return n.realPath
	return file.Path(pathTrie.Get(n.trieNodeID))
}

func (n *FileNode) IsLink() bool {
	return n.FileType == file.TypeHardLink || n.FileType == file.TypeSymLink
}

func IDByPath(p file.Path) node.ID {
	return node.ID(p)
}

func (n *FileNode) RenderLinkDestination() file.Path {
	if !n.IsLink() {
		return ""
	}

	if n.LinkPath.IsAbsolutePath() {
		// use links with absolute paths blindly
		return n.LinkPath
	}

	// resolve relative link paths
	var parentDir string
	parentDir, _ = filepath.Split(string(n.RealPath())) // TODO: alex: should this be path.Split, not filepath.Split?

	// assemble relative link path by normalizing: "/cur/dir/../file1.txt" --> "/cur/file1.txt"
	return file.Path(path.Clean(path.Join(parentDir, string(n.LinkPath))))
}

// PathTrie represents the trie structure
type PathTrie struct {
	root      *TrieNode
	nodeIDMap map[int64]*TrieNode
	counter   int64
	//answer    map[int64]string
}

// TrieNode represents a node in the trie
type TrieNode struct {
	children map[string]*TrieNode
	nodeID   int64
}

// NewPathTrie initializes and returns a new PathTrie
func NewPathTrie() *PathTrie {
	t := &PathTrie{
		root:      &TrieNode{children: make(map[string]*TrieNode)},
		nodeIDMap: make(map[int64]*TrieNode),
		//answer:    make(map[int64]string),
		counter: 0,
	}

	t.nodeIDMap[0] = t.root

	return t
}

// Insert inserts a path into the trie and returns the node ID
func (pt *PathTrie) Insert(p string) int64 {
	p = path.Clean(p)
	if p == "" || p == "/" {
		//pt.answer[pt.root.nodeID] = "/"
		return pt.root.nodeID
	}
	parts := strings.Split(p, "/")
	currentNode := pt.root
	for _, part := range parts {
		if part == "" {
			continue
		}
		if _, exists := currentNode.children[part]; !exists {
			pt.counter++
			currentNode.children[part] = &TrieNode{nodeID: pt.counter, children: make(map[string]*TrieNode)}
			pt.nodeIDMap[pt.counter] = currentNode
		}
		currentNode = currentNode.children[part]
	}
	//pt.answer[currentNode.nodeID] = p
	return currentNode.nodeID
}

// Get returns the path string for the given node ID
func (pt *PathTrie) Get(nodeID int64) string {
	//return path.Clean(pt.answer[nodeID])

	n, exists := pt.nodeIDMap[nodeID]
	if !exists {
		return ""
	}
	return path.Clean(pt.getPath(n))
}

// getPath traverses the trie from the node to the root to build the path string
func (pt *PathTrie) getPath(node *TrieNode) string {
	if node == pt.root {
		return "/"
	}
	for _, parentNode := range pt.nodeIDMap {
		for part, childNode := range parentNode.children {
			if childNode == node {
				parentPath := pt.getPath(parentNode)
				if parentPath == "" {
					return part
				}
				return parentPath + "/" + part
			}
		}
	}
	return ""
}
