package tree

import "github.com/anchore/stereoscope/pkg/tree/node"

type Walker interface {
	WalkAll()
	Walk(from node.Node) node.Node
}
