package extract

import (
	"codegraph-cli/internal/model"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func extractGo(filePath string, source []byte) model.ExtractResult {
	res := model.ExtractResult{}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, source, parser.ParseComments)
	if err != nil {
		res.Errors = append(res.Errors, "parse failed: "+err.Error())
	}
	if file == nil {
		return res
	}
	for _, im := range file.Imports {
		module := strings.Trim(im.Path.Value, "\"`")
		name := moduleBase(module)
		if im.Name != nil && im.Name.Name != "." && im.Name.Name != "_" {
			name = im.Name.Name
		}
		line := fset.Position(im.Pos()).Line
		qn := module
		if im.Name != nil {
			qn = name
		}
		res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, qn, line), Kind: "import", Name: name, QualifiedName: qn, FilePath: filePath, Language: "go", StartLine: line, EndLine: line, IsExported: false, ImportModule: strPtr(module)})
	}
	for _, d := range file.Decls {
		switch x := d.(type) {
		case *ast.GenDecl:
			appendGoDecl(filePath, fset, x, &res)
		case *ast.FuncDecl:
			line := fset.Position(x.Pos()).Line
			name := x.Name.Name
			qn := name
			kind := "function"
			if x.Recv != nil && len(x.Recv.List) > 0 {
				kind = "method"
				recv := receiverName(x.Recv.List[0].Type)
				if recv != "" {
					qn = recv + "." + name
				}
			}
			sig := goSignature(source, fset.Position(x.Pos()).Offset)
			node := model.SymbolNode{ID: stableID(filePath, qn, line), Kind: kind, Name: name, QualifiedName: qn, FilePath: filePath, Language: "go", StartLine: line, EndLine: fset.Position(x.End()).Line, Signature: strPtr(sig), IsExported: ast.IsExported(name)}
			res.Nodes = append(res.Nodes, node)
			if x.Body != nil {
				ast.Inspect(x.Body, func(n ast.Node) bool {
					if decl, ok := n.(*ast.DeclStmt); ok {
						if gen, ok := decl.Decl.(*ast.GenDecl); ok {
							appendGoDecl(filePath, fset, gen, &res)
						}
						return true
					}
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					callee, simple := goCallee(call.Fun)
					if simple == "" {
						return true
					}
					pos := fset.Position(call.Lparen)
					res.Edges = append(res.Edges, model.EdgeRecord{Source: node.ID, Target: "name:" + simple, Kind: "calls", Line: intPtr(pos.Line), Col: intPtr(pos.Column - 1), Metadata: map[string]any{"callee_text": callee}})
					return true
				})
			}
		}
	}
	return res
}

func appendGoDecl(filePath string, fset *token.FileSet, decl *ast.GenDecl, res *model.ExtractResult) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			line := fset.Position(s.Pos()).Line
			kind := "type_alias"
			switch s.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			node := model.SymbolNode{ID: stableID(filePath, s.Name.Name, line), Kind: kind, Name: s.Name.Name, QualifiedName: s.Name.Name, FilePath: filePath, Language: "go", StartLine: line, EndLine: fset.Position(s.End()).Line, IsExported: ast.IsExported(s.Name.Name)}
			res.Nodes = append(res.Nodes, node)
		case *ast.ValueSpec:
			kind := "variable"
			if decl.Tok == token.CONST {
				kind = "constant"
			}
			for _, n := range s.Names {
				line := fset.Position(n.Pos()).Line
				res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, n.Name, line), Kind: kind, Name: n.Name, QualifiedName: n.Name, FilePath: filePath, Language: "go", StartLine: line, EndLine: line, IsExported: ast.IsExported(n.Name)})
			}
		}
	}
}

func moduleBase(module string) string {
	module = strings.Trim(module, "/")
	if module == "" {
		return module
	}
	parts := strings.Split(module, "/")
	return parts[len(parts)-1]
}

func receiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return receiverName(x.X)
	case *ast.IndexExpr:
		return receiverName(x.X)
	case *ast.IndexListExpr:
		return receiverName(x.X)
	default:
		return ""
	}
}

func goSignature(src []byte, off int) string {
	s := string(src[off:])
	if i := strings.Index(s, "{"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
}

func goCallee(expr ast.Expr) (string, string) {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name, x.Name
	case *ast.SelectorExpr:
		left, _ := goCallee(x.X)
		if left == "" {
			left = exprString(x.X)
		}
		return left + "." + x.Sel.Name, x.Sel.Name
	case *ast.IndexExpr:
		return goCallee(x.X)
	case *ast.IndexListExpr:
		return goCallee(x.X)
	default:
		return exprString(expr), ""
	}
}

func exprString(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		a, _ := goCallee(x)
		return a
	case *ast.IndexExpr:
		return exprString(x.X)
	case *ast.IndexListExpr:
		return exprString(x.X)
	default:
		return ""
	}
}
