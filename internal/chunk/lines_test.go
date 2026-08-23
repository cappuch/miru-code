package chunk

import "testing"

func TestSplitLinesKeepEnds(t *testing.T) {
	groups := SplitLinesKeepEnds("a\nb\n")
	if len(groups) != 2 {
		t.Fatalf("got %d groups: %#v", len(groups), groups)
	}
	if groups[0].Text != "a\n" || groups[1].Text != "b\n" {
		t.Fatalf("unexpected texts: %#v", groups)
	}

	groups = SplitLinesKeepEnds("hello")
	if len(groups) != 1 || groups[0].Text != "hello" {
		t.Fatalf("no-newline: %#v", groups)
	}
}

func TestChunkLinesMerges(t *testing.T) {
	source := "line1\nline2\nline3\n"
	boundaries := ChunkLines(source, 20)
	if len(boundaries) == 0 {
		t.Fatal("expected boundaries")
	}
	for _, b := range boundaries {
		if b.End <= b.Start {
			t.Fatalf("bad boundary %#v", b)
		}
		if b.End > len(source) {
			t.Fatalf("end past source %#v", b)
		}
	}
}

func TestChunkLinesEmpty(t *testing.T) {
	if ChunkLines("   \n  ", 1500) != nil {
		t.Fatal("expected nil for whitespace-only")
	}
}

func TestChunkSourceLineNumbers(t *testing.T) {
	lang := "python"
	source := "def a():\n  return 1\n\ndef b():\n  return 2\n"
	chunks := ChunkSource(source, "x.py", &lang)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, c := range chunks {
		if c.StartLine < 1 || c.EndLine < c.StartLine {
			t.Fatalf("bad lines %#v", c)
		}
		if c.FilePath != "x.py" {
			t.Fatalf("path %q", c.FilePath)
		}
		if c.Language == nil || *c.Language != "python" {
			t.Fatalf("lang %#v", c.Language)
		}
	}
}
