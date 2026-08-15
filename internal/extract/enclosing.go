package extract

import (
	"go/ast"
	"go/token"
)

// enclosingFuncName returns Type.Method or Func for the innermost func containing pos.
func enclosingFuncName(file *ast.File, pos token.Pos) string {
	if file == nil || !pos.IsValid() {
		return "unknown"
	}
	var best string
	var bestSize token.Pos = 1<<31 - 1
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		if pos < fd.Pos() || pos > fd.End() {
			continue
		}
		size := fd.End() - fd.Pos()
		if size >= bestSize {
			continue
		}
		bestSize = size
		name := fd.Name.Name
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			recv := typeExprName(fd.Recv.List[0].Type)
			if recv != "" {
				name = recv + "." + name
			}
		}
		best = name
	}
	if best == "" {
		return "unknown"
	}
	return best
}

func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	case *ast.IndexExpr:
		return typeExprName(t.X)
	case *ast.IndexListExpr:
		return typeExprName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}
