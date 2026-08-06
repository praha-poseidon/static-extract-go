package find

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Anchor struct {
	Pkg      *packages.Package
	File     *ast.File
	Node     ast.Node
	Kind     string
	Name     string
	RecvType string
	CallArgs []ast.Expr
	Fun      ast.Expr
}

func FindAnchors(pkgs []*packages.Package, findAtoms []string) []Anchor {
	if len(findAtoms) == 0 {
		return nil
	}
	kind := findAtoms[0]
	rest := findAtoms[1:]
	var out []Anchor
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				switch kind {
				case "call":
					ce, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name, recv := callName(ce, pkg.TypesInfo)
					if matchName(rest, name, recv) {
						out = append(out, Anchor{
							Pkg: pkg, File: file, Node: ce, Kind: "call",
							Name: name, RecvType: recv, CallArgs: ce.Args, Fun: ce.Fun,
						})
					}
				case "func":
					fd, ok := n.(*ast.FuncDecl)
					if !ok || fd.Recv != nil {
						return true
					}
					if matchSimple(rest, fd.Name.Name) {
						out = append(out, Anchor{Pkg: pkg, File: file, Node: fd, Kind: "func", Name: fd.Name.Name})
					}
				case "method":
					fd, ok := n.(*ast.FuncDecl)
					if !ok || fd.Recv == nil {
						return true
					}
					recv := recvTypeName(fd, pkg.TypesInfo)
					if matchName(rest, fd.Name.Name, recv) {
						out = append(out, Anchor{Pkg: pkg, File: file, Node: fd, Kind: "method", Name: fd.Name.Name, RecvType: recv})
					}
				}
				return true
			})
		}
	}
	return out
}

func matchSimple(rest []string, name string) bool {
	if len(rest) == 0 || (len(rest) == 1 && (rest[0] == "*" || rest[0] == "")) {
		return true
	}
	for _, r := range rest {
		r = strings.Trim(r, "[]")
		for _, part := range strings.Split(r, ",") {
			part = strings.TrimSpace(part)
			if part == name || part == "*" {
				return true
			}
		}
	}
	return false
}

func matchName(rest []string, name, recv string) bool {
	if len(rest) == 0 {
		return true
	}
	joined := strings.ReplaceAll(strings.Join(rest, "."), " ", "")
	if strings.HasPrefix(joined, "[") && strings.HasSuffix(joined, "]") {
		inner := strings.Trim(joined, "[]")
		for _, part := range strings.Split(inner, ",") {
			if callMatches(strings.TrimSpace(part), name, recv) {
				return true
			}
		}
		return false
	}
	return callMatches(joined, name, recv)
}

func callMatches(spec, name, recv string) bool {
	if spec == "*" || spec == "" {
		return true
	}
	if strings.Contains(spec, ".") {
		parts := strings.Split(spec, ".")
		wantName := parts[len(parts)-1]
		wantRecv := strings.Join(parts[:len(parts)-1], ".")
		if wantName != name && wantName != "*" {
			return false
		}
		if wantRecv == "*" {
			return true
		}
		if recv == "" {
			return false
		}
		return recv == wantRecv || strings.HasSuffix(recv, "."+wantRecv) || strings.HasSuffix(recv, wantRecv) || baseIdent(recv) == wantRecv
	}
	return name == spec
}

func baseIdent(recv string) string {
	if i := strings.LastIndex(recv, "."); i >= 0 {
		return recv[i+1:]
	}
	return recv
}

func callName(ce *ast.CallExpr, info *types.Info) (name, recv string) {
	switch f := ce.Fun.(type) {
	case *ast.Ident:
		return f.Name, ""
	case *ast.SelectorExpr:
		name = f.Sel.Name
		if info != nil {
			if sel := info.Selections[f]; sel != nil {
				if t := sel.Recv(); t != nil {
					recv = types.TypeString(t, (*types.Package).Name)
				}
			}
			if recv == "" {
				if tv, ok := info.Types[f.X]; ok && tv.Type != nil {
					recv = types.TypeString(tv.Type, (*types.Package).Name)
				}
			}
		}
		if recv == "" {
			if id, ok := f.X.(*ast.Ident); ok {
				recv = id.Name
			}
		}
		return name, strings.TrimPrefix(recv, "*")
	default:
		return "", ""
	}
}

func recvTypeName(fd *ast.FuncDecl, info *types.Info) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	if info != nil {
		if obj := info.Defs[fd.Name]; obj != nil {
			if fn, ok := obj.(*types.Func); ok {
				if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
					return strings.TrimPrefix(types.TypeString(sig.Recv().Type(), (*types.Package).Name), "*")
				}
			}
		}
	}
	return ""
}

func ArgString(a Anchor, i int) string {
	if i < 0 || i >= len(a.CallArgs) {
		return ""
	}
	return constString(a.CallArgs[i], a.Pkg.TypesInfo)
}

func constString(e ast.Expr, info *types.Info) string {
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
	}
	return ""
}
