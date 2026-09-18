// Package miru provides programmatic code indexing and search.
// The module path remains github.com/takara-ai/miru-code for compatibility
// with existing consumers of the cappuch/miru-code fork.
package miru

import (
	"github.com/takara-ai/miru-code/internal/miruindex"
	"github.com/takara-ai/miru-code/internal/types"
)

type Chunk = types.Chunk
type ContentType = types.ContentType
type SearchResult = types.SearchResult

const (
	ContentCode   = types.ContentCode
	ContentDocs   = types.ContentDocs
	ContentConfig = types.ContentConfig
)

// MiruIndex exposes the current index with the original positional Search API.
// Other index operations, including Chunks, Save, and FindRelated, are promoted.
type MiruIndex struct {
	*miruindex.MiruIndex
}

// FromPath indexes a local directory, optionally selecting an embedding model.
func FromPath(path string, content []ContentType, embeddingModel ...string) (*MiruIndex, error) {
	var model *string
	if len(embeddingModel) > 0 {
		model = &embeddingModel[0]
	}
	idx, err := miruindex.FromPath(path, content, model)
	if err != nil {
		return nil, err
	}
	return &MiruIndex{MiruIndex: idx}, nil
}

// Search runs hybrid keyword and semantic retrieval, with optional filters.
func (m *MiruIndex) Search(query string, topK int, alpha *float64, filterLanguages, filterPaths []string, rerank *bool) ([]SearchResult, error) {
	return m.MiruIndex.Search(miruindex.SearchOptions{
		Query: query, TopK: topK, Alpha: alpha,
		FilterLanguages: filterLanguages, FilterPaths: filterPaths, Rerank: rerank,
	})
}

// SaveToDefaultCache preserves the original cache convenience method.
func (m *MiruIndex) SaveToDefaultCache(sourcePath string) error {
	return m.SaveToCache(sourcePath, false)
}
