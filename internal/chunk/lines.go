package chunk

// ChunkBoundary is a half-open character span [start, end) in source text.
// Matches lines.ts: end is exclusive in the sense of a slice end index
// (TS stores end as start+len, and chunking uses end-1 as last inclusive char).
type ChunkBoundary struct {
	Start int
	End   int
}

// LineGroup is a line with its text including line endings.
type LineGroup struct {
	Start int
	End   int
	Text  string
}

// MergeAdjacentChunks merges consecutive boundaries until desiredLength is exceeded.
func MergeAdjacentChunks(chunks []ChunkBoundary, desiredLength int) []ChunkBoundary {
	if len(chunks) == 0 {
		return nil
	}
	first := chunks[0]
	merged := make([]ChunkBoundary, 0, len(chunks))
	currentStart := first.Start
	currentEnd := first.End
	currentLength := currentEnd - currentStart

	for _, group := range chunks[1:] {
		start := group.Start
		end := group.End
		length := end - start

		if currentLength+length > desiredLength {
			merged = append(merged, ChunkBoundary{Start: currentStart, End: currentEnd})
			currentStart = start
			currentEnd = end
			currentLength = length
			continue
		}
		currentEnd = end
		currentLength += length
	}
	merged = append(merged, ChunkBoundary{Start: currentStart, End: currentEnd})
	return merged
}

// SplitLinesKeepEnds splits source into lines that retain their terminators.
func SplitLinesKeepEnds(source string) []LineGroup {
	if source == "" {
		return nil
	}
	groups := make([]LineGroup, 0)
	start := 0
	i := 0
	for i < len(source) {
		if source[i] == '\r' {
			i++
			if i < len(source) && source[i] == '\n' {
				i++
			}
			groups = append(groups, LineGroup{Start: start, End: i, Text: source[start:i]})
			start = i
			continue
		}
		if source[i] == '\n' {
			i++
			groups = append(groups, LineGroup{Start: start, End: i, Text: source[start:i]})
			start = i
			continue
		}
		i++
	}
	if start < len(source) {
		groups = append(groups, LineGroup{Start: start, End: len(source), Text: source[start:]})
	} else if start == len(source) && len(source) > 0 {
		// Trailing empty match after final newline is not emitted (matches TS break).
	}
	// Match TS: final `$` empty match ends the loop without emitting empty text.
	// If source has no trailing newline, the last line was already emitted above.
	// If source ends with newline, last group already includes it.
	return groups
}

// ChunkLines splits source into merged line-based boundaries.
func ChunkLines(source string, desiredLength int) []ChunkBoundary {
	if trimSpaceCheck(source) == "" {
		return nil
	}
	lineGroups := SplitLinesKeepEnds(source)
	boundaries := make([]ChunkBoundary, len(lineGroups))
	for i, g := range lineGroups {
		boundaries[i] = ChunkBoundary{Start: g.Start, End: g.End}
	}
	return MergeAdjacentChunks(boundaries, desiredLength)
}

func trimSpaceCheck(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
