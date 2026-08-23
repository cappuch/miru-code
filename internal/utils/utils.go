package utils

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/takara-ai/miru-code/internal/snippet"
	"github.com/takara-ai/miru-code/internal/types"
)

var gitURLSchemes = []string{
	"https://", "http://", "ssh://", "git://", "git+ssh://", "file://",
}

var scpGitURLRe = regexp.MustCompile(`^[\w.-]+@[\w.-]+:`)

// IsGitURL reports whether path looks like a git remote.
func IsGitURL(path string) bool {
	for _, scheme := range gitURLSchemes {
		if strings.HasPrefix(path, scheme) {
			return true
		}
	}
	// SCP-like: user@host:path — exclude user@host:/absolute (not used by git SCP).
	loc := scpGitURLRe.FindStringIndex(path)
	if loc == nil {
		return false
	}
	rest := path[loc[1]:]
	return rest != "" && !strings.HasPrefix(rest, "/")
}

// HTTPGitCloneAllowed checks MIRU_ALLOW_HTTP_GIT.
func HTTPGitCloneAllowed() bool {
	raw := os.Getenv("MIRU_ALLOW_HTTP_GIT")
	return raw == "1" || raw == "true"
}

// IsAllowedRepoSource validates local paths and https (or allowed http) remotes.
func IsAllowedRepoSource(repo string) bool {
	if !IsGitURL(repo) {
		return true
	}
	if strings.HasPrefix(repo, "https://") {
		return true
	}
	if strings.HasPrefix(repo, "http://") {
		return HTTPGitCloneAllowed()
	}
	return false
}

// LocalRepoRoot returns absolute path for local repos, nil for remotes.
func LocalRepoRoot(repo string) *string {
	if IsGitURL(repo) {
		return nil
	}
	resolved, err := filepath.Abs(repo)
	if err != nil {
		return nil
	}
	return &resolved
}

// ValidateLocalRepoPath errors when MIRU_WORKSPACE_ROOT is set and path escapes it.
func ValidateLocalRepoPath(repo string) error {
	workspaceRoot := strings.TrimSpace(os.Getenv("MIRU_WORKSPACE_ROOT"))
	if workspaceRoot == "" || IsGitURL(repo) {
		return nil
	}
	resolvedRepo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	resolvedWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	sep := string(os.PathSeparator)
	prefix := resolvedWorkspace
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	if resolvedRepo != resolvedWorkspace && !strings.HasPrefix(resolvedRepo, prefix) {
		return &pathOutsideWorkspaceError{repo: repo}
	}
	return nil
}

type pathOutsideWorkspaceError struct{ repo string }

func (e *pathOutsideWorkspaceError) Error() string {
	return "Local repo path is outside workspace: " + e.repo
}

// ToIndexedFilePath maps absolute/repo-relative paths to index-relative form.
func ToIndexedFilePath(filePath string, repoRoot *string) string {
	if repoRoot == nil || *repoRoot == "" {
		return filePath
	}
	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		return filePath
	}
	candidate := filePath
	if !filepath.IsAbs(filePath) {
		candidate = filepath.Join(root, filePath)
	} else {
		candidate, _ = filepath.Abs(filePath)
	}
	if candidate == root {
		return ""
	}
	sep := string(os.PathSeparator)
	prefix := root
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	if strings.HasPrefix(candidate, prefix) {
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			return filePath
		}
		return filepath.ToSlash(rel)
	}
	return filePath
}

// ResolveChunk finds the chunk covering a line (preferring non-end-boundary).
func ResolveChunk(chunks []types.Chunk, filePath string, line int, repoRoot *string) *types.Chunk {
	indexedPath := ToIndexedFilePath(filePath, repoRoot)
	var fallback *types.Chunk
	for i := range chunks {
		chunk := &chunks[i]
		if chunk.FilePath == indexedPath && chunk.StartLine <= line && line <= chunk.EndLine {
			if line < chunk.EndLine {
				return chunk
			}
			if fallback == nil {
				fallback = chunk
			}
		}
	}
	return fallback
}

// ExpandResults is the expand tool payload.
type ExpandResults struct {
	FilePath   string           `json:"file_path"`
	Line       int              `json:"line"`
	ChunkCount int              `json:"chunk_count"`
	Anchor     map[string]any   `json:"anchor"`
	Chunks     []map[string]any `json:"chunks"`
}

const (
	DefaultMCPTopK      = 3
	MaxMCPTopK          = 10
	DefaultExpandBefore = 1
	DefaultExpandAfter  = 1
)

// ClampMCPTopK clamps MCP top_k.
func ClampMCPTopK(topK *int) int {
	if topK == nil {
		return DefaultMCPTopK
	}
	value := *topK
	if value < 1 {
		return DefaultMCPTopK
	}
	if value > MaxMCPTopK {
		return MaxMCPTopK
	}
	return value
}

func chunkToResponseDict(chunk types.Chunk, repoRoot *string) map[string]any {
	dict := types.ChunkToDict(chunk)
	if repoRoot == nil || *repoRoot == "" {
		return dict
	}
	root, _ := filepath.Abs(*repoRoot)
	absolutePath := filepath.Join(root, chunk.FilePath)
	dict["absolute_path"] = absolutePath
	dict["location"] = absolutePath + ":" + itoa(chunk.StartLine) + "-" + itoa(chunk.EndLine)
	return dict
}

// DedupeResultsByFile keeps the best-scoring chunk per file.
func DedupeResultsByFile(results []types.SearchResult) []types.SearchResult {
	best := map[string]types.SearchResult{}
	for _, result := range results {
		fp := result.Chunk.FilePath
		existing, ok := best[fp]
		if !ok || result.Score > existing.Score {
			best[fp] = result
		}
	}
	out := make([]types.SearchResult, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// ChunksForFile returns sorted chunks for a file.
func ChunksForFile(chunks []types.Chunk, filePath string, repoRoot *string) []types.Chunk {
	indexedPath := ToIndexedFilePath(filePath, repoRoot)
	out := make([]types.Chunk, 0)
	for _, chunk := range chunks {
		if chunk.FilePath == indexedPath {
			out = append(out, chunk)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartLine < out[j].StartLine })
	return out
}

// ExpandChunksAtLine returns adjacent indexed chunks around a line.
func ExpandChunksAtLine(chunks []types.Chunk, filePath string, line int, repoRoot *string, before, after int) (anchor *types.Chunk, expanded []types.Chunk) {
	fileChunks := ChunksForFile(chunks, filePath, repoRoot)
	if len(fileChunks) == 0 {
		return nil, nil
	}
	anchor = ResolveChunk(chunks, filePath, line, repoRoot)
	if anchor == nil {
		return nil, nil
	}
	anchorIndex := -1
	for i, chunk := range fileChunks {
		if chunk.StartLine == anchor.StartLine && chunk.EndLine == anchor.EndLine {
			anchorIndex = i
			break
		}
	}
	if anchorIndex < 0 {
		return anchor, []types.Chunk{*anchor}
	}
	start := anchorIndex - before
	if start < 0 {
		start = 0
	}
	end := anchorIndex + after + 1
	if end > len(fileChunks) {
		end = len(fileChunks)
	}
	return anchor, fileChunks[start:end]
}

// FormatRelevanceScore formats relative % relevance.
func FormatRelevanceScore(score, maxScore float64) string {
	if maxScore <= 0 {
		return "0%"
	}
	pct := int((score/maxScore)*100 + 0.5)
	return itoa(pct) + "%"
}

// FormatResultsOptions configures FormatResults.
type FormatResultsOptions struct {
	RepoRoot *string
	Snippet  *bool
}

// FormatResults builds MCP/CLI search JSON.
func FormatResults(query string, results []types.SearchResult, options *FormatResultsOptions) map[string]any {
	var repoRoot *string
	useSnippet := snippet.SearchSnippetsEnabled()
	if options != nil {
		repoRoot = options.RepoRoot
		if options.Snippet != nil {
			useSnippet = *options.Snippet
		}
	}
	var payload []snippet.AppliedSnippet
	if useSnippet {
		payload = snippet.ApplySnippetsToResults(results, query, nil)
	}
	maxScore := 0.0
	for _, r := range results {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}
	outResults := make([]map[string]any, len(results))
	for i, result := range results {
		chunk := result.Chunk
		var meta *snippet.SnippetMeta
		if payload != nil {
			chunk = payload[i].Result.Chunk
			meta = &payload[i].Meta
		}
		dict := chunkToResponseDict(chunk, repoRoot)
		score := FormatRelevanceScore(result.Score, maxScore)
		if meta != nil && meta.Truncated {
			dict["truncated"] = true
			dict["anchor_line"] = meta.AnchorLine
			dict["full_start_line"] = meta.FullStartLine
			dict["full_end_line"] = meta.FullEndLine
		}
		outResults[i] = map[string]any{"chunk": dict, "score": score}
	}
	return map[string]any{"query": query, "results": outResults}
}

// FormatExpandOptions configures FormatExpandResults.
type FormatExpandOptions struct {
	RepoRoot *string
	Before   *int
	After    *int
}

// FormatExpandResults builds expand payload.
func FormatExpandResults(filePath string, line int, anchor *types.Chunk, expanded []types.Chunk, options *FormatExpandOptions) ExpandResults {
	var repoRoot *string
	if options != nil {
		repoRoot = options.RepoRoot
	}
	indexedPath := ToIndexedFilePath(filePath, repoRoot)
	if indexedPath == "" {
		indexedPath = filePath
	}
	var anchorDict map[string]any
	if anchor != nil {
		anchorDict = chunkToResponseDict(*anchor, repoRoot)
	}
	chunkDicts := make([]map[string]any, len(expanded))
	for i, c := range expanded {
		chunkDicts[i] = chunkToResponseDict(c, repoRoot)
	}
	return ExpandResults{
		FilePath:   indexedPath,
		Line:       line,
		ChunkCount: len(expanded),
		Anchor:     anchorDict,
		Chunks:     chunkDicts,
	}
}

// ResolveContent parses content type flags.
func ResolveContent(raw []string) []types.ContentType {
	if len(raw) == 0 {
		return types.DefaultContentTypesCopy()
	}
	for _, item := range raw {
		if item == "all" {
			return []types.ContentType{types.ContentCode, types.ContentDocs, types.ContentConfig}
		}
	}
	valid := map[string]types.ContentType{
		"code": types.ContentCode, "docs": types.ContentDocs, "config": types.ContentConfig,
	}
	out := make([]types.ContentType, 0, len(raw))
	for _, item := range raw {
		if ct, ok := valid[item]; ok {
			out = append(out, ct)
		}
	}
	return out
}

// ComputeSourceCacheKey builds the cache key for a source.
func ComputeSourceCacheKey(source string, ref *string) string {
	if IsGitURL(source) {
		if ref != nil && *ref != "" {
			return source + "@" + *ref
		}
		return source
	}
	resolved, err := filepath.Abs(source)
	if err != nil {
		return source
	}
	return resolved
}

// ResolveSearchPath resolves local paths; leaves git URLs unchanged.
func ResolveSearchPath(path string) string {
	if IsGitURL(path) {
		return path
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return resolved
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
