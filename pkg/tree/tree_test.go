package tree

import (
	"fmt"
	"testing"

	"github.com/anchore/stereoscope/pkg/tree/node"
	"github.com/stretchr/testify/assert"
)

type testNode struct {
	Id node.ID
}

func toId(i interface{}) node.ID {
	return node.ID(fmt.Sprintf("%v", i))
}

func newTestNode(id interface{}) *testNode {
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
