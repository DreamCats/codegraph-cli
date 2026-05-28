package extract

import "codegraph-cli/internal/model"

func ExtractFile(path string, source []byte) model.ExtractResult {
	switch DetectLanguage(path) {
	case "go":
		return extractGo(path, source)
	case "python":
		return extractPython(path, source)
	case "typescript", "tsx", "javascript":
		return extractJSLike(path, source)
	default:
		return model.ExtractResult{}
	}
}
