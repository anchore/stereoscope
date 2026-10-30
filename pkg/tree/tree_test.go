package tree

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

type testNode struct {
	Id node.ID
}

func toId(i any) node.ID {
	return node.ID(fmt.Sprintf("%v", i))
}

func newTestNode(id any) *testNode {
	return &testNode{
		Id: toId(id),
	}
}

func (n *testNode) ID() node.ID {
	return n.Id
}

func (n *testNode) Copy() node.Node {
	return newTestNode(n.Id)
}

func TestTree_AddRoot(t *testing.T) {
	rootIds := []int{1, 3}
	tr := NewTree()

	for _, id := range rootIds {
		err := tr.AddRoot(newTestNode(id))
		if err != nil {
			t.Fatal(fmt.Sprintf("could not add root node (%v)", id), err)
		}

		if !tr.HasNode(toId(id)) {
			t.Errorf("could not find root node (%v)", id)
		}

		found := false
	roots:
		for _, root := range tr.Roots() {
			if root.ID() == toId(id) {
				found = true
				break roots
			}
		}
		if !found {
			t.Errorf("could not find root in Tree.Roots() (%v)", id)
		}
	}

	roots := tr.Roots()
	if len(roots) != 2 {
		t.Error("unexpected number of root nodes", len(roots))
	}
}

func TestTree_AddChild(t *testing.T) {
	tr := NewTree()

	zero, one := newTestNode(0), newTestNode(1)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair", err)
	}

	children := tr.Children(zero)
	if len(children) != 1 {
		t.Fatal("unexpected length of child nodes", len(children))
	}

	if children[0].ID() != one.ID() {
		t.Fatal("unexpected child id")
	}
}

func TestTree_AddChild_Nested(t *testing.T) {
	tr := NewTree()

	zero, one, two, three := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair (0-1)", err)
	}

	err = tr.AddChild(zero, two)
	if err != nil {
		t.Fatal("could not add node pair (0-2)", err)
	}

	err = tr.AddChild(two, three)
	if err != nil {
		t.Fatal("could not add node pair (2-3)", err)
	}

	children := tr.Children(zero)
	if len(children) != 2 {
		t.Fatal("unexpected length of child nodes", len(children))
	}

	if !node.Nodes([]node.Node{one, two}).Equal(children) {
		t.Fatal("unexpected children", children)
	}

	children = tr.Children(two)
	if len(children) != 1 {
		t.Fatal("unexpected length of child node (node:2)", children)
	}

	if children[0].ID() != three.ID() {
		t.Fatal("unexpected child id (!=3)")
	}
}

func TestTree_RemoveNode(t *testing.T) {
	tr := NewTree()

	zero, one := newTestNode(0), newTestNode(1)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair", err)
	}

	removedNodes, err := tr.RemoveNode(one)
	if err != nil {
		t.Fatal("could not remove node", err)
	}

	if len(removedNodes) != 1 {
		t.Fatal("unexpected number of removed nodes", len(removedNodes))
	}

	if !removedNodes.Equal([]node.Node{one}) {
		t.Fatal("unexpected removed nodes", removedNodes)
	}

	children := tr.Children(zero)
	if len(children) != 0 {
		t.Fatal("unexpected length of child nodes", len(children))
	}

}

func TestTree_RemoveNode_Nested(t *testing.T) {
	tr := NewTree()

	zero, one, two, three := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair (0-1)", err)
	}

	err = tr.AddChild(zero, two)
	if err != nil {
		t.Fatal("could not add node pair (0-2)", err)
	}

	err = tr.AddChild(two, three)
	if err != nil {
		t.Fatal("could not add node pair (2-3)", err)
	}

	removedNodes, err := tr.RemoveNode(two)
	if err != nil {
		t.Fatal("could not remove node", err)
	}

	if len(removedNodes) != 2 {
		t.Fatal("unexpected number of removed nodes", len(removedNodes))
	}

	if !removedNodes.Equal([]node.Node{two, three}) {
		t.Fatal("unexpected removed nodes", removedNodes)
	}

	children := tr.Children(zero)
	if len(children) != 1 {
		t.Fatal("unexpected length of child nodes", len(children))
	}

	if children[0].ID() != one.ID() {
		t.Fatal("unexpected child id")
	}

}

func TestTree_RemoveNode_Root(t *testing.T) {
	tr := NewTree()

	zero, one, two, three := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair (0-1)", err)
	}

	err = tr.AddChild(zero, two)
	if err != nil {
		t.Fatal("could not add node pair (0-2)", err)
	}

	err = tr.AddChild(two, three)
	if err != nil {
		t.Fatal("could not add node pair (2-3)", err)
	}

	removedNodes, err := tr.RemoveNode(zero)
	if err != nil {
		t.Fatal("could not remove node", err)
	}

	if len(removedNodes) != 4 {
		t.Fatal("unexpected number of removed nodes", len(removedNodes))
	}

	if !removedNodes.Equal([]node.Node{zero, one, two, three}) {
		t.Fatal("unexpected removed nodes", removedNodes)
	}

	nodes := tr.Nodes()
	if len(nodes) != 0 {
		t.Fatal("unexpected length of Tree nodes", len(nodes))
	}

	for _, id := range []int{0, 1, 2, 3} {
		if tr.HasNode(toId(id)) {
			t.Fatal("node should no longer be part of the Tree", id)
		}
	}

}

func TestTree_Replace(t *testing.T) {
	tr := NewTree()

	zero, one, two, three, four := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3), newTestNode(4)
	err := tr.AddChild(zero, one)
	if err != nil {
		t.Fatal("could not add node pair (0-1)", err)
	}

	err = tr.AddChild(zero, two)
	if err != nil {
		t.Fatal("could not add node pair (0-2)", err)
	}

	err = tr.AddChild(two, three)
	if err != nil {
		t.Fatal("could not add node pair (2-3)", err)
	}

	err = tr.AddChild(two, four)
	if err != nil {
		t.Fatal("could not add node pair (2-4)", err)
	}

	five := newTestNode(5)

	err = tr.Replace(two, five)
	if err != nil {
		t.Fatal("could not replace node", err)
	}

	children := tr.Children(zero)
	if len(children) != 2 {
		t.Fatal("unexpected length of child nodes", len(children))
	}

	if !node.Nodes([]node.Node{one, five}).Equal(children) {
		t.Fatal("unexpected children (node:0)", children)
	}

	children = tr.Children(five)
	if len(children) != 2 {
		t.Fatal("unexpected length of child node (node:5)", children)
	}

	if !node.Nodes([]node.Node{three, four}).Equal(children) {
		t.Fatal("unexpected children (node:5)", children)
	}

	for _, n := range []node.Node{three, four} {
		if tr.Parent(n).ID() != five.ID() {
			t.Fatalf("unexpected parent (node:%v) %+v", n.ID(), tr.Parent(n).ID())
		}
	}

	if tr.Parent(five).ID() != zero.ID() {
		t.Fatalf("unexpected parent (node:5) %+v", tr.Parent(five).ID())
	}
}

func TestTree(t *testing.T) {
	zero, one, two, three, four := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3), newTestNode(4)

	tests := []struct {
		name     string
		fields   *Tree
		roots    node.Nodes
		id       node.ID
		notThere node.ID
	}{
		{
			name:   "empty",
			fields: NewTree(),
			roots:  make([]node.Node, 0),
		},
		{
			name: "has nodes-children-parent",
			fields: &Tree{
				nodes: map[node.ID]entry{
					zero.ID(): {node: zero},
					one.ID():  {node: one, parent: four.ID(), hasParent: true},
				},
				children: map[node.ID]map[node.ID]struct{}{
					one.ID(): {
						two.ID():   {},
						three.ID(): {},
					},
				},
			},
			roots:    node.Nodes{zero},
			id:       zero.ID(),
			notThere: node.ID("bla"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.fields, tt.fields.Copy())
			assert.Equal(t, tt.roots, tt.fields.Roots())

			if tt.id != "" {
				assert.True(t, tt.fields.HasNode(tt.id))
			}

			if tt.notThere != "" {
				assert.False(t, tt.fields.HasNode(tt.notThere))
			}
		})
	}
}

func TestTree_Copy_IsIsolated(t *testing.T) {
	zero, one, two, three, four := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3), newTestNode(4)
	original := NewTree()
	require.NoError(t, original.AddRoot(zero))
	require.NoError(t, original.AddChild(zero, one))
	require.NoError(t, original.AddChild(one, two))
	require.NoError(t, original.AddChild(zero, three))

	cp := original.Copy()
	// every mutating op on the copy, touching existing child sets (the root's, one's) and creating new ones
	require.NoError(t, cp.AddChild(two, four))
	require.NoError(t, cp.AddChild(one, newTestNode(5)))
	require.NoError(t, cp.Replace(three, newTestNode(6)))
	_, err := cp.RemoveNode(one)
	require.NoError(t, err)

	assert.Equal(t, 4, original.Length())
	assert.ElementsMatch(t, node.Nodes{one, three}, original.Children(zero))
	assert.Equal(t, node.Nodes{two}, original.Children(one))
	assert.Empty(t, original.Children(two))
	assert.Equal(t, one, original.Parent(two))
	assert.False(t, original.HasNode(four.ID()))
}

// a realistic shape (nested dirs, one child set per dir) rather than a flat one, which flatters the alloc count
func BenchmarkTree_Copy(b *testing.B) {
	tr := NewTree()
	root := newTestNode("/")
	require.NoError(b, tr.AddRoot(root))
	var add func(parent node.Node, prefix string, depth int)
	add = func(parent node.Node, prefix string, depth int) {
		for i := 0; i < 10; i++ {
			child := newTestNode(fmt.Sprintf("%s/%d", prefix, i))
			require.NoError(b, tr.AddChild(parent, child))
			if depth > 0 {
				add(child, string(child.ID()), depth-1)
			}
		}
	}
	add(root, "", 3) // ~11k nodes, ~1.1k dirs

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tr.Copy()
	}
}

// replace onto an ID already in the tree must fail and leave the tree as it was
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

// replacing a root with a node of a different ID moves the root and its children over
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

// children returns nil, not an empty slice, for a node whose last child was removed
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

// after a same-ID Replace of a parent, Parent returns the new node, not the one it replaced
func TestTree_Parent_ReturnsReplacedNode(t *testing.T) {
	root, a := newTestNode("root"), newTestNode("a")
	tr := NewTree()
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, a))

	replacement := newTestNode("root")
	require.NoError(t, tr.Replace(root, replacement))
	assert.Same(t, replacement, tr.Parent(a))
}

// copy drops child sets that RemoveNode left empty; adding a child to that node in the copy must create a set of the
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

// addChild of a node that already has a parent leaves it in the old parent's child set. Removing both parents then
// reaches a child ID that is no longer in the tree, which must be skipped rather than panic.
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
