package ranking_test

import (
	"testing"

	"github.com/takara-ai/miru-code/internal/ranking"
	"github.com/takara-ai/miru-code/internal/types"
)

func TestIsSymbolQuery(t *testing.T) {
	if !ranking.IsSymbolQuery("FooBar") {
		t.Fatal("FooBar should be symbol")
	}
	if !ranking.IsSymbolQuery("pkg::Type") {
		t.Fatal("pkg::Type should be symbol")
	}
	if ranking.IsSymbolQuery("where is auth middleware") {
		t.Fatal("NL query should not be symbol")
	}
}

func TestResolveAlpha(t *testing.T) {
	if a := ranking.ResolveAlpha("FooBar", nil); a != 0.3 {
		t.Fatalf("symbol alpha=%v want 0.3", a)
	}
	if a := ranking.ResolveAlpha("find auth", nil); a != 0.5 {
		t.Fatalf("nl alpha=%v want 0.5", a)
	}
	custom := 0.7
	if a := ranking.ResolveAlpha("FooBar", &custom); a != 0.7 {
		t.Fatalf("custom alpha=%v", a)
	}
}

func TestRerankTopkPenalizesTests(t *testing.T) {
	chunks := map[string]types.Chunk{
		"src/auth.go:1:1":  {Content: "auth", FilePath: "src/auth.go", StartLine: 1, EndLine: 1},
		"tests/auth_test.go:1:1": {Content: "auth", FilePath: "tests/auth_test.go", StartLine: 1, EndLine: 1},
	}
	scores := map[string]float64{
		"src/auth.go:1:1":        1.0,
		"tests/auth_test.go:1:1": 1.0,
	}
	ranked := ranking.RerankTopk(scores, chunks, 2, true)
	if len(ranked) == 0 {
		t.Fatal("empty")
	}
	if ranked[0].Chunk.FilePath != "src/auth.go" {
		t.Fatalf("top=%s want src/auth.go", ranked[0].Chunk.FilePath)
	}
}

func TestIsLocationQuery(t *testing.T) {
	if !ranking.IsLocationQuery("where is the entry point") {
		t.Fatal("expected location query")
	}
}
