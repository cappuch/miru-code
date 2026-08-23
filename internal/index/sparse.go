package index

import (
	"path"
	"strings"

	"github.com/takara-ai/miru-code/internal/tokens"
	"github.com/takara-ai/miru-code/internal/types"
)

// SelectorToMask builds a weight mask from selector indices.
func SelectorToMask(selector []int, size int) []bool {
	if len(selector) == 0 {
		return nil
	}
	mask := make([]bool, size)
	for _, idx := range selector {
		if idx >= 0 && idx < size {
			mask[idx] = true
		}
	}
	return mask
}

func enrichForBm25(content, filePath string) string {
	parts := strings.Split(strings.ReplaceAll(filePath, "\\", "/"), "/")
	stem := ""
	if len(parts) > 0 {
		stem = strings.TrimSuffix(parts[len(parts)-1], path.Ext(parts[len(parts)-1]))
	}
	dirParts := parts[:max(0, len(parts)-1)]
	var dirs []string
	for _, p := range dirParts {
		if p != "" && p != "." && p != ".." {
			dirs = append(dirs, p)
		}
	}
	if len(dirs) > 3 {
		dirs = dirs[len(dirs)-3:]
	}
	dirText := strings.Join(dirs, " ")
	return content + " " + stem + " " + stem + " " + dirText
}

// EnrichChunkForBm25 matches sparse.ts enrichForBm25.
func EnrichChunkForBm25(chunk types.Chunk) string {
	return enrichForBm25(chunk.Content, chunk.FilePath)
}

// AddChunk adds a chunk to BM25.
func AddChunk(b *BM25Index, chunk types.Chunk) {
	b.AddDocument(tokens.Tokenize(EnrichChunkForBm25(chunk)))
}

// BuildBm25FromChunks builds BM25 from chunks.
func BuildBm25FromChunks(chunks []types.Chunk) *BM25Index {
	b := NewBM25Index()
	for _, c := range chunks {
		AddChunk(b, c)
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
