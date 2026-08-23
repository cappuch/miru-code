package index

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestQuantizePersistRoundtrip(t *testing.T) {
	vectors := [][]float32{
		{0.1, -0.2, 0.3, 0.4},
		{0.5, 0.6, -0.7, 0.8},
		{-0.1, 0.2, -0.3, 0.4},
	}
	qi := NewQuantizedVectorIndex(vectors)
	dir := t.TempDir()
	if err := qi.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadQuantized(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Size() != qi.Size() || loaded.Dimensions() != qi.Dimensions() {
		t.Fatalf("size/dim mismatch %d/%d vs %d/%d", loaded.Size(), loaded.Dimensions(), qi.Size(), qi.Dimensions())
	}
	query := []float32{0.1, -0.2, 0.3, 0.4}
	orig, err := qi.Query(query, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loaded.Query(query, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(orig.Indices) != len(got.Indices) {
		t.Fatalf("indices len %v vs %v", orig.Indices, got.Indices)
	}
	for i := range orig.Indices {
		if orig.Indices[i] != got.Indices[i] {
			t.Fatalf("indices %v vs %v", orig.Indices, got.Indices)
		}
		if math.Abs(orig.Distances[i]-got.Distances[i]) > 1e-5 {
			t.Fatalf("distances %v vs %v", orig.Distances, got.Distances)
		}
	}
	// Ensure meta.json exists under the save dir.
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Fatal(err)
	}
}

func TestQuantizeVectorScale(t *testing.T) {
	qv := QuantizeVector([]float32{0, 127, -127})
	if qv.Scale <= 0 {
		t.Fatalf("scale %v", qv.Scale)
	}
	if len(qv.Codes) != 3 {
		t.Fatalf("codes %v", qv.Codes)
	}
}
