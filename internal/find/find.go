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
	Pkg           *packages.Package
	File          *ast.File
	Node          ast.Node
	Kind          string
	Name          string
	RecvType      string // legacy call/declaration identity owner
	ReceiverType  string // actual method receiver type; empty for package functions
	CallOwner     string // method receiver type or selected package identifier
	EnclosingType string // Go type containing the anchor, when any
	CallArgs      []ast.Expr
	Fun           ast.Expr
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
			enclosingByNode := indexEnclosingTypes(file, pkg.TypesInfo)
			ast.Inspect(file, func(n ast.Node) bool {
				switch kind {
				case "call":
					ce, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name, owner, receiverType := callIdentity(ce, pkg.TypesInfo)
					if matchName(rest, name, owner) {
						out = append(out, Anchor{
							Pkg: pkg, File: file, Node: ce, Kind: "call",
							Name: name, RecvType: owner, ReceiverType: receiverType,
							CallOwner: owner, EnclosingType: enclosingByNode[ce],
							CallArgs: ce.Args, Fun: ce.Fun,
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
					// 1) Methods with receiver (class methods) — FuncDecl
					if fd, ok := n.(*ast.FuncDecl); ok && fd.Recv != nil {
						recv := recvTypeName(fd, pkg.TypesInfo)
						if matchName(rest, fd.Name.Name, recv) {
							out = append(out, Anchor{
								Pkg: pkg, File: file, Node: fd, Kind: "method", Name: fd.Name.Name,
								RecvType: recv, ReceiverType: recv, EnclosingType: recv,
							})
						}
					}
					// 2) Interface method signatures (no body) — aligned with Java interface methods
					if ts, ok := n.(*ast.TypeSpec); ok {
						it, ok := ts.Type.(*ast.InterfaceType)
						if !ok || it.Methods == nil {
							return true
						}
						ifaceName := ts.Name.Name
						for _, field := range it.Methods.List {
							if field.Names == nil {
								continue // embedded interface
							}
							if _, isFunc := field.Type.(*ast.FuncType); !isFunc {
								continue
							}
							for _, id := range field.Names {
								if matchName(rest, id.Name, ifaceName) || matchSimple(rest, id.Name) {
									out = append(out, Anchor{
										Pkg: pkg, File: file, Node: id, Kind: "method",
										Name: id.Name, RecvType: ifaceName,
										ReceiverType: ifaceName, EnclosingType: ifaceName,
									})
								}
							}
						}
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
	joined := strings.ReplaceAll(strings.Join(rest, ""), " ", "")
	if strings.HasPrefix(joined, "[") && strings.HasSuffix(joined, "]") {
		inner := strings.Trim(joined, "[]")
		for _, part := range strings.Split(inner, ",") {
			if callMatches(strings.TrimSpace(part), name, recv) {
				return true
			}
		}
		return false
	}
	if listStart := strings.Index(joined, ".["); listStart > 0 && strings.HasSuffix(joined, "]") {
		owner := joined[:listStart]
		methods := joined[listStart+2 : len(joined)-1]
		for _, method := range strings.Split(methods, ",") {
			if callMatches(owner+"."+strings.TrimSpace(method), name, recv) {
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

func callIdentity(ce *ast.CallExpr, info *types.Info) (name, owner, receiverType string) {
	switch f := ce.Fun.(type) {
	case *ast.Ident:
		return f.Name, "", ""
	case *ast.SelectorExpr:
		name = f.Sel.Name
		if info != nil {
			if sel := info.Selections[f]; sel != nil {
				if t := sel.Recv(); t != nil {
					receiverType = types.TypeString(t, (*types.Package).Name)
					owner = receiverType
				}
			}
			if owner == "" {
				if id, ok := f.X.(*ast.Ident); ok {
					if _, ok := info.Uses[id].(*types.PkgName); ok {
						// Keep the source-level package identifier for compatibility with
						// existing `find call pkg.Func` rules (including import aliases).
						owner = id.Name
					}
				}
			}
			if owner == "" {
				if tv, ok := info.Types[f.X]; ok && tv.Type != nil {
					receiverType = types.TypeString(tv.Type, (*types.Package).Name)
					owner = receiverType
				}
			}
		}
		if owner == "" {
			if id, ok := f.X.(*ast.Ident); ok {
				owner = id.Name
			}
		}
		return name, normalizeType(owner), normalizeType(receiverType)
	default:
		return "", "", ""
	}
}

func indexEnclosingTypes(file *ast.File, info *types.Info) map[ast.Node]string {
	byNode := map[ast.Node]string{}
	var current string
	var parents []string
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			current = parents[len(parents)-1]
			parents = parents[:len(parents)-1]
			return true
		}
		parents = append(parents, current)
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Recv != nil {
			current = normalizeType(recvTypeName(fd, info))
		}
		byNode[n] = current
		return true
	})
	return byNode
}

func normalizeType(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "*")
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
