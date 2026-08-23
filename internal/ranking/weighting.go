package ranking

import "regexp"

const (
	alphaSymbol = 0.3
	alphaNL     = 0.5
)

var symbolQueryRe = regexp.MustCompile(`^(?:(?:[A-Za-z_][A-Za-z0-9_]*(?:(?:::|\\|->|\.)[A-Za-z_][A-Za-z0-9_]*)+)|_(?:[A-Za-z0-9_]*)|(?:[A-Za-z][A-Za-z0-9]*[A-Z_][A-Za-z0-9_]*)|(?:[A-Z][A-Za-z0-9]*))$`)

// IsSymbolQuery matches boosting.ts isSymbolQuery.
func IsSymbolQuery(query string) bool {
	return symbolQueryRe.MatchString(trimSpace(query))
}

// ResolveAlpha matches weighting.ts.
func ResolveAlpha(query string, alpha *float64) float64 {
	if alpha != nil {
		return *alpha
	}
	if IsSymbolQuery(query) {
		return alphaSymbol
	}
	return alphaNL
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
