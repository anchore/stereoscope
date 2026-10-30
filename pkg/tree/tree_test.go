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

func TestTree_Replace_Root(t *testing.T) {
	// a root has no parent entry to re-point, the replacement must become a root in its place
	tr := NewTree()
	root, child, replacement := newTestNode(0), newTestNode(1), newTestNode(2)
	require.NoError(t, tr.AddRoot(root))
	require.NoError(t, tr.AddChild(root, child))

	require.NoError(t, tr.Replace(root, replacement))

	assert.Equal(t, node.Nodes{replacement}, tr.Roots())
	assert.Equal(t, node.Nodes{child}, tr.Children(replacement))
	assert.Equal(t, replacement, tr.Parent(child))
	assert.Nil(t, tr.Parent(replacement))
	assert.False(t, tr.HasNode(root.ID()))
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
				nodes: map[node.ID]node.Node{
					zero.ID(): zero,
					one.ID():  one,
				},
				children: map[node.ID]map[node.ID]node.Node{
					one.ID(): {
						two.ID():   two,
						three.ID(): nil,
					},
				},
				parent: map[node.ID]node.Node{
					four.ID(): nil,
					one.ID():  one,
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

func TestTree_AddChild_Reparent(t *testing.T) {
	// 0 -> 1 -> 3 -> 4 and 0 -> 2, then 3 (with its subtree) is moved from 1 to 2
	setup := func(t *testing.T) (*Tree, node.Node, node.Node, node.Node, node.Node) {
		tr := NewTree()
		zero, one, two, three, four := newTestNode(0), newTestNode(1), newTestNode(2), newTestNode(3), newTestNode(4)
		require.NoError(t, tr.AddRoot(zero))
		require.NoError(t, tr.AddChild(zero, one))
		require.NoError(t, tr.AddChild(zero, two))
		require.NoError(t, tr.AddChild(one, three))
		require.NoError(t, tr.AddChild(three, four))
		require.NoError(t, tr.AddChild(two, three))
		assert.Empty(t, tr.Children(one))
		assert.Equal(t, node.Nodes{three}, tr.Children(two))
		assert.Equal(t, two, tr.Parent(three))
		return tr, one, two, three, four
	}

	t.Run("remove new parent first", func(t *testing.T) {
		tr, one, two, _, _ := setup(t)
		_, err := tr.RemoveNode(two)
		require.NoError(t, err)
		_, err = tr.RemoveNode(one)
		require.NoError(t, err)
		assert.Equal(t, 1, tr.Length())
	})

	t.Run("remove old parent first", func(t *testing.T) {
		tr, one, _, three, four := setup(t)
		_, err := tr.RemoveNode(one)
		require.NoError(t, err)
		assert.True(t, tr.HasNode(three.ID()), "reparented node must not go with its old parent")
		assert.True(t, tr.HasNode(four.ID()), "subtree must move with the reparented node")
		assert.Equal(t, three, tr.Parent(four))
	})

	t.Run("re-add under the same parent", func(t *testing.T) {
		tr, _, two, three, _ := setup(t)
		require.NoError(t, tr.AddChild(two, three))
		assert.Equal(t, node.Nodes{three}, tr.Children(two))
		assert.Equal(t, two, tr.Parent(three))
	})

	t.Run("reparent a root", func(t *testing.T) {
		tr := NewTree()
		root, other := newTestNode(0), newTestNode(1)
		require.NoError(t, tr.AddRoot(root))
		require.NoError(t, tr.AddRoot(other))
		require.NoError(t, tr.AddChild(other, root))
		assert.Equal(t, node.Nodes{other}, tr.Roots())
		assert.Equal(t, other, tr.Parent(root))
	})

	t.Run("reject cycles", func(t *testing.T) {
		tr, one, two, three, four := setup(t)
		// under a direct child
		require.Error(t, tr.AddChild(three, two))
		// under a deeper descendant
		require.Error(t, tr.AddChild(four, two))
		// a root under its own child
		zero := tr.Node(toId(0))
		require.Error(t, tr.AddChild(one, zero))

		// rejected edges leave the tree untouched
		assert.Equal(t, node.Nodes{zero}, tr.Roots())
		assert.Equal(t, two, tr.Parent(three))
		assert.Equal(t, three, tr.Parent(four))
		assert.Equal(t, zero, tr.Parent(two))
	})

	t.Run("replacing an existing parent updates its children's parent entry", func(t *testing.T) {
		// AddChild(parentV2, addedChild) swaps the stored object for "p" from parentV1 to parentV2. Parent() must
		// then return parentV2 for existingChild as well, not only for addedChild.
		tr := NewTree()
		parentV1, parentV2 := newTestNode("p"), newTestNode("p")
		existingChild, addedChild := newTestNode(1), newTestNode(2)
		require.NoError(t, tr.AddRoot(parentV1))
		require.NoError(t, tr.AddChild(parentV1, existingChild))
		require.NoError(t, tr.AddChild(parentV2, addedChild))

		assert.Same(t, parentV2, tr.Node(parentV2.ID()))
		assert.Same(t, parentV2, tr.Parent(addedChild))
		assert.Same(t, parentV2, tr.Parent(existingChild))
	})
}
