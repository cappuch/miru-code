package chunk

import "testing"

func TestChunkAstGoSource(t *testing.T) {
	src := `package main

import "fmt"

func hello() {
	fmt.Println("hi")
}

func world() {
	fmt.Println("world")
}
`
	lang := "go"
	bounds := ChunkAst(src, "main.go", &lang, 1500)
	if bounds == nil {
		t.Fatal("expected AST boundaries for Go")
	}
	if len(bounds) == 0 {
		t.Fatal("empty boundaries")
	}
	chunks := ChunkSource(src, "main.go", &lang)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	// AST path should produce at least one chunk with package main
	found := false
	for _, c := range chunks {
		if contains(c.Content, "package main") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("chunks missing package main: %+v", chunks)
	}
}

func TestChunkAstDisabled(t *testing.T) {
	t.Setenv("MIRU_AST_CHUNKING", "0")
	lang := "go"
	if ChunkAst("package main\n", "main.go", &lang, 1500) != nil {
		t.Fatal("expected nil when MIRU_AST_CHUNKING=0")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
