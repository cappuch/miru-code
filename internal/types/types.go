package types

// ContentType is what to index: source code, docs, and/or config.
type ContentType string

const (
	ContentCode   ContentType = "code"
	ContentDocs   ContentType = "docs"
	ContentConfig ContentType = "config"
)

// DefaultContentTypes is source code plus config.
var DefaultContentTypes = []ContentType{ContentCode, ContentConfig}

// DefaultContentTypesCopy returns a mutable copy of the defaults.
func DefaultContentTypesCopy() []ContentType {
	out := make([]ContentType, len(DefaultContentTypes))
	copy(out, DefaultContentTypes)
	return out
}

// Chunk is a searchable text span from a source file.
type Chunk struct {
	Content   string  `json:"content"`
	FilePath  string  `json:"file_path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Language  *string `json:"language"`
}

// SearchResult is a ranked hit.
type SearchResult struct {
	Chunk Chunk   `json:"chunk"`
	Score float64 `json:"score"`
}

// ChunkKey returns path:start:end.
func ChunkKey(c Chunk) string {
	return c.FilePath + ":" + itoa(c.StartLine) + ":" + itoa(c.EndLine)
}

// ChunkToDict matches types.ts chunkToDict.
func ChunkToDict(c Chunk) map[string]any {
	var lang any
	if c.Language == nil {
		lang = nil
	} else {
		lang = *c.Language
	}
	return map[string]any{
		"content":    c.Content,
		"file_path":  c.FilePath,
		"start_line": c.StartLine,
		"end_line":   c.EndLine,
		"language":   lang,
		"location":   c.FilePath + ":" + itoa(c.StartLine) + "-" + itoa(c.EndLine),
	}
}

// ChunkFromDict matches types.ts chunkFromDict.
func ChunkFromDict(data map[string]any) Chunk {
	c := Chunk{
		Content:   asString(data["content"]),
		FilePath:  asString(data["file_path"]),
		StartLine: asInt(data["start_line"]),
		EndLine:   asInt(data["end_line"]),
	}
	if data["language"] == nil {
		c.Language = nil
	} else {
		s := asString(data["language"])
		c.Language = &s
	}
	return c
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
