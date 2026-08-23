package literal_test

import (
	"testing"

	"github.com/takara-ai/miru-code/internal/literal"
	"github.com/takara-ai/miru-code/internal/types"
)

func TestLocateLinesMode(t *testing.T) {
	chunks := []types.Chunk{
		{Content: "const REDIS_HOST = \"localhost\"\n", FilePath: "src/config.ts", StartLine: 1, EndLine: 1},
		{Content: "export function connect() {}\n", FilePath: "src/db.ts", StartLine: 1, EndLine: 1},
	}
	res := literal.Locate(chunks, "REDIS_HOST", literal.LocateOptions{Mode: literal.ModeLines})
	if res.N == 0 {
		t.Fatal("expected hits")
	}
	formatted := literal.FormatLocate(res)
	if formatted["literal"] != "REDIS_HOST" {
		t.Fatalf("literal key missing: %v", formatted)
	}
	if formatted["mode"] != literal.ModeLines {
		t.Fatalf("mode=%v", formatted["mode"])
	}
}

func TestLocateCountMode(t *testing.T) {
	chunks := []types.Chunk{
		{Content: "foo bar foo\n", FilePath: "a.go", StartLine: 1, EndLine: 1},
		{Content: "another foo here\n", FilePath: "b.go", StartLine: 1, EndLine: 1},
	}
	res := literal.Locate(chunks, "foo", literal.LocateOptions{Mode: literal.ModeCount})
	if res.N < 1 {
		t.Fatalf("expected hits, got %d", res.N)
	}
	if res.Files < 2 {
		t.Fatalf("expected 2 files, got %d", res.Files)
	}
}

func TestLocateIgnoreCase(t *testing.T) {
	chunks := []types.Chunk{
		{Content: "HelloWorld\n", FilePath: "a.go", StartLine: 1, EndLine: 1},
	}
	res := literal.Locate(chunks, "helloworld", literal.LocateOptions{Mode: literal.ModeLocations, IgnoreCase: true})
	if res.N == 0 {
		t.Fatal("expected case-insensitive hit")
	}
}
