package extract

import (
	"path/filepath"
	"strings"
)

var supportedExt = map[string]string{
	".py": "python", ".pyi": "python",
	".ts": "typescript", ".tsx": "tsx", ".mts": "typescript", ".cts": "typescript",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".go": "go",
}

func DetectLanguage(path string) string {
	return supportedExt[strings.ToLower(filepath.Ext(path))]
}

func SupportedExtensions() map[string]string {
	out := make(map[string]string, len(supportedExt))
	for ext, lang := range supportedExt {
		out[ext] = lang
	}
	return out
}
