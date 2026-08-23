package tokencount

import (
	"path/filepath"
	"testing"

	"github.com/takara-ai/miru-code/internal/tokenizer"
)

func TestCountTokensRoundTrip(t *testing.T) {
	ResetCache()
	defer ResetCache()

	if TokenCountMethod() != "wordpiece" {
		t.Fatalf("TokenCountMethod = %q, want wordpiece", TokenCountMethod())
	}

	n := CountTokens("Hello world")
	if n <= 0 {
		t.Fatalf("CountTokens = %d, want > 0", n)
	}
	if n != 2 {
		t.Fatalf("CountTokens(Hello world) = %d, want 2", n)
	}

	path := TokenizerJSONPath()
	if path == "" {
		t.Fatal("TokenizerJSONPath empty")
	}
}

func TestLoadBundledTokenizerJSON(t *testing.T) {
	// Round-trip: load the on-disk assets copy (same bytes as embed) and count.
	path := filepath.Join("..", "..", "assets", "tokenizer", "tokenizer.json")
	tok, err := tokenizer.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	n := tok.Count("export async function main() {}")
	if n <= 0 {
		t.Fatalf("count = %d, want > 0", n)
	}
	if n != 8 {
		t.Fatalf("count = %d, want 8 (parity with TS)", n)
	}
}

func TestParitySamples(t *testing.T) {
	ResetCache()
	defer ResetCache()

	expected := map[string]int{
		"export async function hybridSearch() {}":                         10,
		"Hello world":                                                     2,
		"export async function main() {}":                                 8,
		"  semanticIndex: SemanticIndex,":                                 8,
		`import { printBrandBanner } from "./brand-banner.ts";`:           18,
		"async function main(): Promise<void> {":                          11,
		"CLI entry point main command line interface":                     7,
	}
	for sample, want := range expected {
		if got := CountTokens(sample); got != want {
			t.Errorf("CountTokens(%q) = %d, want %d", sample, got, want)
		}
	}
}
