package tree

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

// tree represents a simple tree data structure.
type tree struct {
	nodes    map[node.ID]node.Node
	children map[node.ID]map[node.ID]node.Node
	parent   map[node.ID]node.Node
}

// newTree returns an instance of a tree.
func newTree() *tree {
	return &tree{
		nodes:    make(map[node.ID]node.Node),
		children: make(map[node.ID]map[node.ID]node.Node),
		parent:   make(map[node.ID]node.Node),
	}
}

// Roots is all of the nodes with no parents.
func (t *tree) Roots() node.Nodes {
	var nodes = make([]node.Node, 0)
	for _, n := range t.nodes {
		if parent := t.parent[n.ID()]; parent == nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// HasNode indicates is the given node ID exists in the tree.
func (t *tree) HasNode(id node.ID) bool {
	return t.nodes[id] != nil
}

// Node returns a node object for the given ID.
func (t *tree) Node(id node.ID) node.Node {
	return t.nodes[id]
}

// Nodes returns all nodes in the tree.
func (t *tree) Nodes() node.Nodes {
	if len(t.nodes) == 0 {
		return nil
	}
	nodes := make([]node.Node, len(t.nodes))
	i := 0
	for _, n := range t.nodes {
		nodes[i] = n
		i++
	}

	return nodes
}

// addNode adds the node to the tree; returns an error on node ID collisions.
func (t *tree) addNode(n node.Node) error {
	if _, exists := t.nodes[n.ID()]; exists {
		return fmt.Errorf("node ID collision: %+v", n.ID())
	}
	t.nodes[n.ID()] = n
	t.children[n.ID()] = make(map[node.ID]node.Node)
	t.parent[n.ID()] = nil
	return nil
}

// Replace takes the given old node and replaces it with the given new one.
func (t *tree) Replace(old node.Node, new node.Node) error {
	if !t.HasNode(old.ID()) {
		return fmt.Errorf("cannot replace node not in the tree")
	}

	// add the new node
	err := t.addNode(new)
	if err != nil {
		return err
	}

	// set the new node parent to the old node parent
	t.parent[new.ID()] = t.parent[old.ID()]

	for cid := range t.children[old.ID()] {
		// replace the parent entry for each child
		t.parent[cid] = new

		// add child entries to the new node
		t.children[new.ID()][cid] = t.nodes[cid]
	}

	// replace the child entry for the old parents node
	delete(t.children[t.parent[old.ID()].ID()], old.ID())
	t.children[t.parent[old.ID()].ID()][new.ID()] = new

	// remove the old node (if not already overwritten)
	if old.ID() != new.ID() {
		delete(t.children, old.ID())
		delete(t.nodes, old.ID())
		delete(t.parent, old.ID())
	}

	return nil
}

// AddRoot adds a node to the tree (with no parent).
func (t *tree) AddRoot(n node.Node) error {
	return t.addNode(n)
}

// AddChild adds a node to the tree under the given parent.
func (t *tree) AddChild(from, to node.Node) error {
	var (
		fid = from.ID()
		tid = to.ID()
		err error
	)

	if fid == tid {
		return fmt.Errorf("should not add self edge")
	}

	if _, ok := t.nodes[fid]; !ok {
		err = t.addNode(from)
		if err != nil {
			return err
		}
	} else {
		t.nodes[fid] = from
	}
	if _, ok := t.nodes[tid]; !ok {
		err = t.addNode(to)
		if err != nil {
			return err
		}
	} else {
		t.nodes[tid] = to
	}

	t.children[fid][tid] = to
	t.parent[tid] = from
	return nil
}

// RemoveNode deletes the node from the tree and returns the removed node.
func (t *tree) RemoveNode(n node.Node) (node.Nodes, error) {
	removedNodes := make([]node.Node, 0)
	nid := n.ID()
	if _, ok := t.nodes[nid]; !ok {
		return nil, fmt.Errorf("unable to remove node: %+v", nid)
	}
	for _, child := range t.children[nid] {
		subNodes, err := t.RemoveNode(child)
		for _, sn := range subNodes {
			removedNodes = append(removedNodes, sn)
		}
		if err != nil {
			return nil, err
		}
	}

	removedNodes = append(removedNodes, t.nodes[nid])

	delete(t.children, nid)
	if t.parent[nid] != nil {
		delete(t.children[t.parent[nid].ID()], nid)
	}
	delete(t.parent, nid)
	delete(t.nodes, nid)
	return removedNodes, nil
}

// Children returns all children of the given node.
func (t *tree) Children(n node.Node) node.Nodes {
	nid := n.ID()
	if _, ok := t.children[nid]; !ok {
		return nil
	}

	if len(t.children) == 0 {
		return nil
	}

	from := make([]node.Node, len(t.children[nid]))
	i := 0
	for vid := range t.children[nid] {
		from[i] = t.nodes[vid]
		i++
	}

	return from
}

// Parent returns the parent of the given node (or nil if it is a root)
func (t *tree) Parent(n node.Node) node.Node {
	if parent, ok := t.parent[n.ID()]; ok {
		return parent
	}
	return nil
}
