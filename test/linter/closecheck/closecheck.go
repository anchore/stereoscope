// Command closecheck is an SSA-based analyzer that flags io.Closer values returned
// from a call that are then dropped -- neither closed nor allowed to escape (returned,
// stored, or captured by a closure). unlike the ruleguard (AST) rules, this follows the
// value through the def-use graph, so it correctly ignores a closer that is returned
// several statements later or stashed in a struct field, while still catching the
// "drain it then leak it" footgun.
//
// run it standalone (it is invoked by the `lint` make task):
//
//	go run ./test/linter/closecheck ./...
package main

import (
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/singlechecker"
	"golang.org/x/tools/go/ssa"
)

func main() {
	singlechecker.Main(Analyzer)
}

// includeTests opts back into reporting findings in _test.go files. tests routinely
// hand a fixture's lifecycle to a helper (e.g. t.Cleanup), which this interprocedural
// analyzer cannot see, so those files are excluded by default.
var includeTests bool

func init() {
	Analyzer.Flags.BoolVar(&includeTests, "includetests", false, "also report findings in _test.go files")
}

const analyzerName = "closecheck"

var Analyzer = &analysis.Analyzer{
	Name:     analyzerName,
	Doc:      "reports io.Closer values from a call that are neither closed nor escape (return/store/capture)",
	Requires: []*analysis.Analyzer{buildssa.Analyzer},
	Run:      run,
}

// closerIface is the io.Closer interface, rebuilt from the universe error type so the
// analyzer does not depend on the analyzed package importing "io".
var closerIface = buildCloserIface()

func buildCloserIface() *types.Interface {
	errType := types.Universe.Lookup("error").Type()
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", errType))
	sig := types.NewSignatureType(nil, nil, nil, nil, results, false)
	closeFn := types.NewFunc(token.NoPos, nil, "Close", sig)
	return types.NewInterfaceType([]*types.Func{closeFn}, nil).Complete()
}

func implementsCloser(t types.Type) bool {
	if t == nil {
		return false
	}
	// a concrete type with a pointer-receiver Close only satisfies the interface via *T.
	return types.Implements(t, closerIface) || types.Implements(types.NewPointer(t), closerIface)
}

func run(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	nolint := nolintLines(pass)

	for _, fn := range in.SrcFuncs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				v, ok := instr.(ssa.Value)
				if !ok || !isCloserSource(v) {
					continue
				}
				// skip closers whose lifecycle the stdlib owns: exec.Cmd pipes are
				// closed by Cmd.Wait, and the docs say callers must not close them.
				if call := originatingCall(v); call != nil && isStdlibManagedPipe(call) {
					continue
				}
				pos := posOf(v)
				if !includeTests && isTestFile(pass, pos) {
					continue
				}
				if handled(v, map[ssa.Value]bool{}) {
					continue
				}
				// honor an inline `//nolint:closecheck` on the finding's line (or the
				// line above) for the false positives this analyzer cannot see, e.g. a
				// closer whose Close just forwards to a handle owned elsewhere.
				if suppressed(pass, nolint, pos) {
					continue
				}
				pass.Reportf(pos, "closer (%s) is dropped: never closed and does not escape via return/store", v.Type().String())
			}
		}
	}
	return nil, nil
}

// isCloserSource reports whether v is a freshly-acquired closer: the result of a call,
// either a single-value call or element #0 of a multi-value (closer, error) call.
func isCloserSource(v ssa.Value) bool {
	switch v := v.(type) {
	case *ssa.Call:
		return implementsCloser(v.Type())
	case *ssa.Extract:
		if _, ok := v.Tuple.(*ssa.Call); !ok {
			return false
		}
		return implementsCloser(v.Type())
	}
	return false
}

type fileLine struct {
	file string
	line int
}

// nolintLines collects the lines carrying a `//nolint` directive that applies to this
// analyzer -- either bare `//nolint` or `//nolint:...` whose list includes our name.
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

// isNolintFor reports whether a comment is a `//nolint` directive that silences this
// analyzer: bare `//nolint`, or `//nolint:a,b` whose list contains name.
func isNolintFor(text, name string) bool {
	text = strings.TrimSpace(strings.TrimPrefix(text, "//"))
	if !strings.HasPrefix(text, "nolint") {
		return false
	}
	rest := strings.TrimPrefix(text, "nolint")
	if rest == "" || strings.HasPrefix(rest, " ") {
		return true // bare //nolint (optionally with a trailing comment) silences everything
	}
	if !strings.HasPrefix(rest, ":") {
		return false // e.g. "nolintfoo" is not a directive
	}
	list := rest[1:]
	if i := strings.IndexByte(list, ' '); i >= 0 {
		list = list[:i] // drop a trailing " // reason"
	}
	for _, n := range strings.Split(list, ",") {
		if strings.TrimSpace(n) == name {
			return true
		}
	}
	return false
}

// suppressed reports whether a finding at pos is silenced by a directive on the same
// line or the line directly above it.
func suppressed(pass *analysis.Pass, nolint map[fileLine]bool, pos token.Pos) bool {
	if !pos.IsValid() {
		return false
	}
	p := pass.Fset.Position(pos)
	return nolint[fileLine{p.Filename, p.Line}] || nolint[fileLine{p.Filename, p.Line - 1}]
}

// isTestFile reports whether the position lies in a _test.go file.
func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	if !pos.IsValid() {
		return false
	}
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

// originatingCall returns the call that produced the closer value, whether it is a
// single-value call or element #0 of a multi-value (closer, error) call.
func originatingCall(v ssa.Value) *ssa.Call {
	switch v := v.(type) {
	case *ssa.Call:
		return v
	case *ssa.Extract:
		if c, ok := v.Tuple.(*ssa.Call); ok {
			return c
		}
	}
	return nil
}

// isStdlibManagedPipe reports whether the call is one of the exec.Cmd pipe methods,
// whose result is closed by Cmd.Wait -- callers must not close it themselves.
func isStdlibManagedPipe(c *ssa.Call) bool {
	callee := c.Common().StaticCallee()
	if callee == nil {
		return false
	}
	recv := callee.Signature.Recv()
	if recv == nil || !isNamedType(recv.Type(), "os/exec", "Cmd") {
		return false
	}
	switch callee.Name() {
	case "StdoutPipe", "StderrPipe", "StdinPipe":
		return true
	}
	return false
}

// isNamedType reports whether t (or its pointer element) is the named type pkgPath.name.
func isNamedType(t types.Type, pkgPath, name string) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}

// handled reports whether the closer is closed on some path or escapes the function. it
// follows alias-creating instructions (interface boxing, conversions, phi nodes) so the
// disposition is tracked across the value's whole def-use chain.
func handled(v ssa.Value, seen map[ssa.Value]bool) bool {
	if seen[v] {
		return false
	}
	seen[v] = true

	refs := v.Referrers()
	if refs == nil {
		return false
	}

	for _, instr := range *refs {
		switch r := instr.(type) {
		case *ssa.Return:
			return true // escapes to the caller
		case *ssa.Store:
			if r.Val == v {
				return true // handed off into a variable, field, or global
			}
		case *ssa.MakeClosure:
			return true // captured by a closure, e.g. `defer func() { x.Close() }()`
		case *ssa.Call:
			if isCloseCall(r.Common(), v) {
				return true
			}
		case *ssa.Defer:
			if isCloseCall(r.Common(), v) {
				return true
			}
		case *ssa.Go:
			if isCloseCall(r.Common(), v) {
				return true
			}
		case *ssa.MakeInterface, *ssa.ChangeType, *ssa.ChangeInterface, *ssa.Convert, *ssa.Phi:
			// the value flows onward unchanged; follow the alias.
			if handled(r.(ssa.Value), seen) {
				return true
			}
		}
	}
	return false
}

// isCloseCall reports whether c is a `recv.Close()` invocation.
func isCloseCall(c *ssa.CallCommon, recv ssa.Value) bool {
	if c.IsInvoke() {
		// interface dispatch: c.Value is the receiver.
		return c.Method.Name() == "Close" && c.Value == recv
	}
	// direct method call: Close(recv) with the receiver as the first argument.
	callee := c.StaticCallee()
	if callee == nil || callee.Name() != "Close" || callee.Signature.Recv() == nil {
		return false
	}
	return len(c.Args) > 0 && c.Args[0] == recv
}

func posOf(v ssa.Value) token.Pos {
	if v.Pos().IsValid() {
		return v.Pos()
	}
	if e, ok := v.(*ssa.Extract); ok {
		return e.Tuple.Pos()
	}
	return token.NoPos
}
