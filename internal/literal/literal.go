package literal

import (
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/takara-ai/miru-code/internal/types"
)

// Mode is count | locations | lines.
type Mode string

const (
	ModeCount     Mode = "count"
	ModeLocations Mode = "locations"
	ModeLines     Mode = "lines"
)

// DefaultMode matches literal.ts.
const DefaultMode = ModeLines

// Hit is one file:line match.
type Hit struct {
	FilePath         string   `json:"file_path"`
	Line             int      `json:"line"`
	Text             *string  `json:"text,omitempty"`
	Context          []string `json:"context,omitempty"`
	ContextStartLine *int     `json:"context_start_line,omitempty"`
}

// LocateResult is the locateLiteral return value.
type LocateResult struct {
	Literal   string   `json:"literal"`
	Literals  []string `json:"literals,omitempty"`
	Mode      Mode     `json:"mode"`
	N         int      `json:"n"`
	Files     int      `json:"files"`
	Truncated bool     `json:"truncated"`
	Hits      []Hit    `json:"hits"`
}

// LocateOptions configures Locate.
type LocateOptions struct {
	Mode          Mode
	Limit         *int
	IgnoreCase    bool
	MatchVariants bool
	Include       []string
	Exclude       []string
	ContextLines  *int
}

func resolveLimit(limit *int) *int {
	if limit == nil {
		return nil
	}
	n := *limit
	if n < 1 {
		n = 1
	}
	return &n
}

func lineMatchesAny(haystack string, needles []string, ignoreCase bool) bool {
	hay := haystack
	if ignoreCase {
		hay = strings.ToLower(haystack)
	}
	for _, needle := range needles {
		n := needle
		if ignoreCase {
			n = strings.ToLower(needle)
		}
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

var sepRe = regexp.MustCompile(`[_\-\s]+`)

func splitIdentifierWords(literal string) []string {
	normalized := sepRe.ReplaceAllString(literal, " ")
	var b strings.Builder
	runes := []rune(normalized)
	for i, r := range runes {
		if i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				if unicode.IsUpper(r) {
					b.WriteByte(' ')
				}
			}
			if unicode.IsUpper(prev) {
				if i+1 < len(runes) && unicode.IsUpper(r) && unicode.IsLower(runes[i+1]) {
					b.WriteByte(' ')
				}
			}
		}
		b.WriteRune(r)
	}
	parts := strings.Fields(b.String())
	out := make([]string, 0, len(parts))
	for _, w := range parts {
		w = strings.TrimSpace(w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func identifierVariants(literal string) []string {
	words := splitIdentifierWords(literal)
	if len(words) == 0 {
		return nil
	}
	lower := make([]string, len(words))
	capitalized := make([]string, len(words))
	for i, w := range words {
		lw := strings.ToLower(w)
		lower[i] = lw
		if lw == "" {
			capitalized[i] = ""
			continue
		}
		capitalized[i] = strings.ToUpper(lw[:1]) + lw[1:]
	}
	camel := lower[0] + strings.Join(capitalized[1:], "")
	pascal := strings.Join(capitalized, "")
	snake := strings.Join(lower, "_")
	kebab := strings.Join(lower, "-")
	constant := strings.ToUpper(snake)
	return []string{camel, pascal, snake, kebab, constant}
}

func expandLiterals(literals []string, matchVariants bool) []string {
	seen := map[string]struct{}{}
	order := make([]string, 0)
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		order = append(order, s)
	}
	for _, lit := range literals {
		if lit == "" {
			continue
		}
		add(lit)
		if matchVariants {
			for _, v := range identifierVariants(lit) {
				add(v)
			}
		}
	}
	return order
}

func matchGitignore(pattern, filePath string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	p := strings.TrimPrefix(filePath, "/")
	pat := pattern
	if strings.HasPrefix(pat, "/") {
		pat = strings.TrimPrefix(pat, "/")
		ok, _ := path.Match(pat, p)
		return ok
	}
	if strings.Contains(pat, "/") {
		ok, _ := path.Match(pat, p)
		if ok {
			return true
		}
		// also try matching as suffix path
		ok, _ = path.Match("*/"+pat, p)
		return ok
	}
	base := path.Base(p)
	ok, _ := path.Match(pat, base)
	if ok {
		return true
	}
	ok, _ = path.Match(pat, p)
	if ok {
		return true
	}
	ok, _ = path.Match("*/"+pat, p)
	return ok
}

func matchesAny(patterns []string, filePath string) bool {
	for _, p := range patterns {
		if matchGitignore(p, filePath) {
			return true
		}
	}
	return false
}

func filterChunksByGlob(chunks []types.Chunk, include, exclude []string) []types.Chunk {
	if len(include) == 0 && len(exclude) == 0 {
		return chunks
	}
	out := make([]types.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		p := strings.TrimLeft(chunk.FilePath, "/")
		if len(include) > 0 && !matchesAny(include, p) {
			continue
		}
		if len(exclude) > 0 && matchesAny(exclude, p) {
			continue
		}
		out = append(out, chunk)
	}
	return out
}

// Locate scans indexed chunks for exact substring matches.
func Locate(chunks []types.Chunk, literal any, options LocateOptions) LocateResult {
	mode := options.Mode
	if mode == "" {
		mode = DefaultMode
	}
	ignoreCase := options.IgnoreCase
	limit := resolveLimit(options.Limit)
	contextLines := 0
	if options.ContextLines != nil && *options.ContextLines >= 0 {
		contextLines = *options.ContextLines
	}

	var inputLiterals []string
	switch v := literal.(type) {
	case string:
		if v != "" {
			inputLiterals = []string{v}
		}
	case []string:
		for _, l := range v {
			if l != "" {
				inputLiterals = append(inputLiterals, l)
			}
		}
	}

	label := strings.Join(inputLiterals, " | ")
	needles := expandLiterals(inputLiterals, options.MatchVariants)
	if len(needles) == 0 {
		return LocateResult{Literal: label, Mode: mode, Hits: []Hit{}}
	}

	scoped := filterChunksByGlob(chunks, options.Include, options.Exclude)
	seen := map[string]struct{}{}
	fileSet := map[string]struct{}{}
	hits := make([]Hit, 0)
	n := 0

	for _, chunk := range scoped {
		if !lineMatchesAny(chunk.Content, needles, ignoreCase) {
			continue
		}
		lines := strings.Split(chunk.Content, "\n")
		for i, lineText := range lines {
			if !lineMatchesAny(lineText, needles, ignoreCase) {
				continue
			}
			line := chunk.StartLine + i
			key := chunk.FilePath + ":" + itoa(line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			fileSet[chunk.FilePath] = struct{}{}
			n++
			if mode == ModeCount {
				continue
			}
			if limit != nil && len(hits) >= *limit {
				continue
			}
			hit := Hit{FilePath: chunk.FilePath, Line: line}
			if mode == ModeLines {
				t := lineText
				hit.Text = &t
				if contextLines > 0 {
					ctxStart := i - contextLines
					if ctxStart < 0 {
						ctxStart = 0
					}
					ctxEnd := i + contextLines
					if ctxEnd > len(lines)-1 {
						ctxEnd = len(lines) - 1
					}
					hit.Context = append([]string(nil), lines[ctxStart:ctxEnd+1]...)
					csl := chunk.StartLine + ctxStart
					hit.ContextStartLine = &csl
				}
			}
			hits = append(hits, hit)
		}
	}

	result := LocateResult{
		Literal:   label,
		Mode:      mode,
		N:         n,
		Files:     len(fileSet),
		Truncated: mode != ModeCount && limit != nil && n > len(hits),
		Hits:      hits,
	}
	if mode == ModeCount {
		result.Hits = []Hit{}
	}
	if len(needles) > 1 {
		result.Literals = needles
	}
	return result
}

// FormatLocate returns compact agent-facing JSON keys.
func FormatLocate(result LocateResult) map[string]any {
	base := map[string]any{
		"literal": result.Literal,
		"mode":    result.Mode,
		"n":       result.N,
		"files":   result.Files,
	}
	if len(result.Literals) > 0 {
		base["literals"] = result.Literals
	}
	if result.Mode == ModeCount {
		return base
	}
	if result.Truncated {
		base["truncated"] = true
	}
	hits := make([]map[string]any, 0, len(result.Hits))
	for _, hit := range result.Hits {
		h := map[string]any{"f": hit.FilePath, "l": hit.Line}
		if hit.Text != nil {
			h["t"] = *hit.Text
		}
		if hit.Context != nil {
			h["ctx"] = hit.Context
			h["ctx_l"] = hit.ContextStartLine
		}
		hits = append(hits, h)
	}
	base["hits"] = hits
	return base
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
