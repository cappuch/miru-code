package miru_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	miru "github.com/takara-ai/miru-code"
)

// Exercise the public import and signatures used by embedding applications.
func TestFromPathAndSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode embedding request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.Model != "test-model" {
			t.Errorf("embedding model = %q, want test-model", request.Model)
		}
		data := make([]map[string]any, len(request.Input))
		for i := range data {
			data[i] = map[string]any{"index": i, "embedding": []float64{1, 0, 0}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
			t.Errorf("encode embedding response: %v", err)
		}
	}))
	defer server.Close()
	t.Setenv("TAKARA_API_KEY", "test-key")
	t.Setenv("MIRU_OPENAI_BASE_URL", server.URL)
	t.Setenv("MIRU_EMBEDDING_DIMENSIONS", "3")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\n\nfunc HelloMiru() string { return \"hello\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := miru.FromPath(root, []miru.ContentType{miru.ContentCode}, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	rerank := false
	results, err := idx.Search("HelloMiru", 1, nil, []string{"go"}, []string{"sample.go"}, &rerank)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Chunk.FilePath != "sample.go" {
		t.Fatalf("search results = %+v, want sample.go", results)
	}
}

func TestFromPathInvalid(t *testing.T) {
	idx, err := miru.FromPath(filepath.Join(t.TempDir(), "missing"), nil)
	if err == nil || idx != nil {
		t.Fatalf("FromPath missing path = %v, %v; want nil index and error", idx, err)
	}
}
