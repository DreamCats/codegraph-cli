package model

const (
	Version     = "0.1.0-go"
	DBFileName  = "codegraph.db"
	MaxFileSize = 1024 * 1024
)

type SymbolNode struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Name          string  `json:"name"`
	QualifiedName string  `json:"qualified_name"`
	FilePath      string  `json:"file_path"`
	Language      string  `json:"language"`
	StartLine     int     `json:"start_line"`
	EndLine       int     `json:"end_line"`
	StartColumn   int     `json:"start_column"`
	EndColumn     int     `json:"end_column"`
	Docstring     *string `json:"docstring,omitempty"`
	Signature     *string `json:"signature,omitempty"`
	IsExported    bool    `json:"is_exported"`
	IsAsync       bool    `json:"is_async"`
	IsStatic      bool    `json:"is_static"`
	ImportModule  *string `json:"import_module,omitempty"`
	ImportSymbol  *string `json:"import_symbol,omitempty"`
}

type EdgeRecord struct {
	Source   string         `json:"source"`
	Target   string         `json:"target"`
	Kind     string         `json:"kind"`
	Line     *int           `json:"line,omitempty"`
	Col      *int           `json:"col,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ExtractResult struct {
	Nodes  []SymbolNode
	Edges  []EdgeRecord
	Errors []string
}

type IndexStats struct {
	FilesScanned         int      `json:"files_scanned"`
	FilesIndexed         int      `json:"files_indexed"`
	FilesSkipped         int      `json:"files_skipped"`
	FilesFailed          int      `json:"files_failed"`
	FilesDeleted         int      `json:"files_deleted"`
	Nodes                int      `json:"nodes"`
	Edges                int      `json:"edges"`
	EdgesResolved        int      `json:"edges_resolved"`
	EdgesStillUnresolved int      `json:"edges_still_unresolved"`
	Errors               []string `json:"errors"`
	ErrorsTotal          int      `json:"errors_total"`
}

type ResolveStats struct {
	EdgesTotal           int `json:"edges_total"`
	EdgesResolvedBefore  int `json:"edges_resolved_before"`
	EdgesResolvedNow     int `json:"edges_resolved_now"`
	EdgesStillUnresolved int `json:"edges_still_unresolved"`
}
