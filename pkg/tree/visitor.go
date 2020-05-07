package tree

import "github.com/anchore/stereoscope/pkg/tree/node"

type Visitor func(n node.Node)
