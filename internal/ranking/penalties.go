package ranking

import (
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/takara-ai/miru-code/internal/types"
)

var (
	testFileRe = regexp.MustCompile(`(?:^|/)(?:test_[^/]*\.py|[^/]*_test\.py|[^/]*_test\.go|[^/]*Tests?\.java|[^/]*Test\.php|[^/]*_spec\.rb|[^/]*_test\.rb|[^/]*\.test\.[jt]sx?|[^/]*\.spec\.[jt]sx?|[^/]*Tests?\.kt|[^/]*Spec\.kt|[^/]*Tests?\.swift|[^/]*Spec\.swift|[^/]*Tests?\.cs|test_[^/]*\.cpp|[^/]*_test\.cpp|test_[^/]*\.c|[^/]*_test\.c|[^/]*Spec\.scala|[^/]*Suite\.scala|[^/]*Test\.scala|[^/]*_test\.dart|test_[^/]*\.dart|[^/]*_spec\.lua|[^/]*_test\.lua|test_[^/]*\.lua|test_helpers?[^/]*\.\w+)$`)
	testDirRe  = regexp.MustCompile(`(?:^|/)(?:tests?|__tests__|spec|testing)(?:/|$)`)
	compatDirRe = regexp.MustCompile(`(?:^|/)(?:compat|_compat|legacy)(?:/|$)`)
	examplesDirRe = regexp.MustCompile(`(?:^|/)(?:_?examples?|docs?_src)(?:/|$)`)
	typeDefsRe = regexp.MustCompile(`\.d\.ts$`)
)

const (
	strongPenalty            = 0.3
	moderatePenalty          = 0.5
	mildPenalty              = 0.7
	fileSaturationThreshold  = 1
	fileSaturationDecay      = 0.5
)

var reexportFilenames = map[string]struct{}{
	"__init__.py":        {},
	"package-info.java":  {},
}

func filePathPenalty(filePath string) float64 {
	normalised := strings.ReplaceAll(filePath, "\\", "/")
	penalty := 1.0
	if testFileRe.MatchString(normalised) || testDirRe.MatchString(normalised) {
		penalty *= strongPenalty
	}
	if _, ok := reexportFilenames[filepath.Base(filePath)]; ok {
		penalty *= moderatePenalty
	}
	if compatDirRe.MatchString(normalised) {
		penalty *= strongPenalty
	}
	if examplesDirRe.MatchString(normalised) {
		penalty *= strongPenalty
	}
	if typeDefsRe.MatchString(normalised) {
		penalty *= mildPenalty
	}
	return penalty
}

type scoredKey struct {
	key   string
	score float64
}

// RerankTopk applies path penalties and file saturation, returns topK.
func RerankTopk(scores map[string]float64, chunksByKey map[string]types.Chunk, topK int, penalisePaths bool) []types.SearchResult {
	if len(scores) == 0 {
		return nil
	}
	penaltyCache := map[string]float64{}
	penalised := map[string]float64{}
	for key, score := range scores {
		chunk, ok := chunksByKey[key]
		if !ok {
			continue
		}
		pen := score
		if penalisePaths {
			mult, ok := penaltyCache[chunk.FilePath]
			if !ok {
				mult = filePathPenalty(chunk.FilePath)
				penaltyCache[chunk.FilePath] = mult
			}
			pen *= mult
		}
		penalised[key] = pen
	}
	ranked := make([]scoredKey, 0, len(penalised))
	for k, s := range penalised {
		ranked = append(ranked, scoredKey{k, s})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	fileSelected := map[string]int{}
	selected := []scoredKey{}
	minSelected := math.Inf(1)

	for _, item := range ranked {
		chunk, ok := chunksByKey[item.key]
		if !ok {
			continue
		}
		if len(selected) >= topK && item.score <= minSelected {
			break
		}
		already := fileSelected[chunk.FilePath]
		effScore := item.score
		if already >= fileSaturationThreshold {
			excess := already - fileSaturationThreshold + 1
			effScore *= math.Pow(fileSaturationDecay, float64(excess))
		}
		selected = append(selected, scoredKey{item.key, effScore})
		fileSelected[chunk.FilePath] = already + 1
		if len(selected) >= topK {
			minSelected = math.Inf(1)
			for _, s := range selected {
				if s.score < minSelected {
					minSelected = s.score
				}
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].score > selected[j].score })
	if len(selected) > topK {
		selected = selected[:topK]
	}
	out := make([]types.SearchResult, 0, len(selected))
	for _, s := range selected {
		chunk, ok := chunksByKey[s.key]
		if !ok {
			continue
		}
		out = append(out, types.SearchResult{Chunk: chunk, Score: s.score})
	}
	return out
}
