package index

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/takara-ai/miru-code/internal/chunk"
	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/types"
)

// ApplyIncrementalFileChanges rebuilds BM25 + semantic for changed relative paths
// (matches incremental.ts: drop old chunks for those files, re-chunk/re-embed, full rebuild).
func ApplyIncrementalFileChanges(
	root string,
	existing []types.Chunk,
	semantic SemanticIndex,
	changedRelPaths []string,
	embeddings embed.EmbeddingBackend,
	content []types.ContentType,
) (*BM25Index, SemanticIndex, []types.Chunk, error) {
	changed := map[string]struct{}{}
	for _, p := range changedRelPaths {
		changed[filepath.ToSlash(p)] = struct{}{}
	}

	kept := make([]types.Chunk, 0, len(existing))
	keptVectors := make([][]float32, 0, len(existing))
	for i, c := range existing {
		if _, drop := changed[filepath.ToSlash(c.FilePath)]; drop {
			continue
		}
		kept = append(kept, c)
		vec, err := semantic.VectorAt(i)
		if err != nil {
			return nil, nil, nil, err
		}
		keptVectors = append(keptVectors, vec)
	}

	var newChunks []types.Chunk
	var newTexts []string
	for _, rel := range changedRelPaths {
		abs := filepath.Join(root, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, nil, err
		}
		parts := chunk.ChunkFile(string(data), rel)
		for _, p := range parts {
			newChunks = append(newChunks, p)
			newTexts = append(newTexts, p.Content)
		}
	}

	var newVectors [][]float32
	if len(newTexts) > 0 {
		vecs, err := embeddings.EmbedDocuments(newTexts)
		if err != nil {
			return nil, nil, nil, err
		}
		if len(vecs) != len(newTexts) {
			return nil, nil, nil, fmt.Errorf("embed count mismatch: got %d want %d", len(vecs), len(newTexts))
		}
		newVectors = vecs
	}

	merged := append(kept, newChunks...)
	mergedVecs := append(keptVectors, newVectors...)
	bm25 := BuildBm25FromChunks(merged)
	sem := BuildSemanticIndex(mergedVecs)
	_ = content
	return bm25, sem, merged, nil
}
