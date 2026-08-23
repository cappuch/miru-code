package search

import (
	"sort"
	"strings"

	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/index"
	"github.com/takara-ai/miru-code/internal/ranking"
	"github.com/takara-ai/miru-code/internal/tokens"
	"github.com/takara-ai/miru-code/internal/types"
)

const rrfK = 60

func rrfScores(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}
	type pair struct {
		key   string
		score float64
	}
	ranked := make([]pair, 0, len(scores))
	for k, s := range scores {
		ranked = append(ranked, pair{k, s})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	out := make(map[string]float64, len(ranked))
	for i, p := range ranked {
		out[p.key] = 1.0 / float64(rrfK+i+1)
	}
	return out
}

func semanticFromQueryVector(
	queryVec []float32,
	semanticIndex index.SemanticIndex,
	chunks []types.Chunk,
	topK int,
	selector []int,
) ([]types.SearchResult, error) {
	res, err := semanticIndex.Query(queryVec, topK, selector)
	if err != nil {
		return nil, err
	}
	out := make([]types.SearchResult, 0, len(res.Indices))
	for i, idx := range res.Indices {
		if idx < 0 || idx >= len(chunks) {
			continue
		}
		dist := 0.0
		if i < len(res.Distances) {
			dist = res.Distances[i]
		}
		out = append(out, types.SearchResult{Chunk: chunks[idx], Score: 1.0 - dist})
	}
	return out, nil
}

func searchBm25(
	query string,
	bm25Index *index.BM25Index,
	chunks []types.Chunk,
	topK int,
	selector []int,
) []types.SearchResult {
	toks := tokens.Tokenize(query)
	if len(toks) == 0 {
		return nil
	}
	mask := index.SelectorToMask(selector, len(chunks))
	scores := bm25Index.GetScoresAsync(toks, mask)
	indices := index.SelectTopKScoreIndices(scores, topK)
	out := make([]types.SearchResult, 0, len(indices))
	for _, i := range indices {
		if i < 0 || i >= len(scores) || i >= len(chunks) {
			continue
		}
		if scores[i] <= 0 {
			continue
		}
		out = append(out, types.SearchResult{Chunk: chunks[i], Score: scores[i]})
	}
	return out
}

// HybridOptions configures hybridSearch.
type HybridOptions struct {
	Query         string
	Embeddings    embed.EmbeddingBackend
	SemanticIndex index.SemanticIndex
	BM25Index     *index.BM25Index
	Chunks        []types.Chunk
	TopK          int
	Alpha         *float64
	Selector      []int
	Rerank        *bool
}

// HybridSearch blends BM25 + semantic via RRF and optional boost/rerank.
func HybridSearch(opts HybridOptions) ([]types.SearchResult, error) {
	rerank := true
	if opts.Rerank != nil {
		rerank = *opts.Rerank
	}
	alphaWeight := ranking.ResolveAlpha(opts.Query, opts.Alpha)
	candidateCount := opts.TopK * 5
	if ranking.SearchImprovementsEnabled() && ranking.IsLocationQuery(opts.Query) {
		candidateCount = opts.TopK * 10
	}

	chunksByKey := make(map[string]types.Chunk, len(opts.Chunks))
	for _, c := range opts.Chunks {
		chunksByKey[types.ChunkKey(c)] = c
	}

	type embedResult struct {
		vec []float32
		err error
	}
	embedCh := make(chan embedResult, 1)
	go func() {
		vec, err := opts.Embeddings.EmbedQuery(opts.Query)
		embedCh <- embedResult{vec, err}
	}()

	bm25Hits := searchBm25(opts.Query, opts.BM25Index, opts.Chunks, candidateCount, opts.Selector)
	er := <-embedCh
	if er.err != nil {
		return nil, er.err
	}
	semantic, err := semanticFromQueryVector(er.vec, opts.SemanticIndex, opts.Chunks, candidateCount, opts.Selector)
	if err != nil {
		return nil, err
	}

	semanticScores := map[string]float64{}
	for _, r := range semantic {
		semanticScores[types.ChunkKey(r.Chunk)] = r.Score
	}
	bm25Scores := map[string]float64{}
	for _, r := range bm25Hits {
		if r.Score != 0 {
			bm25Scores[types.ChunkKey(r.Chunk)] = r.Score
		}
	}

	normalizedSemantic := rrfScores(semanticScores)
	normalizedBm25 := rrfScores(bm25Scores)

	allKeys := map[string]struct{}{}
	for k := range normalizedSemantic {
		allKeys[k] = struct{}{}
	}
	for k := range normalizedBm25 {
		allKeys[k] = struct{}{}
	}
	sortedKeys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		ca, oka := chunksByKey[sortedKeys[i]]
		cb, okb := chunksByKey[sortedKeys[j]]
		if !oka || !okb {
			return false
		}
		return ca.StartLine < cb.StartLine
	})

	combinedScores := make(map[string]float64, len(sortedKeys))
	for _, key := range sortedKeys {
		combinedScores[key] = alphaWeight*(normalizedSemantic[key]) + (1-alphaWeight)*(normalizedBm25[key])
	}

	if rerank {
		ranking.BoostMultiChunkFiles(combinedScores, chunksByKey)
		ranking.ApplyQueryBoost(combinedScores, opts.Query, opts.Chunks, chunksByKey)
		return ranking.RerankTopk(combinedScores, chunksByKey, opts.TopK, alphaWeight < 1.0), nil
	}

	type scored struct {
		key   string
		score float64
	}
	entries := make([]scored, 0, len(combinedScores))
	for k, s := range combinedScores {
		entries = append(entries, scored{k, s})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].score > entries[j].score })
	if len(entries) > opts.TopK {
		entries = entries[:opts.TopK]
	}
	out := make([]types.SearchResult, 0, len(entries))
	for _, e := range entries {
		chunk, ok := chunksByKey[e.key]
		if !ok {
			continue
		}
		out = append(out, types.SearchResult{Chunk: chunk, Score: e.score})
	}
	return out, nil
}

// SemanticOnlyOptions configures searchSemanticOnly.
type SemanticOnlyOptions struct {
	Query         string
	Embeddings    embed.EmbeddingBackend
	SemanticIndex index.SemanticIndex
	Chunks        []types.Chunk
	TopK          int
	Selector      []int
}

// SearchSemanticOnly embeds the query and returns nearest neighbors.
func SearchSemanticOnly(opts SemanticOnlyOptions) ([]types.SearchResult, error) {
	vec, err := opts.Embeddings.EmbedQuery(opts.Query)
	if err != nil {
		return nil, err
	}
	return semanticFromQueryVector(vec, opts.SemanticIndex, opts.Chunks, opts.TopK, opts.Selector)
}

// TrimQuery is a tiny helper used by callers.
func TrimQuery(q string) string { return strings.TrimSpace(q) }
