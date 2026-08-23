package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PersistencePaths mirrors persistence.ts.
type PersistencePaths struct {
	Root          string
	Bm25Index     string
	SemanticIndex string
	Chunks        string
	Metadata      string
}

// NewPersistencePaths builds paths under root.
func NewPersistencePaths(root string) PersistencePaths {
	return PersistencePaths{
		Root:          root,
		Bm25Index:     filepath.Join(root, "bm25_index.json"),
		SemanticIndex: filepath.Join(root, "semantic_index"),
		Chunks:        filepath.Join(root, "chunks.json"),
		Metadata:      filepath.Join(root, "metadata.json"),
	}
}

// PathsExist returns names of missing required pieces.
func PathsExist(paths PersistencePaths) []string {
	var missing []string
	checks := []struct {
		name string
		path string
	}{
		{"bm25", paths.Bm25Index},
		{"semantic", filepath.Join(paths.SemanticIndex, "meta.json")},
		{"chunks", paths.Chunks},
		{"metadata", paths.Metadata},
	}
	for _, c := range checks {
		if _, err := os.Stat(c.path); err != nil {
			missing = append(missing, c.name)
		}
	}
	return missing
}

// SaveSemantic saves a semantic index.
func SaveSemantic(si SemanticIndex, dir string) error {
	return si.Save(dir)
}

// LoadSemantic loads int8 or float32 from meta.
func LoadSemantic(dir string) (SemanticIndex, error) {
	metaBytes, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Storage string `json:"storage"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}
	if meta.Storage == "int8" {
		return LoadQuantized(dir)
	}
	return LoadVector(dir)
}

// SaveIndexBundle writes the full on-disk bundle.
func SaveIndexBundle(paths PersistencePaths, bm25 *BM25Index, semantic SemanticIndex, chunks any, metadata map[string]any) error {
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return err
	}
	if err := SaveBM25(bm25, paths.Bm25Index); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.SemanticIndex, 0o755); err != nil {
		return err
	}
	if err := SaveSemantic(semantic, paths.SemanticIndex); err != nil {
		return err
	}
	chunkBytes, err := json.Marshal(chunks)
	if err != nil {
		return err
	}
	if err := os.WriteFile(paths.Chunks, chunkBytes, 0o644); err != nil {
		return err
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.Metadata, metaBytes, 0o644)
}

// LoadIndexBundle loads BM25, semantic, chunks JSON, metadata.
func LoadIndexBundle(paths PersistencePaths) (*BM25Index, SemanticIndex, []map[string]any, map[string]any, error) {
	missing := PathsExist(paths)
	if len(missing) > 0 {
		return nil, nil, nil, nil, fmt.Errorf("missing index pieces: %v", missing)
	}
	bm25, err := LoadBM25(paths.Bm25Index)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	semantic, err := LoadSemantic(paths.SemanticIndex)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	chunkBytes, err := os.ReadFile(paths.Chunks)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var chunks []map[string]any
	if err := json.Unmarshal(chunkBytes, &chunks); err != nil {
		return nil, nil, nil, nil, err
	}
	metaBytes, err := os.ReadFile(paths.Metadata)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var metadata map[string]any
	if err := json.Unmarshal(metaBytes, &metadata); err != nil {
		return nil, nil, nil, nil, err
	}
	return bm25, semantic, chunks, metadata, nil
}
