package mcp

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/takara-ai/miru-code/internal/miruindex"
	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/utils"
)

const cacheMaxSize = 10

type cacheEntry struct {
	source string
	index  *miruindex.MiruIndex
	err    error
	ready  chan struct{}
}

// IndexCache is a session cache of MiruIndex instances.
type IndexCache struct {
	mu      sync.Mutex
	content []types.ContentType
	ref     *string
	entries map[string]*cacheEntry
	order   []string
}

// NewIndexCache creates a session index cache.
func NewIndexCache(content []types.ContentType, ref *string) *IndexCache {
	if len(content) == 0 {
		content = types.DefaultContentTypesCopy()
	}
	return &IndexCache{
		content: content,
		ref:     ref,
		entries: map[string]*cacheEntry{},
	}
}

// Get returns (building if needed) an index for source.
func (c *IndexCache) Get(source string, ref *string) (*miruindex.MiruIndex, error) {
	resolvedRef := c.ref
	if ref != nil {
		resolvedRef = ref
	}
	cacheKey := utils.ComputeSourceCacheKey(source, resolvedRef)

	c.mu.Lock()
	if entry, ok := c.entries[cacheKey]; ok {
		c.mu.Unlock()
		<-entry.ready
		return entry.index, entry.err
	}
	entry := &cacheEntry{source: source, ready: make(chan struct{})}
	c.entries[cacheKey] = entry
	c.order = append(c.order, cacheKey)
	for len(c.order) > cacheMaxSize {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	c.mu.Unlock()

	idx, err := miruindex.FromSource(source, c.content, nil, resolvedRef)
	if err == nil && !utils.IsGitURL(source) {
		resolved, _ := filepath.Abs(source)
		_ = idx.SaveToCache(resolved, false)
	}
	entry.index = idx
	entry.err = err
	close(entry.ready)
	return idx, err
}

// Close clears the cache.
func (c *IndexCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*cacheEntry{}
	c.order = nil
}

// GetIndexForRepo validates repo and returns a cached index.
func GetIndexForRepo(repo string, cache *IndexCache, ref *string) (*miruindex.MiruIndex, error) {
	if repo == "" {
		return nil, fmt.Errorf("Pass an https:// or http:// git URL or local directory path as `repo` (project root for local workspaces).")
	}
	if utils.IsGitURL(repo) && !utils.IsAllowedRepoSource(repo) {
		if len(repo) >= 7 && repo[:7] == "http://" {
			return nil, fmt.Errorf("Plain http:// git URLs are disabled by default. Set MIRU_ALLOW_HTTP_GIT=1 to opt in.")
		}
		return nil, fmt.Errorf("Only https:// git URLs or local directory paths are accepted as `repo`. Got: %s", repo)
	}
	if err := utils.ValidateLocalRepoPath(repo); err != nil {
		return nil, err
	}
	idx, err := cache.Get(repo, ref)
	if err != nil {
		return nil, fmt.Errorf("Failed to index %s: %w", repo, err)
	}
	return idx, nil
}
