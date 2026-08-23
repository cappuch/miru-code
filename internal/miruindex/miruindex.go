package miruindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/takara-ai/miru-code/internal/cache"
	"github.com/takara-ai/miru-code/internal/chunk"
	"github.com/takara-ai/miru-code/internal/embed"
	gitclone "github.com/takara-ai/miru-code/internal/git"
	"github.com/takara-ai/miru-code/internal/index"
	"github.com/takara-ai/miru-code/internal/literal"
	"github.com/takara-ai/miru-code/internal/search"
	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/utils"
	"github.com/takara-ai/miru-code/internal/version"
)

// MiruIndex is an in-memory hybrid search index over a codebase.
type MiruIndex struct {
	Embeddings     embed.EmbeddingBackend
	chunks         []types.Chunk
	bm25Index      *index.BM25Index
	semanticIndex  index.SemanticIndex
	loadedFromDisk bool
	EmbeddingModel string
	root           *string
	content        []types.ContentType
	fileMapping    map[string][]int
	languageMapping map[string][]int
	storedFileMtimes map[string]float64
}

// Options for constructing MiruIndex.
type Options struct {
	Embeddings       embed.EmbeddingBackend
	BM25Index        *index.BM25Index
	SemanticIndex    index.SemanticIndex
	Chunks           []types.Chunk
	EmbeddingModel   string
	Root             *string
	Content          []types.ContentType
	LoadedFromDisk   bool
	StoredFileMtimes map[string]float64
}

// New constructs a MiruIndex.
func New(opts Options) *MiruIndex {
	content := opts.Content
	if len(content) == 0 {
		content = types.DefaultContentTypesCopy()
	}
	m := &MiruIndex{
		Embeddings:       opts.Embeddings,
		chunks:           opts.Chunks,
		bm25Index:        opts.BM25Index,
		semanticIndex:    opts.SemanticIndex,
		loadedFromDisk:   opts.LoadedFromDisk,
		EmbeddingModel:   opts.EmbeddingModel,
		root:             opts.Root,
		content:          content,
		storedFileMtimes: opts.StoredFileMtimes,
	}
	if m.storedFileMtimes == nil {
		m.storedFileMtimes = map[string]float64{}
	}
	m.rebuildMappings()
	return m
}

// Chunks returns indexed chunks.
func (m *MiruIndex) Chunks() []types.Chunk { return m.chunks }

// LoadedFromDisk reports whether hydrated from cache.
func (m *MiruIndex) LoadedFromDisk() bool { return m.loadedFromDisk }

// Root returns the local root path if any.
func (m *MiruIndex) Root() *string { return m.root }

// ContentTypes returns indexed content kinds.
func (m *MiruIndex) ContentTypes() []types.ContentType { return m.content }

func (m *MiruIndex) rebuildMappings() {
	m.fileMapping = map[string][]int{}
	m.languageMapping = map[string][]int{}
	for i, c := range m.chunks {
		m.fileMapping[c.FilePath] = append(m.fileMapping[c.FilePath], i)
		if c.Language != nil {
			lang := *c.Language
			m.languageMapping[lang] = append(m.languageMapping[lang], i)
		}
	}
}

// FromPath indexes a directory, reusing a validated cache when available.
func FromPath(path string, content []types.ContentType, embeddingModel *string) (*MiruIndex, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("Path does not exist: %s", path)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("Path is not a directory: %s", path)
	}
	if len(content) == 0 {
		content = types.DefaultContentTypesCopy()
	}
	model := embed.ResolveEmbeddingModel()
	if embeddingModel != nil && *embeddingModel != "" {
		model = *embeddingModel
	}
	cachedPath, err := cache.GetValidatedCache(resolved, &model, content)
	if err != nil {
		return nil, err
	}
	if cachedPath != "" {
		return LoadFromDisk(cachedPath, &model)
	}

	embeddings := embed.GetEmbeddingBackend(model)
	bm25, semantic, chunks, err := createIndexFromPath(resolved, embeddings, content, resolved)
	if err != nil {
		return nil, err
	}
	return New(Options{
		Embeddings:     embeddings,
		BM25Index:      bm25,
		SemanticIndex:  semantic,
		Chunks:         chunks,
		EmbeddingModel: model,
		Root:           &resolved,
		Content:        content,
	}), nil
}

// LoadFromDisk hydrates from a saved index bundle.
func LoadFromDisk(path string, embeddingModel *string) (*MiruIndex, error) {
	model := embed.ResolveEmbeddingModel()
	if embeddingModel != nil && *embeddingModel != "" {
		model = *embeddingModel
	}
	cached, err := cache.LoadCachedIndex(path)
	if err != nil {
		return nil, err
	}
	content := types.DefaultContentTypesCopy()
	if raw, ok := cached.Metadata["content_type"]; ok {
		b, _ := json.Marshal(raw)
		var strs []string
		if json.Unmarshal(b, &strs) == nil && len(strs) > 0 {
			content = nil
			for _, s := range strs {
				content = append(content, types.ContentType(s))
			}
		}
	}
	var root *string
	if rp, ok := cached.Metadata["root_path"].(string); ok && rp != "" {
		root = &rp
	}
	mtimes := map[string]float64{}
	if raw, ok := cached.Metadata["file_mtimes"].(map[string]any); ok {
		for k, v := range raw {
			switch t := v.(type) {
			case float64:
				mtimes[k] = t
			}
		}
	}
	return New(Options{
		Embeddings:       embed.GetEmbeddingBackend(model),
		BM25Index:        cached.BM25,
		SemanticIndex:    cached.Semantic,
		Chunks:           cached.Chunks,
		EmbeddingModel:   model,
		Root:             root,
		Content:          content,
		LoadedFromDisk:   true,
		StoredFileMtimes: mtimes,
	}), nil
}

// Save writes the index bundle to path.
func (m *MiruIndex) Save(path string) error {
	paths := index.NewPersistencePaths(path)
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return err
	}
	fileMtimes := map[string]float64{}
	if m.root != nil {
		for filePath := range m.fileMapping {
			abs := filepath.Join(*m.root, filePath)
			st, err := os.Stat(abs)
			if err != nil {
				fileMtimes[filePath] = 0
				continue
			}
			fileMtimes[filePath] = float64(st.ModTime().UnixMilli())
		}
	}
	filePaths := make([]string, 0, len(m.fileMapping))
	for fp := range m.fileMapping {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	chunkDicts := make([]map[string]any, len(m.chunks))
	for i, c := range m.chunks {
		chunkDicts[i] = types.ChunkToDict(c)
	}
	dims := m.Embeddings.Dimensions()
	if dims == 0 {
		if d := embed.ResolveEmbeddingDimensions(m.EmbeddingModel); d != nil {
			dims = *d
		}
	}
	metadata := map[string]any{
		"index_epoch":         version.IndexCacheEpoch(),
		"root_path":           m.root,
		"time":                float64(time.Now().UnixNano()) / 1e9,
		"embedding_model":     m.EmbeddingModel,
		"embedding_dimensions": dims,
		"embedding_provider":  "openai",
		"content_type":        m.content,
		"file_paths":          filePaths,
		"file_mtimes":         fileMtimes,
	}
	return index.SaveIndexBundle(paths, m.bm25Index, m.semanticIndex, chunkDicts, metadata)
}

// SaveToCache persists to the default cache location.
func (m *MiruIndex) SaveToCache(sourcePath string, force bool) error {
	if !m.loadedFromDisk || force {
		return m.Save(cache.FindIndexCachePath(sourcePath, nil))
	}
	return nil
}

func (m *MiruIndex) getSelector(filterLanguages, filterPaths []string) []int {
	if len(filterLanguages) == 0 && len(filterPaths) == 0 {
		return nil
	}
	selected := map[int]struct{}{}
	if len(filterLanguages) > 0 {
		for _, lang := range filterLanguages {
			for _, idx := range m.languageMapping[lang] {
				selected[idx] = struct{}{}
			}
		}
	}
	if len(filterPaths) > 0 {
		pathSet := map[int]struct{}{}
		for _, p := range filterPaths {
			p = filepath.ToSlash(p)
			for fp, idxs := range m.fileMapping {
				norm := filepath.ToSlash(fp)
				if norm == p || strings.HasPrefix(norm, strings.TrimSuffix(p, "/")+"/") {
					for _, idx := range idxs {
						pathSet[idx] = struct{}{}
					}
				}
			}
		}
		if len(filterLanguages) == 0 {
			selected = pathSet
		} else {
			intersect := map[int]struct{}{}
			for idx := range selected {
				if _, ok := pathSet[idx]; ok {
					intersect[idx] = struct{}{}
				}
			}
			selected = intersect
		}
	}
	out := make([]int, 0, len(selected))
	for idx := range selected {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

// SearchOptions configures Search.
type SearchOptions struct {
	Query           string
	TopK            int
	Alpha           *float64
	FilterLanguages []string
	FilterPaths     []string
	Rerank          *bool
}

// Search runs hybrid BM25 + semantic retrieval.
func (m *MiruIndex) Search(opts SearchOptions) ([]types.SearchResult, error) {
	if len(m.chunks) == 0 || strings.TrimSpace(opts.Query) == "" {
		return nil, nil
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	rerank := opts.Rerank
	if rerank == nil {
		includesCode := false
		for _, c := range m.content {
			if c == types.ContentCode {
				includesCode = true
				break
			}
		}
		v := includesCode
		rerank = &v
	}
	return search.HybridSearch(search.HybridOptions{
		Query:         opts.Query,
		Embeddings:    m.Embeddings,
		SemanticIndex: m.semanticIndex,
		BM25Index:     m.bm25Index,
		Chunks:        m.chunks,
		TopK:          topK,
		Alpha:         opts.Alpha,
		Selector:      m.getSelector(opts.FilterLanguages, opts.FilterPaths),
		Rerank:        rerank,
	})
}

// LocateLiteral runs exact substring location over indexed chunks.
func (m *MiruIndex) LocateLiteral(lit any, options literal.LocateOptions) literal.LocateResult {
	return literal.Locate(m.chunks, lit, options)
}

// FindRelated returns semantic neighbors of a chunk/hit.
func (m *MiruIndex) FindRelated(source any, topK int) ([]types.SearchResult, error) {
	if topK <= 0 {
		topK = 5
	}
	var target types.Chunk
	switch v := source.(type) {
	case types.Chunk:
		target = v
	case types.SearchResult:
		target = v.Chunk
	default:
		return nil, fmt.Errorf("unsupported source type")
	}
	var selector []int
	if target.Language != nil {
		selector = m.getSelector([]string{*target.Language}, nil)
	}
	results, err := search.SearchSemanticOnly(search.SemanticOnlyOptions{
		Query:         target.Content,
		Embeddings:    m.Embeddings,
		SemanticIndex: m.semanticIndex,
		Chunks:        m.chunks,
		TopK:          topK + 1,
		Selector:      selector,
	})
	if err != nil {
		return nil, err
	}
	key := types.ChunkKey(target)
	out := make([]types.SearchResult, 0, topK)
	for _, r := range results {
		if types.ChunkKey(r.Chunk) == key {
			continue
		}
		out = append(out, r)
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

// --- simplified create (walk + chunk + embed + BM25) ---

var contentExtensions = map[types.ContentType][]string{
	types.ContentCode: {
		".py", ".pyi", ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".mts", ".cts",
		".go", ".rs", ".java", ".kt", ".kts", ".c", ".h", ".cpp", ".cc", ".cxx",
		".hpp", ".hh", ".hxx", ".cs", ".rb", ".php", ".swift", ".scala", ".lua",
		".sh", ".bash", ".zsh", ".sql", ".dart", ".zig", ".vue", ".svelte",
	},
	types.ContentDocs:   {".md", ".mdx", ".rst", ".txt"},
	types.ContentConfig: {".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".xml"},
}

func extensionsFor(content []types.ContentType) map[string]struct{} {
	out := map[string]struct{}{}
	for _, ct := range content {
		for _, e := range contentExtensions[ct] {
			out[e] = struct{}{}
		}
	}
	return out
}

func createIndexFromPath(
	root string,
	embeddings embed.EmbeddingBackend,
	content []types.ContentType,
	displayRoot string,
) (*index.BM25Index, index.SemanticIndex, []types.Chunk, error) {
	exts := extensionsFor(content)
	var chunks []types.Chunk
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := exts[ext]; !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1_000_000 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		source := string(raw)
		if strings.TrimSpace(source) == "" {
			return nil
		}
		rel := path
		if displayRoot != "" {
			if r, err := filepath.Rel(displayRoot, path); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		lang := chunk.DetectLanguage(path)
		chunks = append(chunks, chunk.ChunkSource(source, rel, lang)...)
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}

	bm25 := index.BuildBm25FromChunks(chunks)
	if len(chunks) == 0 {
		return bm25, index.BuildSemanticIndex(nil), chunks, nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	vectors, err := embeddings.EmbedDocuments(texts)
	if err != nil {
		return nil, nil, nil, err
	}
	semantic := index.BuildSemanticIndex(vectors)
	return bm25, semantic, chunks, nil
}

// ApplyFileChanges incrementally rebuilds for changed relative paths.
func (m *MiruIndex) ApplyFileChanges(changedRelPaths []string) error {
	if m.root == nil {
		return fmt.Errorf("ApplyFileChanges requires a local root")
	}
	bm25, semantic, chunks, err := index.ApplyIncrementalFileChanges(
		*m.root, m.chunks, m.semanticIndex, changedRelPaths, m.Embeddings, m.content,
	)
	if err != nil {
		return err
	}
	m.bm25Index = bm25
	m.semanticIndex = semantic
	m.chunks = chunks
	m.loadedFromDisk = false
	m.rebuildMappings()
	return nil
}

// FromSource resolves a local path or clones a git URL.
func FromSource(source string, content []types.ContentType, embeddingModel *string, ref *string) (*MiruIndex, error) {
	if utils.IsGitURL(source) {
		dir, err := gitclone.Clone(source, ref)
		if err != nil {
			return nil, err
		}
		idx, err := FromPath(dir, content, embeddingModel)
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		return idx, nil
	}
	return FromPath(source, content, embeddingModel)
}
