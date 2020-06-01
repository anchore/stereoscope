package tree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree/node"
)

type NodeVisitor func(node.Node)

type FileVisitor func(file.Reference)
