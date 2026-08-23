package index

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// VectorIndex is uncompressed float32 semantic index.
type VectorIndex struct {
	data  []float32
	count int
	dim   int
}

// NewVectorIndex builds from float32 vectors.
func NewVectorIndex(vectors [][]float32) *VectorIndex {
	if len(vectors) == 0 {
		return &VectorIndex{}
	}
	dim := len(vectors[0])
	data := make([]float32, len(vectors)*dim)
	for i, vec := range vectors {
		copy(data[i*dim:], vec)
	}
	return &VectorIndex{data: data, count: len(vectors), dim: dim}
}

// VectorFromFlatBuffer restores from a flat buffer.
func VectorFromFlatBuffer(data []float32, count, dim int) *VectorIndex {
	return &VectorIndex{data: data, count: count, dim: dim}
}

func (v *VectorIndex) Size() int                     { return v.count }
func (v *VectorIndex) Dimensions() int               { return v.dim }
func (v *VectorIndex) MemoryBytes() int              { return v.count * v.dim * 4 }
func (v *VectorIndex) Storage() SemanticStorage      { return StorageFloat32 }

func (v *VectorIndex) VectorAt(docIndex int) ([]float32, error) {
	if docIndex < 0 || docIndex >= v.count {
		return nil, fmt.Errorf("Missing vector at index %d", docIndex)
	}
	out := make([]float32, v.dim)
	copy(out, v.data[docIndex*v.dim:(docIndex+1)*v.dim])
	return out, nil
}

func cosineDistanceFlat(data []float32, dim, docIndex int, query []float32) float64 {
	offset := docIndex * dim
	var dot float64
	for i := 0; i < dim; i++ {
		dot += float64(query[i]) * float64(data[offset+i])
	}
	return 1 - dot
}

// Query returns top-k by cosine distance.
func (v *VectorIndex) Query(queryVector []float32, k int, selector []int) (QueryResult, error) {
	if k < 1 {
		return QueryResult{}, fmt.Errorf("k should be >= 1, is now %d", k)
	}
	if v.count == 0 {
		return QueryResult{}, nil
	}
	maxN := v.count
	if selector != nil {
		maxN = len(selector)
	}
	effectiveK := k
	if effectiveK > maxN {
		effectiveK = maxN
	}
	if effectiveK == 0 {
		return QueryResult{}, nil
	}
	collector := NewTopKDistanceCollector(effectiveK)
	if selector != nil {
		for _, idx := range selector {
			if idx < 0 || idx >= v.count {
				continue
			}
			collector.Offer(idx, cosineDistanceFlat(v.data, v.dim, idx, queryVector))
		}
	} else {
		for i := 0; i < v.count; i++ {
			collector.Offer(i, cosineDistanceFlat(v.data, v.dim, i, queryVector))
		}
	}
	top := collector.Finish()
	res := QueryResult{Indices: make([]int, len(top)), Distances: make([]float64, len(top))}
	for i, e := range top {
		res.Indices[i] = e.Index
		res.Distances[i] = e.Distance
	}
	return res, nil
}

// Save writes vectors.bin + meta.json.
func (v *VectorIndex) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	buf := make([]byte, len(v.data)*4)
	for i, f := range v.data {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	if err := os.WriteFile(filepath.Join(dir, "vectors.bin"), buf, 0o644); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"count": v.count, "dimensions": v.dim, "storage": "float32"})
	return os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644)
}

// LoadVector loads float32 index from disk.
func LoadVector(dir string) (*VectorIndex, error) {
	metaBytes, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Count      int `json:"count"`
		Dimensions int `json:"dimensions"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "vectors.bin"))
	if err != nil {
		return nil, err
	}
	data := make([]float32, len(raw)/4)
	for i := range data {
		data[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return VectorFromFlatBuffer(data, meta.Count, meta.Dimensions), nil
}
