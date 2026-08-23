package oracle_test

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/index"
	"github.com/takara-ai/miru-code/internal/search"
	"github.com/takara-ai/miru-code/internal/tokens"
	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/version"
)

// Token goldens locked to ts_miru_code/tests/tokens.test.ts
func TestTokensParityWithTS(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"HandlerStack", []string{"handlerstack", "handler", "stack"}},
		{"my_func", []string{"my_func", "my", "func"}},
	}
	for _, tc := range cases {
		got := tokens.SplitIdentifier(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitIdentifier(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
	toks := tokens.Tokenize("getHTTPResponse")
	has := map[string]bool{}
	for _, x := range toks {
		has[x] = true
	}
	if !has["gethttpresponse"] || !has["http"] {
		t.Fatalf("tokenize(getHTTPResponse)=%v missing expected pieces", toks)
	}
}

func TestIndexCacheEpoch(t *testing.T) {
	if version.IndexCacheEpoch() != "1" {
		t.Fatalf("epoch=%q want 1 for version %s", version.IndexCacheEpoch(), version.Version)
	}
}

func TestQuantizePersistRoundTripBytes(t *testing.T) {
	vectors := [][]float32{
		normalize([]float32{0.1, -0.2, 0.3, 0.4}),
		normalize([]float32{-0.5, 0.25, 0.1, -0.1}),
		normalize([]float32{0.0, 0.0, 1.0, 0.0}),
	}
	q := index.NewQuantizedVectorIndex(vectors)
	dir := t.TempDir()
	if err := q.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := index.LoadQuantized(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Size() != q.Size() || loaded.Dimensions() != q.Dimensions() {
		t.Fatalf("meta mismatch")
	}
	// codes.bin / scales.bin must be byte-identical after round-trip
	a, _ := os.ReadFile(filepath.Join(dir, "codes.bin"))
	b, _ := os.ReadFile(filepath.Join(dir, "scales.bin"))
	dir2 := t.TempDir()
	if err := loaded.Save(dir2); err != nil {
		t.Fatal(err)
	}
	a2, _ := os.ReadFile(filepath.Join(dir2, "codes.bin"))
	b2, _ := os.ReadFile(filepath.Join(dir2, "scales.bin"))
	if !reflect.DeepEqual(a, a2) || !reflect.DeepEqual(b, b2) {
		t.Fatal("persist round-trip changed binary payloads")
	}
}

func TestBM25EnrichmentAndScores(t *testing.T) {
	chunks := []types.Chunk{
		{Content: "func authenticateUser() {}", FilePath: "src/auth/middleware.go", StartLine: 1, EndLine: 1},
		{Content: "func renderTemplate() {}", FilePath: "src/ui/template.go", StartLine: 1, EndLine: 1},
		{Content: "func hashPassword() {}", FilePath: "src/auth/password.go", StartLine: 1, EndLine: 1},
	}
	bm25 := index.BuildBm25FromChunks(chunks)
	scores := bm25.GetScores(tokens.Tokenize("authenticate middleware"), nil)
	if len(scores) != 3 {
		t.Fatalf("score len %d", len(scores))
	}
	best := 0
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[best] {
			best = i
		}
	}
	if best != 0 {
		t.Fatalf("expected auth/middleware top, got idx=%d scores=%v", best, scores)
	}
}

func TestHybridSearchDeterministicWithMockEmbed(t *testing.T) {
	lang := "go"
	chunks := []types.Chunk{
		{Content: "package auth\nfunc Middleware() {}", FilePath: "auth/middleware.go", StartLine: 1, EndLine: 2, Language: &lang},
		{Content: "package ui\nfunc Button() {}", FilePath: "ui/button.go", StartLine: 1, EndLine: 2, Language: &lang},
		{Content: "package auth\nfunc Login() {}", FilePath: "auth/login.go", StartLine: 1, EndLine: 2, Language: &lang},
	}
	// Fixed mock vectors: query closer to chunk 0
	vecs := [][]float32{
		normalize([]float32{1, 0, 0, 0}),
		normalize([]float32{0, 1, 0, 0}),
		normalize([]float32{0.7, 0.3, 0, 0}),
	}
	bm25 := index.BuildBm25FromChunks(chunks)
	semantic := index.NewQuantizedVectorIndex(vecs)
	backend := &embed.MockBackend{
		ModelName: "mock",
		Dims:      4,
		QueryFn: func(text string) ([]float32, error) {
			return normalize([]float32{1, 0, 0, 0}), nil
		},
		DocsFn: func(texts []string) ([][]float32, error) {
			out := make([][]float32, len(texts))
			for i := range texts {
				out[i] = normalize([]float32{float32(i), 1, 0, 0})
			}
			return out, nil
		},
	}
	results, err := search.HybridSearch(search.HybridOptions{
		Query:         "auth middleware",
		Embeddings:    backend,
		SemanticIndex: semantic,
		BM25Index:     bm25,
		Chunks:        chunks,
		TopK:          2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Chunk.FilePath != "auth/middleware.go" {
		t.Fatalf("top hit=%s want auth/middleware.go (full=%v)", results[0].Chunk.FilePath, pathsOf(results))
	}

	// Second run must be identical (determinism)
	results2, err := search.HybridSearch(search.HybridOptions{
		Query:         "auth middleware",
		Embeddings:    backend,
		SemanticIndex: semantic,
		BM25Index:     bm25,
		Chunks:        chunks,
		TopK:          2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pathsOf(results), pathsOf(results2)) {
		t.Fatalf("non-deterministic: %v vs %v", pathsOf(results), pathsOf(results2))
	}
}

func TestQuantizeCodesMatchTSFormula(t *testing.T) {
	v := []float32{0.5, -0.25, 0.125, 0}
	qv := index.QuantizeVector(v)
	// maxAbs=0.5 → scale=0.5/127, codes = round(v * 127/0.5) clamped
	wantScale := 0.5 / 127
	if math.Abs(qv.Scale-wantScale) > 1e-12 {
		t.Fatalf("scale=%v want %v", qv.Scale, wantScale)
	}
	wantCodes := []int8{127, -64, 32, 0} // round(0.5*254)=127, round(-0.25*254)=-63.5→-64?, let's compute
	inv := float32(127) / 0.5
	for i, x := range v {
		r := math.Round(float64(x * inv))
		if r > 127 {
			r = 127
		}
		if r < -127 {
			r = -127
		}
		wantCodes[i] = int8(r)
	}
	if !reflect.DeepEqual(qv.Codes, wantCodes) {
		t.Fatalf("codes=%v want %v", qv.Codes, wantCodes)
	}
}

func normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	n = math.Sqrt(n)
	out := make([]float32, len(v))
	if n == 0 {
		return out
	}
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

func pathsOf(results []types.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Chunk.FilePath
	}
	return out
}
