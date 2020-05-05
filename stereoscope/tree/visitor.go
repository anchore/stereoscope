package tree

import "github.com/anchore/stereoscope/stereoscope/tree/node"

type Visitor func(n node.Node)
