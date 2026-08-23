package ranking

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/takara-ai/miru-code/internal/types"
)

var (
	locationQueryRe   = regexp.MustCompile(`(?i)\b(where(?:'s| is)?|entry\s*point|bootstrap|starts?|live[s]?|located|defined|wiring)\b`)
	entryPointQueryRe = regexp.MustCompile(`(?i)\b(entry\s*point|bootstrap|starts?|wiring|command\s*line)\b`)
	integrationQueryRe = regexp.MustCompile(`(?i)\b(install|uninstall|hook|agent|configure|setup)\b`)
	installerPathRe   = regexp.MustCompile(`(?:^|/)installer(?:/|$)`)
	entryPointContentRe = regexp.MustCompile(`#!/usr/bin|(?:^|\n)\s*(?:async\s+)?function\s+main\s*\(|(?:^|\n)\s*if\s*\(\s*require\.main\s*===\s*module`)
	packageEntryRe    = regexp.MustCompile(`^\[package entry\]`)
	tokenDashRe       = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_-]*`)
)

const (
	locationBoostMultiplier   = 2.5
	installerLocationPenalty  = 0.35
)

var implementationHints = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range []string{"command", "handler", "router", "routing", "middleware", "model", "channel", "request", "response", "dispatch", "pipeline"} {
		m[w] = struct{}{}
	}
	return m
}()

var genericStemDeny = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range []string{
		"route", "routes", "router", "routing", "handler", "middleware", "channel", "model", "models",
		"request", "response", "utils", "util", "server", "client", "application", "dispatch", "core",
		"lib", "main", "mod", "test", "tests", "schema", "config", "helper", "helpers", "engine",
		"pipeline", "plug", "plugs", "command", "commands", "index",
	} {
		m[w] = struct{}{}
	}
	return m
}()

// IsLocationQuery matches location.ts.
func IsLocationQuery(query string) bool { return locationQueryRe.MatchString(query) }

// IsEntryPointQuery matches location.ts.
func IsEntryPointQuery(query string) bool { return entryPointQueryRe.MatchString(query) }

// IsIntegrationQuery matches location.ts.
func IsIntegrationQuery(query string) bool { return integrationQueryRe.MatchString(query) }

func fileStemMatchesQuery(filePath, query string) bool {
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(filePath), path.Ext(filepath.Base(filePath))))
	rawTokens := tokenDashRe.FindAllString(query, -1)
	for i, t := range rawTokens {
		rawTokens[i] = strings.ToLower(t)
	}
	for _, t := range rawTokens {
		if t == stem {
			return true
		}
	}
	for _, token := range rawTokens {
		if strings.Contains(token, "-") {
			for _, p := range strings.Split(token, "-") {
				if p == stem {
					return false
				}
			}
		}
	}
	stemNorm := strings.ReplaceAll(strings.ReplaceAll(stem, "-", ""), "_", "")
	for _, t := range rawTokens {
		tn := strings.ReplaceAll(strings.ReplaceAll(t, "-", ""), "_", "")
		if tn == stemNorm {
			return true
		}
	}
	return false
}

func chunkLooksLikeEntryPoint(chunk types.Chunk) bool {
	if packageEntryRe.MatchString(chunk.Content) {
		return true
	}
	return entryPointContentRe.MatchString(chunk.Content)
}

// BoostLocationSignals boosts entry-point-like chunks for location queries.
func BoostLocationSignals(boosted map[string]float64, query string, maxScore float64, allChunks []types.Chunk, chunksByKey map[string]types.Chunk) {
	if !IsLocationQuery(query) {
		return
	}
	entryPointQuery := IsEntryPointQuery(query)
	packageBoost := maxScore * 0.75
	if entryPointQuery {
		packageBoost = maxScore * locationBoostMultiplier
	}
	codeBoost := maxScore * locationBoostMultiplier
	for _, chunk := range allChunks {
		if !chunkLooksLikeEntryPoint(chunk) {
			continue
		}
		if packageEntryRe.MatchString(chunk.Content) {
			key := types.ChunkKey(chunk)
			boosted[key] = boosted[key] + packageBoost
			chunksByKey[key] = chunk
			continue
		}
		if !entryPointQuery && !fileStemMatchesQuery(chunk.FilePath, query) {
			continue
		}
		key := types.ChunkKey(chunk)
		boosted[key] = boosted[key] + codeBoost
		chunksByKey[key] = chunk
	}
}

// PenalizeInstallerForLocation downranks installer paths for pure location queries.
func PenalizeInstallerForLocation(boosted map[string]float64, query string, chunksByKey map[string]types.Chunk) {
	if !IsLocationQuery(query) || IsIntegrationQuery(query) {
		return
	}
	for key, score := range boosted {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		if installerPathRe.MatchString(strings.ReplaceAll(chunk.FilePath, "\\", "/")) {
			boosted[key] = score * installerLocationPenalty
		}
	}
}

func hasSpecificImplementationTarget(query string, chunksByKey map[string]types.Chunk) bool {
	lowered := strings.ToLower(query)
	for hint := range implementationHints {
		if !strings.Contains(lowered, hint) {
			continue
		}
		for _, chunk := range chunksByKey {
			stem := strings.ToLower(strings.TrimSuffix(filepath.Base(chunk.FilePath), path.Ext(filepath.Base(chunk.FilePath))))
			if stem == hint {
				return true
			}
		}
	}
	return false
}

// BoostExactStemMatches boosts chunks whose file stem matches query tokens.
func BoostExactStemMatches(boosted map[string]float64, query string, maxScore float64, chunksByKey map[string]types.Chunk) {
	keywords := map[string]struct{}{}
	for _, w := range tokenDashRe.FindAllString(query, -1) {
		if len(w) > 2 {
			keywords[strings.ToLower(w)] = struct{}{}
		}
	}
	if len(keywords) == 0 {
		return
	}
	boost := maxScore * 0.75
	preferImplementation := hasSpecificImplementationTarget(query, chunksByKey)
	for key := range boosted {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(chunk.FilePath), path.Ext(filepath.Base(chunk.FilePath))))
		stemNorm := strings.ReplaceAll(strings.ReplaceAll(stem, "-", ""), "_", "")
		for keyword := range keywords {
			if _, deny := genericStemDeny[keyword]; deny {
				continue
			}
			if preferImplementation && stem == keyword {
				if _, hint := implementationHints[stem]; !hint {
					continue
				}
			}
			keywordNorm := strings.ReplaceAll(strings.ReplaceAll(keyword, "-", ""), "_", "")
			if stem == keyword || stemNorm == keywordNorm {
				boosted[key] += boost
				break
			}
		}
	}
}
