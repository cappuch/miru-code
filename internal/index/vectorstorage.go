package index

import "os"

// SemanticStorage is int8 or float32.
type SemanticStorage string

const (
	StorageInt8    SemanticStorage = "int8"
	StorageFloat32 SemanticStorage = "float32"
)

// SemanticIndex is the dense retrieval interface.
type SemanticIndex interface {
	Size() int
	Dimensions() int
	MemoryBytes() int
	Storage() SemanticStorage
	VectorAt(docIndex int) ([]float32, error)
	Query(queryVector []float32, k int, selector []int) (QueryResult, error)
	Save(dir string) error
}

// ResolveSemanticStorage matches vector-storage.ts.
func ResolveSemanticStorage() SemanticStorage {
	if os.Getenv("MIRU_FLOAT_VECTORS") == "1" {
		return StorageFloat32
	}
	return StorageInt8
}

// SemanticStorageFromMetadata reads vector_storage from metadata.
func SemanticStorageFromMetadata(metadata map[string]any) SemanticStorage {
	if metadata["vector_storage"] == "float32" {
		return StorageFloat32
	}
	return StorageInt8
}

// BuildSemanticIndex builds int8 or float32 based on env.
func BuildSemanticIndex(vectors [][]float32) SemanticIndex {
	if ResolveSemanticStorage() == StorageInt8 {
		return NewQuantizedVectorIndex(vectors)
	}
	return NewVectorIndex(vectors)
}
