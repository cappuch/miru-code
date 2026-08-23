package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/index"
	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/utils"
	"github.com/takara-ai/miru-code/internal/version"
)

// ResolveCacheFolder returns the platform cache root (override MIRU_CACHE_HOME).
func ResolveCacheFolder() string {
	if override := stringsTrim(os.Getenv("MIRU_CACHE_HOME")); override != "" {
		return override
	}
	name := "miru"
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, name, "Cache")
	case "darwin":
		return filepath.Join(home, "Library", "Caches", name)
	default:
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			return filepath.Join(xdg, name)
		}
		return filepath.Join(home, ".cache", name)
	}
}

func stringsTrim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// FindIndexCachePath returns the on-disk index bundle path for a source.
func FindIndexCachePath(path string, ref *string) string {
	normalized := utils.ComputeSourceCacheKey(path, ref)
	sum := sha256.Sum256([]byte(normalized))
	subdir := hex.EncodeToString(sum[:])
	return filepath.Join(ResolveCacheFolder(), subdir, "index")
}

func metadataMatches(metadata map[string]any, embeddingModel string, content []types.ContentType) bool {
	defer func() { _ = recover() }()
	epoch, _ := metadata["index_epoch"].(string)
	if epoch != version.IndexCacheEpoch() {
		return false
	}
	model, _ := metadata["embedding_model"].(string)
	if model != embeddingModel {
		return false
	}
	expectedDims := embed.ResolveEmbeddingDimensions(embeddingModel)
	if expectedDims != nil {
		storedDims, ok := asInt(metadata["embedding_dimensions"])
		if !ok || storedDims != *expectedDims {
			return false
		}
	}
	if index.SemanticStorageFromMetadata(metadata) != index.ResolveSemanticStorage() {
		return false
	}
	stored, ok := metadata["content_type"].([]any)
	if !ok {
		// try []string via JSON re-marshal
		raw, err := json.Marshal(metadata["content_type"])
		if err != nil {
			return false
		}
		var strs []string
		if err := json.Unmarshal(raw, &strs); err != nil {
			return false
		}
		stored = make([]any, len(strs))
		for i, s := range strs {
			stored[i] = s
		}
	}
	a := map[string]struct{}{}
	for _, c := range stored {
		s, _ := c.(string)
		a[s] = struct{}{}
	}
	b := map[string]struct{}{}
	for _, c := range content {
		b[string(c)] = struct{}{}
	}
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		n, err := t.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

// GetValidatedCache returns a cache path when a compatible bundle exists.
func GetValidatedCache(path string, embeddingModel *string, content []types.ContentType) (string, error) {
	indexPath := FindIndexCachePath(path, nil)
	paths := index.NewPersistencePaths(indexPath)
	if missing := index.PathsExist(paths); len(missing) > 0 {
		return "", nil
	}
	model := embed.ResolveEmbeddingModel()
	if embeddingModel != nil && *embeddingModel != "" {
		model = *embeddingModel
	}
	metaBytes, err := os.ReadFile(paths.Metadata)
	if err != nil {
		return "", err
	}
	var metadata map[string]any
	if err := json.Unmarshal(metaBytes, &metadata); err != nil {
		return "", err
	}
	if !metadataMatches(metadata, model, content) {
		epoch, _ := metadata["index_epoch"].(string)
		if epoch != version.IndexCacheEpoch() {
			_ = os.RemoveAll(indexPath)
		}
		return "", nil
	}
	semMetaBytes, err := os.ReadFile(filepath.Join(paths.SemanticIndex, "meta.json"))
	if err != nil {
		return "", err
	}
	var semMeta struct {
		Dimensions int `json:"dimensions"`
	}
	if err := json.Unmarshal(semMetaBytes, &semMeta); err != nil {
		return "", err
	}
	expectedDims := embed.ResolveEmbeddingDimensions(model)
	if expectedDims != nil && semMeta.Dimensions != *expectedDims {
		return "", nil
	}
	if rootPath, ok := metadata["root_path"].(string); ok && rootPath != "" {
		if _, err := os.Stat(rootPath); err != nil {
			return "", nil
		}
		filePaths, _ := metadata["file_paths"].([]any)
		for _, rel := range filePaths {
			s, _ := rel.(string)
			if _, err := os.Stat(filepath.Join(rootPath, s)); err != nil {
				return "", nil
			}
		}
	}
	return indexPath, nil
}

// CachedIndex is a loaded cache bundle.
type CachedIndex struct {
	BM25     *index.BM25Index
	Semantic index.SemanticIndex
	Chunks   []types.Chunk
	Metadata map[string]any
}

// LoadCachedIndex loads BM25, semantic, chunks, and metadata.
func LoadCachedIndex(indexPath string) (*CachedIndex, error) {
	paths := index.NewPersistencePaths(indexPath)
	bm25, semantic, chunkData, metadata, err := index.LoadIndexBundle(paths)
	if err != nil {
		return nil, err
	}
	chunks := make([]types.Chunk, len(chunkData))
	for i, d := range chunkData {
		chunks[i] = types.ChunkFromDict(d)
	}
	return &CachedIndex{BM25: bm25, Semantic: semantic, Chunks: chunks, Metadata: metadata}, nil
}

// ClearCache removes the cached index bundle for a source.
func ClearCache(path string) error {
	indexPath := FindIndexCachePath(path, nil)
	return os.RemoveAll(indexPath)
}
