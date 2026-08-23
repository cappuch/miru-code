package snippet

import (
	"os"
	"regexp"
	"strings"

	"github.com/takara-ai/miru-code/internal/env"
	"github.com/takara-ai/miru-code/internal/tokens"
	"github.com/takara-ai/miru-code/internal/types"
)

var snippetStopwords = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range strings.Fields("a an and are as at be by do does for from has have how if in is it not of on or the to was what when where which who why with") {
		m[w] = struct{}{}
	}
	return m
}()

var identWordRe = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_-]*`)

// SnippetMeta describes truncation around an anchor line.
type SnippetMeta struct {
	Truncated     bool `json:"truncated"`
	AnchorLine    int  `json:"anchor_line"`
	FullStartLine int  `json:"full_start_line"`
	FullEndLine   int  `json:"full_end_line"`
}

// SnippetResult is a trimmed chunk plus meta.
type SnippetResult struct {
	Chunk types.Chunk
	Meta  SnippetMeta
}

// ResolveSnippetLines returns MIRU_SNIPPET_LINES or 15.
func ResolveSnippetLines() int {
	if v := env.EnvOptionalInt([]string{"MIRU_SNIPPET_LINES"}, 3); v != nil {
		return *v
	}
	return 15
}

// SearchSnippetsEnabled defaults true unless MIRU_SEARCH_SNIPPETS is 0/false.
func SearchSnippetsEnabled() bool {
	value := os.Getenv("MIRU_SEARCH_SNIPPETS")
	if value == "0" || value == "false" {
		return false
	}
	if value == "1" || value == "true" {
		return true
	}
	return true
}

func queryMatchTerms(query string) map[string]struct{} {
	terms := map[string]struct{}{}
	for _, tok := range tokens.Tokenize(query) {
		if len(tok) >= 3 {
			if _, stop := snippetStopwords[tok]; !stop {
				terms[tok] = struct{}{}
			}
		}
	}
	for _, word := range identWordRe.FindAllString(query, -1) {
		lower := strings.ToLower(word)
		if len(lower) >= 3 {
			if _, stop := snippetStopwords[lower]; !stop {
				terms[lower] = struct{}{}
			}
		}
	}
	return terms
}

func scoreLine(line string, terms map[string]struct{}) int {
	lower := strings.ToLower(line)
	score := 0
	for term := range terms {
		if strings.Contains(lower, term) {
			score++
		}
	}
	return score
}

// AnchorLineOffset picks the 0-based line index inside content that best matches query.
func AnchorLineOffset(content, query string) int {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return 0
	}
	terms := queryMatchTerms(query)
	bestIndex := len(lines) / 2
	bestScore := -1
	for i, line := range lines {
		score := scoreLine(line, terms)
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	return bestIndex
}

// TrimChunkToSnippet shrinks a chunk around the query anchor.
func TrimChunkToSnippet(chunk types.Chunk, query string, linesEachSide int) SnippetResult {
	if linesEachSide < 0 {
		linesEachSide = ResolveSnippetLines()
	}
	lines := strings.Split(chunk.Content, "\n")
	if len(lines) == 0 {
		return SnippetResult{
			Chunk: chunk,
			Meta: SnippetMeta{
				Truncated:     false,
				AnchorLine:    chunk.StartLine,
				FullStartLine: chunk.StartLine,
				FullEndLine:   chunk.EndLine,
			},
		}
	}

	anchorOffset := AnchorLineOffset(chunk.Content, query)
	startOffset := anchorOffset - linesEachSide
	if startOffset < 0 {
		startOffset = 0
	}
	endOffset := anchorOffset + linesEachSide + 1
	if endOffset > len(lines) {
		endOffset = len(lines)
	}
	truncated := startOffset > 0 || endOffset < len(lines)

	if !truncated {
		return SnippetResult{
			Chunk: chunk,
			Meta: SnippetMeta{
				Truncated:     false,
				AnchorLine:    chunk.StartLine + anchorOffset,
				FullStartLine: chunk.StartLine,
				FullEndLine:   chunk.EndLine,
			},
		}
	}

	snippetContent := strings.Join(lines[startOffset:endOffset], "\n")
	out := chunk
	out.Content = snippetContent
	out.StartLine = chunk.StartLine + startOffset
	out.EndLine = chunk.StartLine + endOffset - 1
	return SnippetResult{
		Chunk: out,
		Meta: SnippetMeta{
			Truncated:     true,
			AnchorLine:    chunk.StartLine + anchorOffset,
			FullStartLine: chunk.StartLine,
			FullEndLine:   chunk.EndLine,
		},
	}
}

// AppliedSnippet pairs a search result with snippet meta.
type AppliedSnippet struct {
	Result types.SearchResult
	Meta   SnippetMeta
}

// ApplySnippetsToResults trims each hit around the query.
func ApplySnippetsToResults(results []types.SearchResult, query string, linesEachSide *int) []AppliedSnippet {
	radius := ResolveSnippetLines()
	if linesEachSide != nil {
		radius = *linesEachSide
	}
	out := make([]AppliedSnippet, len(results))
	for i, result := range results {
		sn := TrimChunkToSnippet(result.Chunk, query, radius)
		out[i] = AppliedSnippet{
			Result: types.SearchResult{Chunk: sn.Chunk, Score: result.Score},
			Meta:   sn.Meta,
		}
	}
	return out
}
