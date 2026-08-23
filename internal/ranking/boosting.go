package ranking

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/takara-ai/miru-code/internal/tokens"
	"github.com/takara-ai/miru-code/internal/types"
)

const (
	embeddedStemMinLen         = 4
	embeddedSymbolBoostScale   = 0.5
	definitionBoostMultiplier  = 3.0
	stemBoostMultiplier        = 1.0
	fileCoherenceBoostFrac     = 0.2
)

var (
	embeddedSymbolRe = regexp.MustCompile(`\b(?:[A-Z][a-z][a-zA-Z0-9]*[A-Z][a-zA-Z0-9]*|[a-z][a-zA-Z0-9]*[A-Z][a-zA-Z0-9]+)\b`)
	tokenWordRe      = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)
	stopwords        = map[string]struct{}{}
	definitionCache  = map[string][2]*regexp.Regexp{}
)

func init() {
	for _, w := range strings.Fields("a an and are as at be by do does for from has have how if in is it not of on or the to was what when where which who why with") {
		stopwords[w] = struct{}{}
	}
}

var definitionKeywords = []string{
	"class", "module", "defmodule", "def", "interface", "struct", "enum", "trait",
	"type", "func", "function", "object", "abstract class", "data class", "fn", "fun",
	"package", "namespace", "protocol", "record", "typedef",
}

var sqlDefinitionKeywords = []string{
	"CREATE TABLE", "CREATE VIEW", "CREATE PROCEDURE", "CREATE FUNCTION",
}

func definitionPatterns(symbolName string) [2]*regexp.Regexp {
	if cached, ok := definitionCache[symbolName]; ok {
		return cached
	}
	escaped := regexp.QuoteMeta(symbolName)
	nsPrefix := `(?:[A-Za-z_][A-Za-z0-9_]*(?:\.|::))*`
	suffix := `)\s+` + nsPrefix + escaped + `(?:\s|[<({\:\[;]|$)`
	var keywordBodies []string
	for _, k := range definitionKeywords {
		keywordBodies = append(keywordBodies, strings.ReplaceAll(regexp.QuoteMeta(k), ` `, `\s+`))
	}
	// RE2 has no lookbehind; (^|\s) is equivalent for MatchString scans.
	keywordPrefix := `(?:^|\s)(?:`
	general := regexp.MustCompile(`(?m)` + keywordPrefix + strings.Join(keywordBodies, "|") + suffix)
	sql := regexp.MustCompile(`(?im)` + keywordPrefix + strings.Join(sqlDefinitionKeywords, "|") + suffix)
	cached := [2]*regexp.Regexp{general, sql}
	definitionCache[symbolName] = cached
	return cached
}

func chunkDefinesSymbol(chunk types.Chunk, symbolName string) bool {
	pats := definitionPatterns(symbolName)
	return pats[0].MatchString(chunk.Content) || pats[1].MatchString(chunk.Content)
}

func stemMatches(stem, name string) bool {
	stemNorm := strings.ReplaceAll(stem, "_", "")
	return stem == name || stemNorm == name ||
		strings.TrimSuffix(stem, "s") == name ||
		strings.TrimSuffix(stemNorm, "s") == name
}

func extractSymbolName(query string) string {
	for _, sep := range []string{"::", `\`, "->", "."} {
		if strings.Contains(query, sep) {
			parts := strings.Split(query, sep)
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return strings.TrimSpace(query)
}

func fileStem(filePath string) string {
	base := filepath.Base(filePath)
	ext := path.Ext(base)
	return strings.ToLower(strings.TrimSuffix(base, ext))
}

func definitionTier(chunk types.Chunk, names map[string]struct{}, boostUnit float64) float64 {
	defines := false
	for n := range names {
		if chunkDefinesSymbol(chunk, n) {
			defines = true
			break
		}
	}
	if !defines {
		return 0
	}
	stem := fileStem(chunk.FilePath)
	for n := range names {
		if stemMatches(stem, strings.ToLower(n)) {
			return boostUnit * 1.5
		}
	}
	return boostUnit * 1.0
}

// BoostMultiChunkFiles applies file coherence boost.
func BoostMultiChunkFiles(scores map[string]float64, chunksByKey map[string]types.Chunk) {
	if len(scores) == 0 {
		return
	}
	maxScore := 0.0
	for _, s := range scores {
		if s > maxScore {
			maxScore = s
		}
	}
	if maxScore == 0 {
		return
	}
	fileSum := map[string]float64{}
	bestChunk := map[string]string{}
	for key, score := range scores {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		fp := chunk.FilePath
		fileSum[fp] += score
		if best, ok := bestChunk[fp]; !ok || score > scores[best] {
			bestChunk[fp] = key
		}
	}
	maxFileSum := 0.0
	for _, s := range fileSum {
		if s > maxFileSum {
			maxFileSum = s
		}
	}
	boostUnit := maxScore * fileCoherenceBoostFrac
	for fp, key := range bestChunk {
		scores[key] = scores[key] + boostUnit*(fileSum[fp]/maxFileSum)
	}
}

// ApplyQueryBoost applies symbol/NL boosts.
func ApplyQueryBoost(combinedScores map[string]float64, query string, allChunks []types.Chunk, chunksByKey map[string]types.Chunk) map[string]float64 {
	if len(combinedScores) == 0 {
		return combinedScores
	}
	maxScore := 0.0
	for _, s := range combinedScores {
		if s > maxScore {
			maxScore = s
		}
	}
	if IsSymbolQuery(query) {
		boostSymbolDefinitions(combinedScores, query, maxScore, allChunks, chunksByKey)
	} else {
		boostStemMatches(combinedScores, query, maxScore, chunksByKey)
		boostEmbeddedSymbols(combinedScores, query, maxScore, allChunks, chunksByKey)
		if SearchImprovementsEnabled() {
			BoostExactStemMatches(combinedScores, query, maxScore, chunksByKey)
			BoostLocationSignals(combinedScores, query, maxScore, allChunks, chunksByKey)
			PenalizeInstallerForLocation(combinedScores, query, chunksByKey)
		}
	}
	return combinedScores
}

func boostSymbolDefinitions(boosted map[string]float64, query string, maxScore float64, allChunks []types.Chunk, chunksByKey map[string]types.Chunk) {
	symbolName := extractSymbolName(query)
	names := map[string]struct{}{symbolName: {}}
	if symbolName != strings.TrimSpace(query) {
		names[strings.TrimSpace(query)] = struct{}{}
	}
	boostUnit := maxScore * definitionBoostMultiplier
	for key := range boosted {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		if tier := definitionTier(chunk, names, boostUnit); tier != 0 {
			boosted[key] += tier
		}
	}
	for _, chunk := range allChunks {
		key := types.ChunkKey(chunk)
		if _, ok := boosted[key]; ok {
			continue
		}
		stem := fileStem(chunk.FilePath)
		match := false
		for n := range names {
			if stemMatches(stem, strings.ToLower(n)) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		if tier := definitionTier(chunk, names, boostUnit); tier != 0 {
			boosted[key] = tier
		}
	}
}

func boostEmbeddedSymbols(boosted map[string]float64, query string, maxScore float64, allChunks []types.Chunk, chunksByKey map[string]types.Chunk) {
	matches := embeddedSymbolRe.FindAllString(query, -1)
	if len(matches) == 0 {
		return
	}
	names := map[string]struct{}{}
	symbolsLower := map[string]struct{}{}
	for _, m := range matches {
		names[m] = struct{}{}
		symbolsLower[strings.ToLower(m)] = struct{}{}
	}
	boostUnit := maxScore * definitionBoostMultiplier * embeddedSymbolBoostScale
	for key := range boosted {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		if tier := definitionTier(chunk, names, boostUnit); tier != 0 {
			boosted[key] += tier
		}
	}
	for _, chunk := range allChunks {
		key := types.ChunkKey(chunk)
		if _, ok := boosted[key]; ok {
			continue
		}
		stem := fileStem(chunk.FilePath)
		stemNorm := strings.ReplaceAll(stem, "_", "")
		ok := false
		for symbolLower := range symbolsLower {
			if stem == symbolLower || stemNorm == symbolLower ||
				(len(stem) >= embeddedStemMinLen && strings.HasPrefix(symbolLower, stem)) ||
				(len(stemNorm) >= embeddedStemMinLen && strings.HasPrefix(symbolLower, stemNorm)) {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		if tier := definitionTier(chunk, names, boostUnit); tier != 0 {
			boosted[key] = tier
		}
	}
}

func countKeywordMatches(keywords, parts map[string]struct{}) int {
	exact := 0
	for k := range keywords {
		if _, ok := parts[k]; ok {
			exact++
		}
	}
	if exact == len(keywords) {
		return exact
	}
	nMatches := exact
	for keyword := range keywords {
		if _, ok := parts[keyword]; ok {
			continue
		}
		for part := range parts {
			shorter, longer := keyword, part
			if len(keyword) > len(part) {
				shorter, longer = part, keyword
			}
			if len(shorter) >= 3 && strings.HasPrefix(longer, shorter) {
				nMatches++
				break
			}
		}
	}
	return nMatches
}

func boostStemMatches(boosted map[string]float64, query string, maxScore float64, chunksByKey map[string]types.Chunk) {
	keywords := map[string]struct{}{}
	for _, w := range tokenWordRe.FindAllString(query, -1) {
		if len(w) > 2 {
			lw := strings.ToLower(w)
			if _, stop := stopwords[lw]; !stop {
				keywords[lw] = struct{}{}
			}
		}
	}
	if len(keywords) == 0 {
		return
	}
	boost := maxScore * stemBoostMultiplier
	pathCache := map[string]map[string]struct{}{}
	for key := range boosted {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		parts, ok := pathCache[chunk.FilePath]
		if !ok {
			stem := strings.TrimSuffix(filepath.Base(chunk.FilePath), path.Ext(filepath.Base(chunk.FilePath)))
			parts = map[string]struct{}{}
			for _, p := range tokens.SplitIdentifier(stem) {
				parts[p] = struct{}{}
			}
			parent := filepath.Base(filepath.Dir(chunk.FilePath))
			if parent != "" && parent != "." && parent != ".." {
				for _, p := range tokens.SplitIdentifier(parent) {
					parts[p] = struct{}{}
				}
			}
			pathCache[chunk.FilePath] = parts
		}
		nMatches := countKeywordMatches(keywords, parts)
		if nMatches > 0 {
			matchRatio := float64(nMatches) / float64(len(keywords))
			if matchRatio >= 0.1 {
				boosted[key] += boost * matchRatio
			}
		}
	}
}
