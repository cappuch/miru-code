//go:build !cgo

package chunk

// ChunkAst requests the structural/line fallback when tree-sitter is unavailable.
// This keeps pure-Go and cross-compiled consumers usable without a C toolchain.
func ChunkAst(source, filePath string, language *string, desiredLength int) []ChunkBoundary {
	return nil
}
