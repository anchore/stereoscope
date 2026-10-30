// Command filenodewrite flags any write through a *filenode.FileNode in non-test code. FileNodes are shared between
// tree copies (see tree.Tree), so a write through the pointer changes every tree holding that node. Copy the node,
// change the copy, and swap it in with Replace instead.
//
// run it standalone (it is invoked by the `lint` make task):
//
//	go run ./test/linter/filenodewrite ./...
package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(Analyzer)
}

const filenodePkg = "github.com/anchore/stereoscope/pkg/filetree/filenode"

var Analyzer = &analysis.Analyzer{
	Name: "filenodewrite",
	Doc:  "reports writes through a *filenode.FileNode, which is shared between tree copies",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(f.Pos()).Filename, "_test.go") {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			var targets []ast.Expr
			switch s := n.(type) {
			case *ast.AssignStmt:
				if s.Tok != token.DEFINE {
					targets = s.Lhs
				}
			case *ast.IncDecStmt:
				targets = []ast.Expr{s.X}
			case *ast.UnaryExpr:
				// taking a field's address lets the write happen somewhere else
				if s.Op == token.AND {
					targets = []ast.Expr{s.X}
				}
			}
			for _, target := range targets {
				if writesThroughPointer(pass, target) {
					pass.Reportf(target.Pos(), "FileNodes are shared between tree copies and must not be modified in place; copy it and swap it in with Replace")
				}
			}
			return true
		})
	}
	return nil, nil
}

func writesThroughPointer(pass *analysis.Pass, e ast.Expr) bool {
	switch e := ast.Unparen(e).(type) {
	case *ast.SelectorExpr:
		// fn.Reference = ..., where fn (or something on the way to it) is a *FileNode
		sel := pass.TypesInfo.Selections[e]
		return sel != nil && sel.Kind() == types.FieldVal && sel.Indirect() && isFileNodePtr(sel.Recv())
	case *ast.StarExpr:
		// *fn = ...
		return isFileNodePtr(pass.TypesInfo.TypeOf(e.X))
	}
	return false
}

func isFileNodePtr(typ types.Type) bool {
	ptr, ok := typ.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && named.Obj().Name() == "FileNode" && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == filenodePkg
}
