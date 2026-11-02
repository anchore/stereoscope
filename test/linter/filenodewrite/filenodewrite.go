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

const (
	analyzerName = "filenodewrite"
	filenodePkg  = "github.com/anchore/stereoscope/pkg/filetree/filenode"
)

var Analyzer = &analysis.Analyzer{
	Name: analyzerName,
	Doc:  "reports writes through a *filenode.FileNode, which is shared between tree copies",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	nolint := nolintLines(pass)
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
				if writesThroughPointer(pass, target) && !suppressed(pass, nolint, target.Pos()) {
					pass.Reportf(target.Pos(), "FileNodes are shared between tree copies and must not be modified in place; copy it by value (c := *fn), change the copy, and swap it in with Replace")
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
		// fn.Reference = ..., where fn (or an embedded field on the way to the selected one) is a *FileNode
		sel := pass.TypesInfo.Selections[e]
		if sel == nil || sel.Kind() != types.FieldVal {
			return false
		}
		if sel.Indirect() {
			return reachesThroughFileNodePtr(sel)
		}
		// no pointer on the way: (*fn).RealPath = ... writes through whatever the operand reaches
		return writesThroughPointer(pass, e.X)
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

// reachesThroughFileNodePtr reports whether a field selection steps through a *FileNode: the receiver itself, or an
// embedded *FileNode on the way to the selected field.
func reachesThroughFileNodePtr(sel *types.Selection) bool {
	typ := sel.Recv()
	index := sel.Index()
	for i, idx := range index {
		if isFileNodePtr(typ) {
			return true
		}
		if i == len(index)-1 {
			break
		}
		if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
		}
		st, ok := typ.Underlying().(*types.Struct)
		if !ok {
			return false
		}
		typ = st.Field(idx).Type()
	}
	return false
}

type fileLine struct {
	file string
	line int
}

// nolintLines collects the lines carrying a `//nolint` directive that applies to this analyzer (as closecheck does).
func nolintLines(pass *analysis.Pass) map[fileLine]bool {
	out := map[fileLine]bool{}
	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if !isNolintFor(c.Text, analyzerName) {
					continue
				}
				p := pass.Fset.Position(c.Pos())
				out[fileLine{p.Filename, p.Line}] = true
			}
		}
	}
	return out
}

// isNolintFor reports whether a comment is a `//nolint` directive that silences this analyzer: bare `//nolint`, or
// `//nolint:a,b` whose list contains name.
func isNolintFor(text, name string) bool {
	text = strings.TrimSpace(strings.TrimPrefix(text, "//"))
	if !strings.HasPrefix(text, "nolint") {
		return false
	}
	rest := strings.TrimPrefix(text, "nolint")
	if rest == "" || strings.HasPrefix(rest, " ") {
		return true
	}
	if !strings.HasPrefix(rest, ":") {
		return false
	}
	list := rest[1:]
	if i := strings.IndexByte(list, ' '); i >= 0 {
		list = list[:i]
	}
	for _, n := range strings.Split(list, ",") {
		if strings.TrimSpace(n) == name {
			return true
		}
	}
	return false
}

// suppressed reports whether a finding at pos is silenced by a directive on the same line or the line directly above.
func suppressed(pass *analysis.Pass, nolint map[fileLine]bool, pos token.Pos) bool {
	if !pos.IsValid() {
		return false
	}
	p := pass.Fset.Position(pos)
	return nolint[fileLine{p.Filename, p.Line}] || nolint[fileLine{p.Filename, p.Line - 1}]
}
