package tree

import (
	"fmt"
	"maps"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

// Tree represents a simple Tree data structure. Edges are kept by ID and resolved through nodes, so replacing the
// object stored for an ID is seen everywhere at once.
type Tree struct {
	nodes    map[node.ID]node.Node  // {node-id: node}
	children map[node.ID]node.IDSet // {parent-id: {child-id}}
	parent   map[node.ID]node.ID    // {child-id: parent-id}, roots have no entry
}

// NewTree returns an instance of a Tree.
func NewTree() *Tree {
	return &Tree{
		nodes:    make(map[node.ID]node.Node),
		children: make(map[node.ID]node.IDSet),
		parent:   make(map[node.ID]node.ID),
	}
}

func (t *Tree) Copy() *Tree {
	ct := NewTree()
	for k, v := range t.nodes {
		if v == nil {
			ct.nodes[k] = nil
			continue
		}
		ct.nodes[k] = v.Copy()
	}
	maps.Copy(ct.parent, t.parent)
	for from, ids := range t.children {
		ct.children[from] = maps.Clone(ids)
	}
	return ct
}

// Roots is all of the nodes with no parents.
func (t *Tree) Roots() node.Nodes {
	nodes := make([]node.Node, 0)
	for id, n := range t.nodes {
		if _, ok := t.parent[id]; !ok {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// HasNode indicates is the given node ID exists in the Tree.
func (t *Tree) HasNode(id node.ID) bool {
	if _, exists := t.nodes[id]; exists {
		return true
	}
	return false
}

// Node returns a node object for the given ID.
func (t *Tree) Node(id node.ID) node.Node {
	return t.nodes[id]
}

// Nodes returns all nodes in the Tree.
func (t *Tree) Nodes() node.Nodes {
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

// addNode adds the node to the Tree; returns an error on node ID collisions.
func (t *Tree) addNode(n node.Node) error {
	if _, exists := t.nodes[n.ID()]; exists {
		return fmt.Errorf("node ID collision: %+v", n.ID())
	}
	t.nodes[n.ID()] = n
	t.children[n.ID()] = node.NewIDSet()
	return nil
}

// Replace takes the given old node and replaces it with the given new one.
func (t *Tree) Replace(old node.Node, newNode node.Node) error {
	if !t.HasNode(old.ID()) {
		return fmt.Errorf("cannot replace node not in the Tree")
	}

	oid, nid := old.ID(), newNode.ID()

	if oid == nid {
		// the underlying objects may be different, but the ID's match. Simply track the new [already existing] node
		// and keep all existing relationships.
		t.nodes[nid] = newNode
		return nil
	}

	// add the new node
	err := t.addNode(newNode)
	if err != nil {
		return err
	}

	// move the old node's children under the new node
	for cid := range t.children[oid] {
		t.parent[cid] = nid
		t.children[nid].Add(cid)
	}

	// take the old node's place under its parent (a replaced root stays a root)
	if pid, ok := t.parent[oid]; ok {
		t.parent[nid] = pid
		t.children[pid].Remove(oid)
		t.children[pid].Add(nid)
	}

	delete(t.children, oid)
	delete(t.nodes, oid)
	delete(t.parent, oid)

	return nil
}

// AddRoot adds a node to the Tree (with no parent).
func (t *Tree) AddRoot(n node.Node) error {
	return t.addNode(n)
}

// AddChild adds the child node (to) under the parent node (from), adding either to the Tree if missing (a missing
// parent becomes a root). The stored objects for both nodes are replaced with the ones given. If the child already
// has a different parent it is moved, along with its subtree, to the new parent. Every node has at most one parent,
// and an edge that would create a cycle is rejected.
func (t *Tree) AddChild(from, to node.Node) error {
	var (
		fid = from.ID()
		tid = to.ID()
		err error
	)

	if fid == tid {
		return fmt.Errorf("should not add self edge")
	}

	// a node not yet in the tree has no descendants, so only an existing child can close a cycle
	if t.HasNode(tid) {
		for pid, ok := t.parent[fid]; ok; pid, ok = t.parent[pid] {
			if pid == tid {
				return fmt.Errorf("adding %+v under %+v would create a cycle", tid, fid)
			}
		}
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
		// reparenting an existing node must drop the edge from its old parent, otherwise removing the old parent
		// would take this node (and its subtree) with it
		if pid, ok := t.parent[tid]; ok {
			t.children[pid].Remove(tid)
		}
	}

	t.children[fid].Add(tid)
	t.parent[tid] = fid
	return nil
}

// RemoveNode deletes the node from the Tree and returns the removed node.
func (t *Tree) RemoveNode(n node.Node) (node.Nodes, error) {
	removedNodes := make([]node.Node, 0)
	nid := n.ID()
	if _, ok := t.nodes[nid]; !ok {
		return nil, fmt.Errorf("unable to remove node: %+v", nid)
	}
	for cid := range t.children[nid] {
		subNodes, err := t.RemoveNode(t.nodes[cid])
		for _, sn := range subNodes {
			removedNodes = append(removedNodes, sn)
		}
		if err != nil {
			return nil, err
		}
	}

	removedNodes = append(removedNodes, t.nodes[nid])

	delete(t.children, nid)
	if pid, ok := t.parent[nid]; ok {
		t.children[pid].Remove(nid)
	}
	delete(t.parent, nid)
	delete(t.nodes, nid)
	return removedNodes, nil
}

// Children returns all children of the given node.
func (t *Tree) Children(n node.Node) node.Nodes {
	ids, ok := t.children[n.ID()]
	if !ok {
		return nil
	}

	from := make([]node.Node, 0, len(ids))
	for cid := range ids {
		from = append(from, t.nodes[cid])
	}

	return from
}

// Parent returns the parent of the given node (or nil if it is a root)
func (t *Tree) Parent(n node.Node) node.Node {
	if pid, ok := t.parent[n.ID()]; ok {
		return t.nodes[pid]
	}
	return nil
}

func (t *Tree) Length() int {
	return len(t.nodes)
}
