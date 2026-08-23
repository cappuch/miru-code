package mcp

import (
	"fmt"
	"strings"

	"github.com/takara-ai/miru-code/internal/utils"
)

// FormatResultsText renders formatResults() output as plain text.
func FormatResultsText(payload map[string]any) string {
	results, _ := payload["results"].([]map[string]any)
	if results == nil {
		if raw, ok := payload["results"].([]any); ok {
			results = make([]map[string]any, 0, len(raw))
			for _, item := range raw {
				if m, ok := item.(map[string]any); ok {
					results = append(results, m)
				}
			}
		}
	}
	if len(results) == 0 {
		return "No results found."
	}
	blocks := make([]string, 0, len(results))
	for _, result := range results {
		chunk, _ := result["chunk"].(map[string]any)
		if chunk == nil {
			continue
		}
		startLine := chunk["start_line"]
		endLine := chunk["end_line"]
		truncatedNote := ""
		if trunc, _ := chunk["truncated"].(bool); trunc {
			startLine = chunk["full_start_line"]
			endLine = chunk["full_end_line"]
			truncatedNote = " [truncated: call expand for full chunk]"
		}
		header := fmt.Sprintf("%v:%v-%v%s", chunk["file_path"], startLine, endLine, truncatedNote)
		blocks = append(blocks, header+"\n"+fmt.Sprint(chunk["content"]))
	}
	return strings.Join(blocks, "\n\n")
}

// FormatLiteralLocateText renders locate payload as plain text.
func FormatLiteralLocateText(payload map[string]any) string {
	label := ""
	if lits, ok := payload["literals"].([]string); ok && len(lits) > 0 {
		label = strings.Join(lits, " | ")
	} else if raw, ok := payload["literals"].([]any); ok && len(raw) > 0 {
		parts := make([]string, 0, len(raw))
		for _, item := range raw {
			parts = append(parts, fmt.Sprint(item))
		}
		label = strings.Join(parts, " | ")
	} else {
		label = fmt.Sprint(payload["literal"])
	}
	n := asInt(payload["n"])
	files := asInt(payload["files"])
	truncatedNote := ""
	if trunc, _ := payload["truncated"].(bool); trunc {
		truncatedNote = " (truncated — pass a narrower include/exclude)"
	}
	matchWord := "matches"
	if n == 1 {
		matchWord = "match"
	}
	fileWord := "files"
	if files == 1 {
		fileWord = "file"
	}
	header := fmt.Sprintf(`"%s": %d %s across %d %s%s`, label, n, matchWord, files, fileWord, truncatedNote)
	hitsRaw, ok := payload["hits"]
	if !ok {
		return header
	}
	hits, _ := hitsRaw.([]map[string]any)
	if hits == nil {
		if arr, ok := hitsRaw.([]any); ok {
			hits = make([]map[string]any, 0, len(arr))
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					hits = append(hits, m)
				}
			}
		}
	}
	lines := []string{header, ""}
	for _, hit := range hits {
		if t, ok := hit["t"]; ok {
			lines = append(lines, fmt.Sprintf("%v:%v: %v", hit["f"], hit["l"], t))
		} else {
			lines = append(lines, fmt.Sprintf("%v:%v", hit["f"], hit["l"]))
		}
		if ctx, ok := hit["ctx"].([]string); ok {
			ctxStart := asInt(hit["ctx_l"])
			for i, line := range ctx {
				lines = append(lines, fmt.Sprintf("  %d: %s", ctxStart+i, line))
			}
		} else if ctxAny, ok := hit["ctx"].([]any); ok {
			ctxStart := asInt(hit["ctx_l"])
			for i, line := range ctxAny {
				lines = append(lines, fmt.Sprintf("  %d: %v", ctxStart+i, line))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// FormatExpandResultsText renders expand payload as plain text.
func FormatExpandResultsText(payload utils.ExpandResults) string {
	var anchorRange string
	if payload.Anchor != nil {
		anchorRange = fmt.Sprintf("%v-%v", payload.Anchor["start_line"], payload.Anchor["end_line"])
	}
	chunkWord := "chunks"
	if payload.ChunkCount == 1 {
		chunkWord = "chunk"
	}
	header := fmt.Sprintf("%s — %d %s around line %d", payload.FilePath, payload.ChunkCount, chunkWord, payload.Line)
	blocks := make([]string, 0, len(payload.Chunks))
	for _, chunk := range payload.Chunks {
		rng := fmt.Sprintf("%v-%v", chunk["start_line"], chunk["end_line"])
		suffix := ""
		if anchorRange != "" && rng == anchorRange {
			suffix = " (anchor)"
		}
		blocks = append(blocks, rng+suffix+"\n"+fmt.Sprint(chunk["content"]))
	}
	return header + "\n\n" + strings.Join(blocks, "\n\n")
}

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}
