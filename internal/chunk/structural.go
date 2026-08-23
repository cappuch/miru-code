package chunk

import (
	"regexp"
	"sort"
	"strings"
)

var supportedStructuralLanguages = map[string]struct{}{
	"python": {}, "go": {}, "typescript": {}, "javascript": {}, "cpp": {}, "c": {},
}

var braceFamilyLanguages = map[string]struct{}{
	"cpp": {}, "c": {},
}

var (
	pythonDeclRe = regexp.MustCompile(`^(async\s+def|def|class)\b`)
	pythonMethodRe = regexp.MustCompile(`^(async\s+def|def)\b`)

	goDeclRe = regexp.MustCompile(`^\s*(func\b|type\b.*\b(struct|interface)\b)`)
	jsDeclRe = regexp.MustCompile(`^\s*(export\s+)?(async\s+function\b|function\b|class\b|interface\b|type\b|(?:const|let|var)\s+[A-Za-z_$][\w$]*\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)(?:\s*:\s*[^=]+)?\s*=>)`)
	// RE2 has no lookahead; type decls and function-like lines are matched separately then filtered.
	cppTypeDeclRe = regexp.MustCompile(`^\s*(?:(?:template\s*<[^>]*>\s*)?(?:class|struct|namespace|enum(?:\s+class)?)\b)`)
	cppFuncDeclRe = regexp.MustCompile(`^\s*(?:[\w:~<>,*&]+\s+)+\w+\s*\([^;{}]*\)\s*(?:const)?\s*(?:override)?(?:\s*=\s*0)?`)
	cTypeDeclRe   = regexp.MustCompile(`^\s*(?:struct|enum|union|typedef)\b`)
	cFuncDeclRe   = regexp.MustCompile(`^\s*(?:[\w:*\s]+\s+)+\w+\s*\([^;{}]*\)`)

	cppControlRe = regexp.MustCompile(`^\s*(?:if|for|while|switch|catch|return|sizeof|static_assert|else|do)\b`)
	cControlRe   = regexp.MustCompile(`^\s*(?:if|for|while|switch|return|sizeof|else|do)\b`)
)

// ChunkStructural returns structural units merged to desiredLength, or nil if unsupported.
func ChunkStructural(source string, language *string, desiredLength int) []ChunkBoundary {
	if language == nil {
		return nil
	}
	if _, ok := supportedStructuralLanguages[*language]; !ok {
		return nil
	}

	lines := SplitLinesKeepEnds(source)
	if len(lines) == 0 {
		return []ChunkBoundary{}
	}

	var units []ChunkBoundary
	if *language == "python" {
		units = pythonUnits(lines)
	} else {
		units = braceUnits(lines, *language)
	}
	if len(units) == 0 {
		return nil
	}
	return MergeAdjacentChunks(units, desiredLength)
}

func lineIndent(text string) int {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return i
}

func stripLine(text string) string {
	return strings.TrimRight(text, "\r\n")
}

func pythonUnits(lines []LineGroup) []ChunkBoundary {
	units := make([]ChunkBoundary, 0)
	firstDeclStart := -1

	for i := 0; i < len(lines); i++ {
		raw := stripLine(lines[i].Text)
		trimmed := strings.TrimSpace(raw)
		if !pythonDeclRe.MatchString(trimmed) {
			continue
		}
		if firstDeclStart < 0 {
			firstDeclStart = lines[i].Start
		}
		declIndent := lineIndent(raw)

		if strings.HasPrefix(trimmed, "class ") {
			classEndLine := findPythonBlockEnd(lines, i, declIndent)
			methodStarts := make([]int, 0)
			for j := i + 1; j <= classEndLine; j++ {
				innerRaw := stripLine(lines[j].Text)
				innerTrim := strings.TrimSpace(innerRaw)
				if !pythonMethodRe.MatchString(innerTrim) {
					continue
				}
				if lineIndent(innerRaw) > declIndent {
					methodStarts = append(methodStarts, j)
				}
			}
			if len(methodStarts) > 0 {
				firstMethodStart := methodStarts[0]
				firstMethodEnd := findPythonBlockEnd(lines, firstMethodStart, lineIndent(stripLine(lines[firstMethodStart].Text)))
				units = append(units, ChunkBoundary{Start: lines[i].Start, End: lines[firstMethodEnd].End})
				for m := 1; m < len(methodStarts); m++ {
					methodStart := methodStarts[m]
					methodIndent := lineIndent(stripLine(lines[methodStart].Text))
					methodEnd := findPythonBlockEnd(lines, methodStart, methodIndent)
					units = append(units, ChunkBoundary{Start: lines[methodStart].Start, End: lines[methodEnd].End})
				}
			} else {
				units = append(units, ChunkBoundary{Start: lines[i].Start, End: lines[classEndLine].End})
			}
			continue
		}

		startLine := i
		for startLine-1 >= 0 {
			prev := strings.TrimSpace(stripLine(lines[startLine-1].Text))
			if strings.HasPrefix(prev, "@") {
				startLine--
				continue
			}
			break
		}
		endLine := findPythonBlockEnd(lines, i, declIndent)
		if declIndent > 0 {
			continue
		}
		start := lines[startLine].Start
		end := lines[endLine].End
		if end > start {
			units = append(units, ChunkBoundary{Start: start, End: end})
		}
	}

	if firstDeclStart > 0 {
		units = append([]ChunkBoundary{{Start: 0, End: firstDeclStart}}, units...)
	}
	return dedupeAndSort(units)
}

func findPythonBlockEnd(lines []LineGroup, start, indent int) int {
	endLine := len(lines) - 1
	for j := start + 1; j < len(lines); j++ {
		nextRaw := stripLine(lines[j].Text)
		trimmed := strings.TrimSpace(nextRaw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if lineIndent(nextRaw) <= indent {
			endLine = j - 1
			break
		}
	}
	return endLine
}

func isDeclLine(text, language string) bool {
	switch language {
	case "go":
		return goDeclRe.MatchString(text)
	case "cpp":
		if cppTypeDeclRe.MatchString(text) {
			return true
		}
		return cppFuncDeclRe.MatchString(text) && !cppControlRe.MatchString(text)
	case "c":
		if cTypeDeclRe.MatchString(text) {
			return true
		}
		return cFuncDeclRe.MatchString(text) && !cControlRe.MatchString(text)
	default:
		return jsDeclRe.MatchString(text)
	}
}

func isBraceFamilyTypeDecl(text, language string) bool {
	switch language {
	case "cpp":
		return cppTypeDeclRe.MatchString(text)
	case "c":
		return cTypeDeclRe.MatchString(text)
	default:
		return false
	}
}

func braceUnits(lines []LineGroup, language string) []ChunkBoundary {
	units := make([]ChunkBoundary, 0)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
	}
	full := b.String()
	firstDeclStart := -1

	for i := 0; i < len(lines); i++ {
		text := stripLine(lines[i].Text)
		if !isDeclLine(text, language) {
			continue
		}
		if _, ok := braceFamilyLanguages[language]; ok {
			if lineIndent(text) > 0 && !isBraceFamilyTypeDecl(text, language) {
				continue
			}
		}
		if firstDeclStart < 0 {
			firstDeclStart = lines[i].Start
		}

		startOffset := lines[i].Start
		openPos := strings.Index(full[startOffset:], "{")
		if openPos < 0 {
			continue
		}
		openPos += startOffset

		limit := len(full)
		if i+60 < len(lines) {
			limit = lines[i+60].End
		}
		if openPos >= limit {
			continue
		}

		depth := 0
		endPos := -1
		for p := openPos; p < len(full); p++ {
			ch := full[p]
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					endPos = p + 1
					break
				}
			}
		}
		if endPos > startOffset {
			extendedEnd := endPos
			for extendedEnd < len(full) && (full[extendedEnd] == ';' || full[extendedEnd] == ' ' || full[extendedEnd] == '\t') {
				extendedEnd++
			}
			if extendedEnd < len(full) && full[extendedEnd] == '\n' {
				extendedEnd++
			}
			units = append(units, ChunkBoundary{Start: startOffset, End: extendedEnd})
		}
	}

	if firstDeclStart > 0 {
		units = append([]ChunkBoundary{{Start: 0, End: firstDeclStart}}, units...)
	}
	return dedupeAndSort(units)
}

func dedupeAndSort(units []ChunkBoundary) []ChunkBoundary {
	sort.Slice(units, func(i, j int) bool {
		if units[i].Start != units[j].Start {
			return units[i].Start < units[j].Start
		}
		return units[i].End < units[j].End
	})
	out := make([]ChunkBoundary, 0, len(units))
	seen := map[string]struct{}{}
	for _, u := range units {
		key := itoa(u.Start) + ":" + itoa(u.End)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, u)
	}
	return out
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
