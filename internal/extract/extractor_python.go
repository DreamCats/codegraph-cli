package extract

import (
	"github.com/DreamCats/codegraph-cli/internal/model"
	"regexp"
	"strings"
)

type pyCtx struct {
	indent int
	name   string
	qname  string
	kind   string
	id     string
}

var (
	pyClassRE  = regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	pyDefRE    = regexp.MustCompile(`^(async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*(\(.*)`)
	pyImportRE = regexp.MustCompile(`^import\s+(.+)$`)
	pyFromRE   = regexp.MustCompile(`^from\s+([A-Za-z0-9_./]+)\s+import\s+(.+)$`)
	pyCallRE   = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)(?:\.[A-Za-z_][A-Za-z0-9_]*)?\s*\(`)
)

func extractPython(filePath string, source []byte) model.ExtractResult {
	res := model.ExtractResult{}
	lines := strings.Split(string(source), "\n")
	stack := []pyCtx{}
	pendingStatic := map[int]bool{}
	for i, line := range lines {
		ln := i + 1
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if strings.HasPrefix(trim, "@staticmethod") {
			pendingStatic[indent] = true
			continue
		}
		if m := pyImportRE.FindStringSubmatch(trim); m != nil {
			for _, part := range strings.Split(m[1], ",") {
				part = strings.TrimSpace(part)
				fields := strings.Fields(part)
				module := fields[0]
				local := module
				if len(fields) >= 3 && fields[1] == "as" {
					local = fields[2]
				}
				name := moduleBase(strings.ReplaceAll(local, ".", "/"))
				res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, local, ln), Kind: "import", Name: name, QualifiedName: module, FilePath: filePath, Language: "python", StartLine: ln, EndLine: ln, ImportModule: strPtr(module)})
			}
			continue
		}
		if m := pyFromRE.FindStringSubmatch(trim); m != nil {
			module := m[1]
			for _, part := range strings.Split(m[2], ",") {
				part = strings.TrimSpace(part)
				fields := strings.Fields(part)
				sym := fields[0]
				local := sym
				if len(fields) >= 3 && fields[1] == "as" {
					local = fields[2]
				}
				qn := module + "." + sym
				res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, qn, ln), Kind: "import", Name: local, QualifiedName: qn, FilePath: filePath, Language: "python", StartLine: ln, EndLine: ln, ImportModule: strPtr(module), ImportSymbol: strPtr(sym)})
			}
			continue
		}
		if m := pyClassRE.FindStringSubmatch(trim); m != nil {
			name := m[1]
			qn := qualify(stack, name)
			doc := nextDocstring(lines, i+1, indent)
			node := model.SymbolNode{ID: stableID(filePath, qn, ln), Kind: "class", Name: name, QualifiedName: qn, FilePath: filePath, Language: "python", StartLine: ln, EndLine: pyBlockEndLine(lines, i, indent), Docstring: doc, IsExported: !strings.HasPrefix(name, "_")}
			res.Nodes = append(res.Nodes, node)
			if parent := enclosingFunc(stack); parent != nil {
				res.Edges = append(res.Edges, model.EdgeRecord{Source: parent.id, Target: node.ID, Kind: "contains"})
			}
			stack = append(stack, pyCtx{indent: indent, name: name, qname: qn, kind: "class", id: node.ID})
			continue
		}
		if m := pyDefRE.FindStringSubmatch(trim); m != nil {
			name := m[2]
			inClass := len(stack) > 0 && stack[len(stack)-1].kind == "class"
			qn := qualify(stack, name)
			kind := "function"
			if inClass {
				kind = "method"
			}
			sig := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(trim, "async ")), ":")
			doc := nextDocstring(lines, i+1, indent)
			node := model.SymbolNode{ID: stableID(filePath, qn, ln), Kind: kind, Name: name, QualifiedName: qn, FilePath: filePath, Language: "python", StartLine: ln, EndLine: pyBlockEndLine(lines, i, indent), Signature: strPtr(sig), Docstring: doc, IsExported: !strings.HasPrefix(name, "_"), IsAsync: m[1] != "", IsStatic: pendingStatic[indent]}
			delete(pendingStatic, indent)
			res.Nodes = append(res.Nodes, node)
			if parent := containingScope(stack); parent != nil {
				res.Edges = append(res.Edges, model.EdgeRecord{Source: parent.id, Target: node.ID, Kind: "contains"})
			}
			stack = append(stack, pyCtx{indent: indent, name: name, qname: qn, kind: kind, id: node.ID})
			continue
		}
		if current := enclosingFunc(stack); current != nil {
			for _, m := range pyCallRE.FindAllStringSubmatch(trim, -1) {
				calleeText := strings.TrimSuffix(strings.TrimSpace(m[0]), "(")
				simple := calleeText
				if strings.Contains(simple, ".") {
					parts := strings.Split(simple, ".")
					simple = parts[len(parts)-1]
				}
				if isKeywordCall(simple) {
					continue
				}
				res.Edges = append(res.Edges, model.EdgeRecord{Source: current.id, Target: "name:" + simple, Kind: "calls", Line: intPtr(ln), Col: intPtr(strings.Index(line, m[0])), Metadata: map[string]any{"callee_text": calleeText}})
			}
		}
	}
	return res
}

func qualify(stack []pyCtx, name string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].kind == "class" {
			return stack[i].qname + "." + name
		}
	}
	return name
}

func enclosingFunc(stack []pyCtx) *pyCtx {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].kind == "function" || stack[i].kind == "method" {
			return &stack[i]
		}
	}
	return nil
}

func containingScope(stack []pyCtx) *pyCtx {
	if len(stack) == 0 {
		return nil
	}
	return &stack[len(stack)-1]
}

func pyBlockEndLine(lines []string, startIdx, parentIndent int) int {
	end := startIdx + 1
	for i := startIdx + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if indent <= parentIndent {
			break
		}
		end = i + 1
	}
	return end
}

func nextDocstring(lines []string, start, parentIndent int) *string {
	for i := start; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			continue
		}
		indent := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if indent <= parentIndent {
			return nil
		}
		if (strings.HasPrefix(t, `"""`) && strings.HasSuffix(t, `"""`)) || (strings.HasPrefix(t, `'''`) && strings.HasSuffix(t, `'''`)) {
			s := strings.Trim(t, `"'`)
			return &s
		}
		return nil
	}
	return nil
}

func isKeywordCall(s string) bool {
	switch s {
	case "if", "for", "while", "return", "await", "switch", "func":
		return true
	}
	return false
}
