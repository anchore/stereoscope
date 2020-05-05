package tree

import "github.com/anchore/stereoscope/stereoscope/tree/node"

type Walker interface {
	WalkAll()
	Walk(from node.Node) node.Node
	Visited(n node.Node) bool
}
