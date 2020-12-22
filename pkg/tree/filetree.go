package tree

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/anchore/stereoscope/internal"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
	"github.com/bmatcuk/doublestar/v2"
)

var ErrRemovingRoot = errors.New("cannot remove the root path (`/`) from the FileTree")

// FileTree represents a file/directory tree
type FileTree struct {
	pathToFileRef map[node.ID]*file.Reference
	tree          *tree
}

// NewFileTree creates a new FileTree instance.
func NewFileTree() *FileTree {
	tree := newTree()

	// Initialize FileTree with a root "/" node
	root := file.Path("/")
	_ = tree.AddRoot(root)

	pathToFileRef := make(map[node.ID]*file.Reference)
	pathToFileRef[root.ID()] = file.NewFileReference(root)

	return &FileTree{
		tree:          tree,
		pathToFileRef: pathToFileRef,
	}
}

// copy returns a copy of the current FileTree.
func (t *FileTree) copy() (*FileTree, error) {
	dest := NewFileTree()
	for p, ref := range t.pathToFileRef {
		pathObj := file.Path(p)
		_, err := dest.AddPath(pathObj)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			var refCopy = *ref
			err = dest.setFile(pathObj, &refCopy)
			if err != nil {
				return nil, err
			}
		} else {
			err = dest.setFile(pathObj, nil)
			if err != nil {
				return nil, err
			}
		}
	}
	return dest, nil
}

// HasPath indicates is the given path is in the file tree.
func (t *FileTree) HasPath(path file.Path) bool {
	exists, _, _ := t.file(path)
	return exists
}

// fileByPathID indicates if the given node ID is in the FileTree (useful for tree -> FileTree node resolution).
func (t *FileTree) fileByPathID(id node.ID) *file.Reference {
	return t.pathToFileRef[id]
}

// VisitorFn, used for traversal, wraps the given user function (meant to walk file.References) with a function that
// can resolve an underlying tree Node to a file.Reference.
func (t *FileTree) VisitorFn(fn func(file.Path, *file.Reference)) func(node.Node) {
	return func(node node.Node) {
		fn(file.Path(node.ID()), t.fileByPathID(node.ID()))
	}
}

// ConditionFn, used for conditioning traversal, wraps the given user function (meant to walk file.References) with a
// function that can resolve an underlying tree Node to a file.Reference.
func (t *FileTree) ConditionFn(fn func(file.Path, *file.Reference) bool) func(node.Node) bool {
	return func(node node.Node) bool {
		return fn(file.Path(node.ID()), t.fileByPathID(node.ID()))
	}
}

// AllFiles returns all files and directories within the FileTree.
func (t *FileTree) AllFiles() []file.Reference {
	files := make([]file.Reference, 0)
	for _, f := range t.pathToFileRef {
		if f != nil {
			files = append(files, *f)
		}
	}
	return files
}

func (t *FileTree) AllPaths() []file.Path {
	paths := make([]file.Path, 0)
	for p := range t.pathToFileRef {
		paths = append(paths, file.Path(p))
	}
	return paths
}

// File fetches a file.Reference for the given path. Returns nil if the path does not exist in the FileTree.
func (t *FileTree) file(path file.Path) (bool, file.Path, *file.Reference) {
	// For:             /some/path/here
	// Where:           /some/path -> /other/place
	// And resolves to: /other/place/here

	// This means a few things:
	//  - /some/path/here CANNOT exist in the tree. If it did, the parent /some/path would have to be a directory,
	//      but since we know it is a link this cannot be true.
	//  - /other/place DOES NOT need to exist in the tree --this would be a dead link and is allowable. Under this case
	//      we return NIL.
	//  - /other/place/here DOES NOT need to exist in the tree, it either
	//          a) exists as a regular file --in which case return the discovered file.Reference
	//	        b) does not exist --return NIL
	//          c) or exists as a symlink that may or may not resolve --this last case does not matter since the
	//             PATH has been resolved to a file.Reference, which is all that matters)
	//
	// Therefore we can safely lookup the path first without worrying about symlink resolution yet... if there is a
	// hit, return it! If not, fallback to symlink resolution.

	if value, ok := t.pathToFileRef[path.ID()]; ok {
		return true, path, value
	}

	// symlink resolution!... note that this is really only valid within the context of a filetree that represents a
	// squash tree (or is simply not a single union FS layer).
	exists, p, ref := t.resolvePath(path)
	return exists, p, ref
}

// File fetches a file.Reference for the given path. Returns nil if the path does not exist in the FileTree.
func (t *FileTree) File(path file.Path) (bool, *file.Reference) {
	exists, _, ref := t.file(path)
	return exists, ref
}

func (t *FileTree) resolveLinkPathToFile(thePath file.Path) (bool, file.Path, *file.Reference, error) {
	// get to the link target
	exists, p, ref := t.resolvePath(thePath)

	// keep resolving links until a regular file or directory is found
	alreadySeen := internal.NewStringSet()
	var nextRef *file.Reference
	currentRef := ref
	currentPath := p
	for {
		// if there is no next path, return this reference (dead link)
		if !exists {
			return exists, currentPath, currentRef, nil
		}

		if alreadySeen.Contains(string(currentPath)) {
			return false, "", nil, fmt.Errorf("cycle during symlink resolution: %+v", currentRef)
		}

		if currentRef != nil && currentRef.LinkPath == "" {
			// no resolution and there is no next link (pseudo dead link)... return what you found
			// any content fetches will fail, but that's ok
			break
		}

		// prepare for the next iteration
		alreadySeen.Add(string(currentPath))

		var nextPath file.Path
		if currentRef != nil {
			if currentRef.LinkPath.IsAbsolutePath() {
				// use links with absolute paths blindly
				nextPath = currentRef.LinkPath
			} else {
				// resolve relative link paths
				var parentDir string
				parentDir, _ = filepath.Split(string(currentRef.Path))
				// assemble relative link path by normalizing: "/cur/dir/../file1.txt" --> "/cur/file1.txt"
				nextPath = file.Path(filepath.Clean(path.Join(parentDir, string(currentRef.LinkPath))))
			}
		}

		// no more links to follow
		if string(nextPath) == "" {
			break
		}

		exists, _, nextRef = t.file(nextPath)
		currentRef = nextRef
		currentPath = nextPath
	}
	return true, currentPath, currentRef, nil
}

func (t *FileTree) resolvePath(path file.Path) (bool, file.Path, *file.Reference) {
	var currentPathStr string
	var currentPath file.Path
	var pathParts = strings.Split(string(path.Normalize()), file.DirSeparator)
	var ref *file.Reference
	var ok bool
	for idx, part := range pathParts {
		if (part == "" || part == file.DirSeparator) && idx == 0 {
			// note: this means that we will NEVER resolve a symlink or file.Reference for /, which is OK
			continue
		}

		// cumulatively gather where we are currently at and provide a rich object
		currentPath = file.Path(currentPathStr + file.DirSeparator + part).Normalize()
		currentPathStr = string(currentPath)

		ref, ok = t.pathToFileRef[currentPath.ID()]
		if !ok {
			// we've reached a point where the given path that has never been observed. This can happen for one reason:
			// 1. the current path is really invalid and we should return NIL indicating that it cannot be resolved.
			// 2. the current path is a link? no, this isn't possible since we are iterating through constituent paths
			//      in order, so we are guaranteed to hit parent links in which we should adjust the search path accordingly.
			return false, "", nil
		}

		// this is positively a path, however, there is no information about this node. This may be OK since we
		// allow for adding children before parents (and even don't require the parent to ever be added --which is
		// potentially valid given the underlying messy data [tar headers]). In this case we keep building the path
		// (which we've already done at this point) and continue.
		if ref == nil {
			continue
		}

		// we definitely have a file reference, which means that the file was specifically given to us by the caller.
		if ref.LinkPath != "" {
			// this is a symlink! let's process the ref to determine if this is a absolute or relative path (which would
			// mean either we will be replacing the currentPathStr we have so far [with the absolute path] or appending
			// the [relative] linkPath onto what we have so far).
			if ref.LinkPath.IsAbsolutePath() {
				currentPathStr = string(ref.LinkPath)
			} else {
				currentPathStr += file.DirSeparator + string(ref.LinkPath)
			}
		}
	}
	// by this point we have processed all constituent paths; there were no un-added paths and the path is guaranteed
	// to have followed link resolution. Let's return the file reference at this point.
	return true, currentPath, ref
}

func (t *FileTree) glob(query string) ([]string, error) {
	if len(query) == 0 {
		return nil, fmt.Errorf("no glob pattern given")
	}

	if query[0] != file.DirSeparator[0] {
		// this is for an image, so it should always be relative to root
		query = file.DirSeparator + query
	}

	matches, err := doublestar.GlobOS(&osAdapter{ft: t}, query)
	if err != nil {
		return nil, err
	}

	return matches, nil
}

// File fetches zero to many file.References for the given glob pattern.
func (t *FileTree) FilesByGlob(query string) ([]file.Reference, error) {
	result := make([]file.Reference, 0)

	matches, err := t.glob(query)
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		_, ref := t.File(file.Path(match))
		_, _, ref, err := t.resolveLinkPathToFile(file.Path(match))
		if err != nil {
			return nil, err
		}
		if ref != nil {
			result = append(result, *ref)
		}
	}

	return result, nil
}

// setFile replaces any file already in the FileTree with the given file.Reference.
func (t *FileTree) setFile(path file.Path, ref *file.Reference) error {
	if err := mustMatch(path, ref); err != nil {
		return err
	}

	_, ok := t.pathToFileRef[path.ID()]

	if !ok {
		return fmt.Errorf("file does not already exist in tree (cannot replace)")
	}

	delete(t.pathToFileRef, path.ID())

	t.pathToFileRef[path.ID()] = ref

	return nil
}

// AddPath adds a new path to the tree. It also adds any
// ancestors of the path that are not already present in the tree. The resulting
// file.Reference of the new (leaf) addition is returned.
func (t *FileTree) AddPath(path file.Path) (*file.Reference, error) {
	if f, ok := t.pathToFileRef[path.ID()]; ok {
		return f, nil
	}

	if err := t.addParentPaths(path); err != nil {
		return nil, err
	}

	f := file.NewFileReference(path)
	return f, t.addPath(path, f)
}

func (t *FileTree) AddLink(path file.Path, linkPath file.Path) (*file.Reference, error) {
	if f, ok := t.pathToFileRef[path.ID()]; ok {
		return f, nil
	}

	if err := t.addParentPaths(path); err != nil {
		return nil, err
	}

	f := file.NewFileLinkReference(path, linkPath)

	return f, t.addPath(path, f)
}

func (t *FileTree) addParentPaths(path file.Path) error {
	parent, err := path.ParentPath()
	if err != nil {
		return fmt.Errorf("unable to add path: %w", err)
	}

	if _, ok := t.pathToFileRef[parent.ID()]; !ok {
		// add parents of the node until an existent parent is found it's important to do this in reverse order
		// to ensure we are checking the fewest amount of parents possible.
		var pathsToAdd []file.Path
		parentPaths := path.ConstituentPaths()
		for idx := len(parentPaths) - 1; idx >= 0; idx-- {
			if _, ok := t.pathToFileRef[parentPaths[idx].ID()]; ok {
				break
			}
			pathsToAdd = append(pathsToAdd, parentPaths[idx])
		}

		// add each path with no file reference; add these in sorted path order (which is guaranteed to be
		// the reverse of the order of insertion)
		for idx := len(pathsToAdd) - 1; idx >= 0; idx-- {
			if err := t.addPath(pathsToAdd[idx], nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *FileTree) addPath(path file.Path, ref *file.Reference) error {
	if err := mustMatch(path, ref); err != nil {
		return err
	}

	parent, err := path.ParentPath()
	if err != nil {
		return fmt.Errorf("unable to add path: %w", err)
	}

	if !t.tree.HasNode(path.ID()) {
		// add the node to the tree
		err = t.tree.AddChild(parent, path)
		if err != nil {
			return err
		}

		// track the path for fast lookup
		t.pathToFileRef[path.ID()] = ref
	}
	return nil
}

// RemovePath deletes the file.Reference from the FileTree by the given path.
func (t *FileTree) RemovePath(path file.Path) error {
	if path.Normalize() == "/" {
		return ErrRemovingRoot
	}

	removedNodes, err := t.tree.RemoveNode(path)
	if err != nil {
		return err
	}
	for _, n := range removedNodes {
		delete(t.pathToFileRef, n.ID())
	}
	return nil
}

// RemoveChildPaths deletes all children of the given path (not including the given path).
func (t *FileTree) RemoveChildPaths(path file.Path) error {
	removedNodes := make(node.Nodes, 0)
	for _, child := range t.tree.Children(path) {
		nodes, err := t.tree.RemoveNode(child)
		if err != nil {
			return err
		}
		removedNodes = append(removedNodes, nodes...)
	}
	for _, n := range removedNodes {
		delete(t.pathToFileRef, n.ID())
	}
	return nil
}

// Reader returns a tree.Reader useful for tree traversal.
func (t *FileTree) Reader() Reader {
	return t.tree
}

// Walk takes a visitor function and invokes it for all file.References within the FileTree in depth-first ordering.
func (t *FileTree) Walk(fn func(path file.Path, f *file.Reference)) {
	visitor := t.VisitorFn(fn)
	w := NewDepthFirstWalker(t.Reader(), visitor)
	w.WalkAll()
}

// PathDiff shows the path differences between two trees (useful for testing)
func (t *FileTree) PathDiff(other *FileTree) (extra, missing []file.Path) {
	extra = make([]file.Path, 0)
	missing = make([]file.Path, 0)

	ourPaths := internal.NewStringSet()
	for p := range t.pathToFileRef {
		ourPaths.Add(string(p))
	}

	theirPaths := internal.NewStringSet()
	for p := range other.pathToFileRef {
		theirPaths.Add(string(p))
	}

	for p := range other.pathToFileRef {
		if !ourPaths.Contains(string(p)) {
			extra = append(extra, file.Path(p))
		}
	}

	for p := range t.pathToFileRef {
		if !theirPaths.Contains(string(p)) {
			missing = append(missing, file.Path(p))
		}
	}

	return
}

// Equal indicates if the two trees have the same paths or not.
func (t *FileTree) Equal(other *FileTree) bool {
	if len(t.pathToFileRef) != len(other.pathToFileRef) {
		return false
	}

	extra, missing := t.PathDiff(other)

	return len(extra) == 0 && len(missing) == 0
}

// merge takes the given tree and combines it with the current tree, preferring files in the other tree if there
// are path conflicts. This is the basis function for squashing (where the current tree is the bottom tree and the
// given tree is the top tree).
func (t *FileTree) merge(other *FileTree) {
	conditions := WalkConditions{
		ShouldContinueBranch: other.ConditionFn(func(p file.Path, f *file.Reference) bool {
			return !p.IsWhiteout()
		}),
		ShouldVisit: other.ConditionFn(func(p file.Path, f *file.Reference) bool {
			return !p.IsDirWhiteout()
		}),
	}

	visitor := other.VisitorFn(func(path file.Path, f *file.Reference) {
		// opaque directories must be processed first
		if other.hasOpaqueDirectory(path) {
			err := t.RemoveChildPaths(path)
			if err != nil {
				log.Errorf("filetree merge failed to remove child paths (path=%s): %w", path, err)
			}
		}

		if path.IsWhiteout() {
			lowerPath, err := path.UnWhiteoutPath()
			if err != nil {
				log.Errorf("filetree merge failed to find original path for whiteout (path=%s): %w", path, err)
			}

			err = t.RemovePath(lowerPath)
			if err != nil {
				log.Errorf("filetree merge failed to remove path (path=%s): %w", lowerPath, err)
			}

			return
		}

		if !t.HasPath(path) {
			if err := t.addPath(path, nil); err != nil {
				log.Errorf("filetree merge failed to add path (path=%s): %w", path, err)
			}
		}

		if f != nil {
			err := t.setFile(path, f)
			if err != nil {
				log.Errorf("filetree merge failed to set file reference (ref=%+v): %w", f, err)
			}
		}
	})

	w := NewDepthFirstWalkerWithConditions(other.Reader(), visitor, conditions)
	w.WalkAll()
}

func (t *FileTree) hasOpaqueDirectory(directoryPath file.Path) bool {
	opaqueWhiteoutChild := file.Path(path.Join(string(directoryPath), file.OpaqueWhiteout))
	return t.HasPath(opaqueWhiteoutChild)
}

func mustMatch(path file.Path, ref *file.Reference) error {
	if ref != nil && path.ID() != ref.Path.ID() {
		return fmt.Errorf("unable to add path for mismatched reference value: %+v != %+v", path.ID(), ref.Path.ID())
	}
	return nil
}
