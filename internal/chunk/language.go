package chunk

import (
	"path/filepath"
	"strings"
)

var extensionToLanguage = map[string]string{
	".py": "python", ".pyi": "python",
	".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascript",
	".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".go": "go", ".rs": "rust",
	".java": "java", ".kt": "kotlin", ".kts": "kotlin",
	".c": "c",
	".h": "cpp", ".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp",
	".hpp": "cpp", ".hh": "cpp", ".hxx": "cpp", ".h++": "cpp",
	".cs": "csharp", ".rb": "ruby", ".php": "php", ".swift": "swift", ".scala": "scala",
	".clj": "clojure", ".cljs": "clojure",
	".ex": "elixir", ".exs": "elixir", ".erl": "erlang", ".hs": "haskell",
	".lua": "lua", ".sh": "bash", ".bash": "bash", ".zsh": "bash", ".fish": "fish",
	".sql": "sql", ".r": "r", ".R": "r", ".dart": "dart", ".zig": "zig",
	".vue": "vue", ".svelte": "svelte",
	".md": "markdown", ".mdx": "markdown", ".rst": "rst", ".txt": "text",
	".json": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "toml",
	".ini": "ini", ".cfg": "ini", ".xml": "xml",
	".html": "html", ".css": "css", ".scss": "scss", ".less": "less",
	".dockerfile": "dockerfile",
}

// DetectLanguage maps a file path extension to a language tag (files.ts).
func DetectLanguage(filePath string) *string {
	base := strings.ToLower(filepath.Base(filePath))
	if base == "dockerfile" {
		s := "dockerfile"
		return &s
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := extensionToLanguage[ext]; ok {
		return &lang
	}
	return nil
}
