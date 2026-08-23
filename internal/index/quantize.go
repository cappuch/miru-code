package index

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// QuantizeVector performs symmetric int8 quantization (matches quantize.ts).
func QuantizeVector(vector []float32) QuantizedVector {
	maxAbs := float32(0)
	for _, v := range vector {
		a := v
		if a < 0 {
			a = -a
		}
		if a > maxAbs {
			maxAbs = a
		}
	}
	scale := float64(1)
	if maxAbs > 0 {
		scale = float64(maxAbs) / 127
	}
	codes := make([]int8, len(vector))
	if maxAbs > 0 {
		inv := float32(127) / maxAbs
		for i, v := range vector {
			r := math.Round(float64(v * inv))
			if r > 127 {
				r = 127
			}
			if r < -127 {
				r = -127
			}
			codes[i] = int8(r)
		}
	}
	return QuantizedVector{Codes: codes, Scale: scale}
}

// QuantizedVectorIndex is the production semantic index.
type QuantizedVectorIndex struct {
	codes  []int8
	scales []float32
	count  int
	dim    int
}

// NewQuantizedVectorIndex builds from float32 vectors.
func NewQuantizedVectorIndex(vectors [][]float32) *QuantizedVectorIndex {
	if len(vectors) == 0 {
		return &QuantizedVectorIndex{}
	}
	dim := len(vectors[0])
	codes := make([]int8, len(vectors)*dim)
	scales := make([]float32, len(vectors))
	for i, vector := range vectors {
		qv := QuantizeVector(vector)
		copy(codes[i*dim:], qv.Codes)
		scales[i] = float32(qv.Scale)
	}
	return &QuantizedVectorIndex{
		codes:  codes,
		scales: scales,
		count:  len(vectors),
		dim:    dim,
	}
}

// QuantizedFromPersisted restores an index from raw buffers.
func QuantizedFromPersisted(codes []int8, scales []float32, count, dim int) *QuantizedVectorIndex {
	return &QuantizedVectorIndex{codes: codes, scales: scales, count: count, dim: dim}
}

func (q *QuantizedVectorIndex) Size() int        { return q.count }
func (q *QuantizedVectorIndex) Dimensions() int  { return q.dim }
func (q *QuantizedVectorIndex) MemoryBytes() int { return q.count*q.dim + len(q.scales)*4 }
func (q *QuantizedVectorIndex) Storage() SemanticStorage { return StorageInt8 }

// VectorAt dequantizes and re-normalizes.
func (q *QuantizedVectorIndex) VectorAt(docIndex int) ([]float32, error) {
	if docIndex < 0 || docIndex >= q.count {
		return nil, fmt.Errorf("Missing quantized vector at index %d", docIndex)
	}
	offset := docIndex * q.dim
	scale := q.scales[docIndex]
	out := make([]float32, q.dim)
	for i := 0; i < q.dim; i++ {
		out[i] = float32(q.codes[offset+i]) * scale
	}
	var norm float64
	for _, v := range out {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range out {
			out[i] = float32(float64(out[i]) / norm)
		}
	}
	return out, nil
}

// QueryResult holds indices and distances.
type QueryResult struct {
	Indices   []int
	Distances []float64
}

// Query returns top-k by cosine distance (1 - int8 similarity).
func (q *QuantizedVectorIndex) Query(queryVector []float32, k int, selector []int) (QueryResult, error) {
	if k < 1 {
		return QueryResult{}, fmt.Errorf("k should be >= 1, is now %d", k)
	}
	if q.count == 0 {
		return QueryResult{}, nil
	}
	qv := QuantizeVector(queryVector)
	maxN := q.count
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
			if idx < 0 || idx >= q.count {
				continue
			}
			sim := QuantizedDotFlat(qv, q.codes, idx*q.dim, q.dim, float64(q.scales[idx]))
			collector.Offer(idx, 1-sim)
		}
	} else {
		for i := 0; i < q.count; i++ {
			sim := QuantizedDotFlat(qv, q.codes, i*q.dim, q.dim, float64(q.scales[i]))
			collector.Offer(i, 1-sim)
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

// Save writes codes.bin, scales.bin, meta.json.
func (q *QuantizedVectorIndex) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	codeBytes := make([]byte, len(q.codes))
	for i, c := range q.codes {
		codeBytes[i] = byte(c)
	}
	if err := os.WriteFile(filepath.Join(dir, "codes.bin"), codeBytes, 0o644); err != nil {
		return err
	}
	scaleBytes := make([]byte, len(q.scales)*4)
	for i, s := range q.scales {
		binary.LittleEndian.PutUint32(scaleBytes[i*4:], math.Float32bits(s))
	}
	if err := os.WriteFile(filepath.Join(dir, "scales.bin"), scaleBytes, 0o644); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"count": q.count, "dimensions": q.dim, "storage": "int8"})
	return os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644)
}

// LoadQuantized loads from disk.
func LoadQuantized(dir string) (*QuantizedVectorIndex, error) {
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
	codeBytes, err := os.ReadFile(filepath.Join(dir, "codes.bin"))
	if err != nil {
		return nil, err
	}
	codes := make([]int8, len(codeBytes))
	for i, b := range codeBytes {
		codes[i] = int8(b)
	}
	scaleBytes, err := os.ReadFile(filepath.Join(dir, "scales.bin"))
	if err != nil {
		return nil, err
	}
	scales := make([]float32, len(scaleBytes)/4)
	for i := range scales {
		scales[i] = math.Float32frombits(binary.LittleEndian.Uint32(scaleBytes[i*4:]))
	}
	return QuantizedFromPersisted(codes, scales, meta.Count, meta.Dimensions), nil
}
