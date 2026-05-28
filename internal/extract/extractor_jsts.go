package extract

import (
	"github.com/DreamCats/codegraph-cli/internal/model"
	"regexp"
	"sort"
	"strings"
)

var (
	jsImportNamedRE = regexp.MustCompile(`import\s+\{([^}]+)\}\s+from\s+['"]([^'"]+)['"]`)
	jsImportNSRE    = regexp.MustCompile(`import\s+\*\s+as\s+([A-Za-z_$][\w$]*)\s+from\s+['"]([^'"]+)['"]`)
	jsImportDefRE   = regexp.MustCompile(`import\s+([A-Za-z_$][\w$]*)\s+from\s+['"]([^'"]+)['"]`)
	jsClassRE       = regexp.MustCompile(`(export\s+)?class\s+([A-Za-z_$][\w$]*)`)
	jsFuncRE        = regexp.MustCompile(`(export\s+)?(async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\(`)
	jsArrowRE       = regexp.MustCompile(`(export\s+)?const\s+([A-Za-z_$][\w$]*)\s*=\s*(async\s+)?\([^)]*\)\s*=>`)
	jsMethodRE      = regexp.MustCompile(`(?m)^\s*(static\s+)?(async\s+)?([A-Za-z_$][\w$]*)\s*\(`)
	jsCallRE        = regexp.MustCompile(`(?:new\s+)?([A-Za-z_$][\w$]*)(?:\.([A-Za-z_$][\w$]*))?\s*\(`)
)

func extractJSLike(filePath string, source []byte) model.ExtractResult {
	res := model.ExtractResult{}
	lang := DetectLanguage(filePath)
	text := string(source)
	lines := lineOffsets(text)
	for _, m := range jsImportNamedRE.FindAllStringSubmatchIndex(text, -1) {
		body := text[m[2]:m[3]]
		module := text[m[4]:m[5]]
		line := lineForOffset(lines, m[0])
		for _, part := range strings.Split(body, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			fields := strings.Fields(part)
			sym, local := fields[0], fields[0]
			if len(fields) >= 3 && fields[1] == "as" {
				local = fields[2]
			}
			qn := module + "." + sym
			res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, qn, line), Kind: "import", Name: local, QualifiedName: qn, FilePath: filePath, Language: lang, StartLine: line, EndLine: line, ImportModule: strPtr(module), ImportSymbol: strPtr(sym)})
		}
	}
	for _, m := range jsImportNSRE.FindAllStringSubmatchIndex(text, -1) {
		local, module := text[m[2]:m[3]], text[m[4]:m[5]]
		line := lineForOffset(lines, m[0])
		res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, local, line), Kind: "import", Name: local, QualifiedName: local, FilePath: filePath, Language: lang, StartLine: line, EndLine: line, ImportModule: strPtr(module), ImportSymbol: strPtr("*")})
	}
	for _, m := range jsImportDefRE.FindAllStringSubmatchIndex(text, -1) {
		if strings.Contains(text[m[0]:m[1]], "{") || strings.Contains(text[m[0]:m[1]], "*") {
			continue
		}
		local, module := text[m[2]:m[3]], text[m[4]:m[5]]
		line := lineForOffset(lines, m[0])
		res.Nodes = append(res.Nodes, model.SymbolNode{ID: stableID(filePath, local, line), Kind: "import", Name: local, QualifiedName: local, FilePath: filePath, Language: lang, StartLine: line, EndLine: line, ImportModule: strPtr(module), ImportSymbol: strPtr("default")})
	}
	for _, m := range jsClassRE.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[4]:m[5]]
		line := lineForOffset(lines, m[0])
		exported := strings.TrimSpace(matchText(text, m, 2)) != ""
		bodyStart := strings.Index(text[m[1]:], "{")
		endLine := line
		if bodyStart >= 0 {
			start := m[1] + bodyStart + 1
			end := matchingBrace(text, start-1)
			if end > start {
				endLine = lineForOffset(lines, end)
			}
		}
		node := model.SymbolNode{ID: stableID(filePath, name, line), Kind: "class", Name: name, QualifiedName: name, FilePath: filePath, Language: lang, StartLine: line, EndLine: endLine, IsExported: exported}
		res.Nodes = append(res.Nodes, node)
		if bodyStart >= 0 {
			start := m[1] + bodyStart + 1
			end := matchingBrace(text, start-1)
			if end > start {
				body := text[start:end]
				for _, mm := range jsMethodRE.FindAllStringSubmatchIndex(body, -1) {
					mname := body[mm[6]:mm[7]]
					if mname == "if" || mname == "for" || mname == "while" || mname == "switch" {
						continue
					}
					ml := lineForOffset(lines, start+mm[0])
					qn := name + "." + mname
					methodEnd := jsFunctionEndLine(body, mm[0], start, lines)
					method := model.SymbolNode{ID: stableID(filePath, qn, ml), Kind: "method", Name: mname, QualifiedName: qn, FilePath: filePath, Language: lang, StartLine: ml, EndLine: methodEnd, IsExported: true, IsStatic: strings.TrimSpace(matchText(body, mm, 2)) != "", IsAsync: strings.TrimSpace(matchText(body, mm, 4)) != ""}
					res.Nodes = append(res.Nodes, method)
					res.Edges = append(res.Edges, model.EdgeRecord{Source: node.ID, Target: method.ID, Kind: "contains"})
					emitJSCalls(&res, method.ID, body[mm[1]:], start+mm[1], lines)
				}
			}
		}
	}
	for _, m := range jsFuncRE.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[6]:m[7]]
		line := lineForOffset(lines, m[0])
		node := model.SymbolNode{ID: stableID(filePath, name, line), Kind: "function", Name: name, QualifiedName: name, FilePath: filePath, Language: lang, StartLine: line, EndLine: jsFunctionEndLine(text, m[0], 0, lines), IsExported: strings.TrimSpace(matchText(text, m, 2)) != "", IsAsync: strings.TrimSpace(matchText(text, m, 4)) != ""}
		res.Nodes = append(res.Nodes, node)
		emitJSCalls(&res, node.ID, text[m[1]:], m[1], lines)
	}
	for _, m := range jsArrowRE.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[4]:m[5]]
		line := lineForOffset(lines, m[0])
		node := model.SymbolNode{ID: stableID(filePath, name, line), Kind: "function", Name: name, QualifiedName: name, FilePath: filePath, Language: lang, StartLine: line, EndLine: jsFunctionEndLine(text, m[0], 0, lines), IsExported: strings.TrimSpace(matchText(text, m, 2)) != "", IsAsync: strings.TrimSpace(matchText(text, m, 6)) != ""}
		res.Nodes = append(res.Nodes, node)
		emitJSCalls(&res, node.ID, text[m[1]:], m[1], lines)
	}
	return res
}

func emitJSCalls(res *model.ExtractResult, sourceID, body string, base int, lines []int) {
	end := strings.Index(body, "\n}")
	if end < 0 || end > 1000 {
		end = min(len(body), 1000)
	}
	body = body[:end]
	for _, m := range jsCallRE.FindAllStringSubmatchIndex(body, -1) {
		name := body[m[2]:m[3]]
		simple := name
		callee := name
		if m[4] >= 0 {
			simple = body[m[4]:m[5]]
			callee = name + "." + simple
		}
		if isKeywordCall(simple) || simple == "function" {
			continue
		}
		line := lineForOffset(lines, base+m[0])
		res.Edges = append(res.Edges, model.EdgeRecord{Source: sourceID, Target: "name:" + simple, Kind: "calls", Line: intPtr(line), Col: intPtr(0), Metadata: map[string]any{"callee_text": callee}})
	}
}

func matchText(s string, match []int, groupStart int) string {
	if groupStart+1 >= len(match) || match[groupStart] < 0 || match[groupStart+1] < 0 {
		return ""
	}
	return s[match[groupStart]:match[groupStart+1]]
}

func jsFunctionEndLine(s string, relStart, base int, lines []int) int {
	open := strings.Index(s[relStart:], "{")
	if open < 0 {
		return lineForOffset(lines, base+relStart)
	}
	end := matchingBrace(s, relStart+open)
	if end < 0 {
		return lineForOffset(lines, base+relStart)
	}
	return lineForOffset(lines, base+end)
}

func lineOffsets(s string) []int {
	out := []int{0}
	for i, ch := range s {
		if ch == '\n' {
			out = append(out, i+1)
		}
	}
	return out
}

func lineForOffset(lines []int, off int) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i] > off })
	if i == 0 {
		return 1
	}
	return i
}

func matchingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
