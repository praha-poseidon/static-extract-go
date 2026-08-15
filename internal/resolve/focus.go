// Package resolve implements language-level navigation: call-chain and def-use.
// Framework-agnostic (no gin/echo branches).
package resolve

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/praha-poseidon/static-extract-go/internal/find"
	"golang.org/x/tools/go/packages"
)

// Focus is the current call expression under evaluation (may differ from find anchor after chain/def-use).
type Focus struct {
	Pkg  *packages.Package
	File *ast.File
	Call *ast.CallExpr
	Name string
}

func FocusFromAnchor(a find.Anchor) (Focus, bool) {
	ce, ok := a.Node.(*ast.CallExpr)
	if !ok {
		return Focus{}, false
	}
	return Focus{Pkg: a.Pkg, File: a.File, Call: ce, Name: a.Name}, true
}

// ReceiverExpr returns the receiver expression of a method call: v.GET(...) -> v; r.Group().GET -> Group() call.
func ReceiverExpr(call *ast.CallExpr) ast.Expr {
	if call == nil {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return sel.X
}

// PrevCall: fluent previous hop. For a.b().c(), prev of c() is b().
func PrevCall(call *ast.CallExpr) *ast.CallExpr {
	recv := ReceiverExpr(call)
	if recv == nil {
		return nil
	}
	if ce, ok := recv.(*ast.CallExpr); ok {
		return ce
	}
	// recv may be paren
	if p, ok := recv.(*ast.ParenExpr); ok {
		if ce, ok := p.X.(*ast.CallExpr); ok {
			return ce
		}
	}
	return nil
}

// NextCall is not well-defined from a call alone in Go AST without parent map.
// For v1 we only support prev (receiver-side), which covers Group().GET and post().uri-style.
func NextCall(_ *ast.CallExpr, _ *ast.File) *ast.CallExpr {
	return nil
}

func CallName(ce *ast.CallExpr) string {
	if ce == nil {
		return ""
	}
	switch f := ce.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}

// NavigateChain walks prev (receiver-side) or next. nameFilter non-empty: walk until call name matches.
// If no hop succeeds, Call is nil so later argument reads yield empty (do not keep original anchor).
func NavigateChain(f Focus, dir string, steps int, nameFilter string) Focus {
	cur := f.Call
	if cur == nil {
		return Focus{Pkg: f.Pkg, File: f.File}
	}
	d := strings.ToLower(dir)
	if d == "previous" {
		d = "prev"
	}
	max := steps
	if max <= 0 {
		max = 1
	}
	moved := false
	if nameFilter != "" {
		for i := 0; i < 16; i++ {
			var next *ast.CallExpr
			if d == "next" {
				next = NextCall(cur, f.File)
			} else {
				next = PrevCall(cur)
			}
			if next == nil {
				break
			}
			cur = next
			moved = true
			if strings.EqualFold(CallName(cur), nameFilter) {
				return Focus{Pkg: f.Pkg, File: f.File, Call: cur, Name: CallName(cur)}
			}
		}
		if !moved {
			return Focus{Pkg: f.Pkg, File: f.File}
		}
		return Focus{Pkg: f.Pkg, File: f.File, Call: cur, Name: CallName(cur)}
	}
	for i := 0; i < max; i++ {
		var next *ast.CallExpr
		if d == "next" {
			next = NextCall(cur, f.File)
		} else {
			next = PrevCall(cur)
		}
		if next == nil {
			break
		}
		cur = next
		moved = true
	}
	if !moved {
		return Focus{Pkg: f.Pkg, File: f.File}
	}
	return Focus{Pkg: f.Pkg, File: f.File, Call: cur, Name: CallName(cur)}
}

// ResolveDef follows an identifier to its RHS expression (same file, lexical previous assignment).
// maxHops limits Ident→Ident chains.
func ResolveDef(pkg *packages.Package, file *ast.File, e ast.Expr, maxHops int) ast.Expr {
	if maxHops <= 0 {
		maxHops = 4
	}
	cur := e
	for hop := 0; hop < maxHops; hop++ {
		id, ok := identOf(cur)
		if !ok {
			return cur
		}
		rhs := findDefRHS(pkg, file, id)
		if rhs == nil {
			return cur
		}
		cur = rhs
		// if still ident, continue; else return expression (often CallExpr)
		if _, ok := identOf(cur); !ok {
			return cur
		}
	}
	return cur
}

func identOf(e ast.Expr) (*ast.Ident, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		return v, true
	case *ast.ParenExpr:
		return identOf(v.X)
	default:
		return nil, false
	}
}

func findDefRHS(pkg *packages.Package, file *ast.File, id *ast.Ident) ast.Expr {
	if id == nil || file == nil {
		return nil
	}
	// Prefer types.Info object identity when available
	var obj types.Object
	if pkg != nil && pkg.TypesInfo != nil {
		obj = pkg.TypesInfo.Uses[id]
		if obj == nil {
			obj = pkg.TypesInfo.Defs[id]
		}
	}
	var best ast.Expr
	var bestPos token.Pos
	targetPos := id.Pos()

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		// skip anything after the use site
		if n.Pos() >= targetPos {
			return true
		}
		switch s := n.(type) {
		case *ast.AssignStmt:
			if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
				return true
			}
			for i, lhs := range s.Lhs {
				lid, ok := lhs.(*ast.Ident)
				if !ok || lid.Name != id.Name {
					continue
				}
				if obj != nil && pkg.TypesInfo != nil {
					if defObj := pkg.TypesInfo.Defs[lid]; defObj != nil && defObj != obj {
						// different object same name in nested scope — still accept if same name before use
					}
					if useObj := pkg.TypesInfo.Uses[lid]; useObj != nil && obj != nil && useObj != obj {
						continue
					}
				}
				if i < len(s.Rhs) {
					if s.Pos() > bestPos {
						best = s.Rhs[i]
						bestPos = s.Pos()
					}
				}
			}
		case *ast.ValueSpec:
			for i, name := range s.Names {
				if name.Name != id.Name {
					continue
				}
				if obj != nil && pkg.TypesInfo != nil {
					if defObj := pkg.TypesInfo.Defs[name]; defObj != nil && defObj != obj {
						// allow same name
					}
				}
				if i < len(s.Values) {
					if name.Pos() > bestPos && name.Pos() < targetPos {
						best = s.Values[i]
						bestPos = name.Pos()
					}
				}
			}
		}
		return true
	})
	return best
}

// ArgString evaluates a constant string at call arg index.
func ArgString(f Focus, idx int) string {
	if f.Call == nil || idx < 0 || idx >= len(f.Call.Args) {
		return ""
	}
	return constString(f.Call.Args[idx], f.Pkg)
}

func constString(e ast.Expr, pkg *packages.Package) string {
	var info *types.Info
	if pkg != nil {
		info = pkg.TypesInfo
	}
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, err := strconv.Unquote(v.Value)
			if err == nil {
				return s
			}
			return strings.Trim(v.Value, `"`)
		}
	case *ast.Ident:
		if info != nil {
			if obj := info.Uses[v]; obj != nil {
				if c, ok := obj.(*types.Const); ok {
					return strings.Trim(c.Val().ExactString(), `"`)
				}
			}
		}
	case *ast.BinaryExpr:
		// only support string + string constants lightly
		if v.Op == token.ADD {
			l, r := constString(v.X, pkg), constString(v.Y, pkg)
			if l != "" || r != "" {
				return l + r
			}
		}
	}
	return ""
}

// FocusFromExpr if e is a call, return Focus; if e is ident, resolve def then call.
func FocusFromExpr(pkg *packages.Package, file *ast.File, e ast.Expr) (Focus, bool) {
	e = ResolveDef(pkg, file, e, 4)
	if ce, ok := e.(*ast.CallExpr); ok {
		return Focus{Pkg: pkg, File: file, Call: ce, Name: CallName(ce)}, true
	}
	if p, ok := e.(*ast.ParenExpr); ok {
		return FocusFromExpr(pkg, file, p.X)
	}
	return Focus{}, false
}
