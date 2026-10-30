package tree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

// Replace onto an ID already in the tree must fail and leave the tree as it was
func TestTree_Replace_IDCollision(t *testing.T) {
	root, a, b := newTestNode("root"), newTestNode("a"), newTestNode("b")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, a))
	require.NoError(t, tr.AddChild(root, b))

	require.ErrorContains(t, tr.Replace(a, b), "node ID collision")
	assert.Equal(t, 3, tr.Length())
	assert.ElementsMatch(t, node.Nodes{a, b}, tr.Children(root))
	assert.Equal(t, root, tr.Parent(a))
	assert.Equal(t, root, tr.Parent(b))
}

// the PR description says replacing a root with a different ID now works (it panicked before)
func TestTree_Replace_RootWithNewID(t *testing.T) {
	root, a, newRoot := newTestNode("root"), newTestNode("a"), newTestNode("root2")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, a))

	require.NoError(t, tr.Replace(root, newRoot))
	assert.Equal(t, node.Nodes{newRoot}, tr.Roots())
	assert.Equal(t, newRoot, tr.Parent(a))
	assert.Equal(t, node.Nodes{a}, tr.Children(newRoot))
	assert.False(t, tr.HasNode(root.ID()))
}

// the PR description says Children returns nil, not an empty slice, for a node with no children
func TestTree_Children_NilAfterLastChildRemoved(t *testing.T) {
	root, a := newTestNode("root"), newTestNode("a")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, a))
	_, err := tr.RemoveNode(a)
	require.NoError(t, err)

	assert.Nil(t, tr.Children(root))
	assert.Nil(t, tr.Copy().Children(root))
}

// the PR description says Parent returns the current node: after a same-ID Replace the child's parent is the new
// object, not the one it replaced (main kept a pointer to the old one)
func TestTree_Parent_ReturnsReplacedNode(t *testing.T) {
	root, a := newTestNode("root"), newTestNode("a")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, a))

	replacement := newTestNode("root")
	require.NoError(t, tr.Replace(root, replacement))
	assert.Same(t, replacement, tr.Parent(a))
}

// Copy drops child sets that RemoveNode left empty; adding a child to that node in the copy must create a set of the
// copy's own, leaving the original's untouched
func TestTree_Copy_IsolatedAfterChildSetEmptied(t *testing.T) {
	root, a, b := newTestNode("root"), newTestNode("a"), newTestNode("b")
	original := NewTree()
	require.NoError(t, original.AddRoot(root))
	require.NoError(t, original.AddChild(root, a))
	_, err := original.RemoveNode(a)
	require.NoError(t, err)

	cp := original.Copy()
	require.NoError(t, cp.AddChild(root, b))
	require.NoError(t, original.AddChild(root, a))

	assert.Equal(t, node.Nodes{b}, cp.Children(root))
	assert.Equal(t, node.Nodes{a}, original.Children(root))
	assert.False(t, original.HasNode(b.ID()))
	assert.False(t, cp.HasNode(a.ID()))
}

// AddChild of a node that already has a parent leaves it in the old parent's child set (pre-existing, split out by the
// author to fix/add-child-reparent). With edges stored as IDs, removing both parents then reaches an ID that is no
// longer in the tree: main returned an error here, the PR head panics.
func TestTree_RemoveNode_StaleChildEdgeDoesNotPanic(t *testing.T) {
	root, p1, p2, x := newTestNode("r"), newTestNode("p1"), newTestNode("p2"), newTestNode("x")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, p1))
	require.NoError(t, tr.AddChild(root, p2))
	require.NoError(t, tr.AddChild(p1, x))
	require.NoError(t, tr.AddChild(p2, x)) // reparents x; p1 keeps a stale edge to it

	_, err := tr.RemoveNode(p2)
	require.NoError(t, err)
	assert.NotPanics(t, func() {
		_, _ = tr.RemoveNode(p1)
	})
}
