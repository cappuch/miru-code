package tokens

import "regexp"

var tokenRe = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)

// SplitIdentifier mirrors tokens.ts splitIdentifier.
func SplitIdentifier(token string) []string {
	lower := toLower(token)
	var parts []string

	if containsUnderscore(token) {
		for _, p := range splitUnderscore(lower) {
			if p != "" {
				parts = append(parts, p)
			}
		}
	} else {
		for _, m := range splitCamel(token) {
			parts = append(parts, toLower(m))
		}
	}

	if len(parts) >= 2 {
		out := make([]string, 0, 1+len(parts))
		out = append(out, lower)
		out = append(out, parts...)
		return out
	}
	return []string{lower}
}

// splitCamel mirrors /[A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|[0-9]+/g without lookaheads.
func splitCamel(token string) []string {
	if token == "" {
		return nil
	}
	var parts []string
	i := 0
	for i < len(token) {
		c := token[i]
		if c >= '0' && c <= '9' {
			j := i + 1
			for j < len(token) && token[j] >= '0' && token[j] <= '9' {
				j++
			}
			parts = append(parts, token[i:j])
			i = j
			continue
		}
		if c >= 'a' && c <= 'z' {
			j := i + 1
			for j < len(token) && token[j] >= 'a' && token[j] <= 'z' {
				j++
			}
			parts = append(parts, token[i:j])
			i = j
			continue
		}
		if c >= 'A' && c <= 'Z' {
			j := i + 1
			for j < len(token) && token[j] >= 'A' && token[j] <= 'Z' {
				j++
			}
			// [A-Z]+(?=[A-Z][a-z]) — peel last uppercase into next word when followed by lowercase.
			if j < len(token) && token[j] >= 'a' && token[j] <= 'z' && j-i >= 2 {
				parts = append(parts, token[i:j-1])
				i = j - 1
				continue
			}
			if j == i+1 && j < len(token) && token[j] >= 'a' && token[j] <= 'z' {
				// [A-Z]?[a-z]+ starting with this capital
				k := j + 1
				for k < len(token) && token[k] >= 'a' && token[k] <= 'z' {
					k++
				}
				parts = append(parts, token[i:k])
				i = k
				continue
			}
			parts = append(parts, token[i:j])
			i = j
			continue
		}
		i++
	}
	return parts
}

// Tokenize mirrors tokens.ts tokenize.
func Tokenize(text string) []string {
	raw := tokenRe.FindAllString(text, -1)
	result := make([]string, 0, len(raw))
	for _, tok := range raw {
		result = append(result, SplitIdentifier(tok)...)
	}
	return result
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsUnderscore(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			return true
		}
	}
	return false
}

func splitUnderscore(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
