//go:build !cgo

package chunk

import (
	"strings"
	"testing"
)

func TestChunkSourceWithoutCGO(t *testing.T) {
	source := "package main\n\nfunc hello() string { return \"hello\" }\n"
	lang := "go"
	if ChunkAst(source, "main.go", &lang, 1500) != nil {
		t.Fatal("expected fallback without cgo")
	}
	chunks := ChunkSource(source, "main.go", &lang)
	var contents strings.Builder
	for _, c := range chunks {
		if c.FilePath != "main.go" || c.StartLine < 1 || c.EndLine < c.StartLine {
			t.Fatalf("invalid chunk metadata: %+v", c)
		}
		contents.WriteString(c.Content)
	}
	if !strings.Contains(contents.String(), "func hello()") {
		t.Fatalf("fallback lost source function: %q", contents.String())
	}
}
