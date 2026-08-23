package chunk

import "github.com/takara-ai/miru-code/internal/types"

// DesiredChunkLengthChars matches hybrid-search target chunk length.
const DesiredChunkLengthChars = 1500

// ChunkSource cascades AST → structural → lines and materializes types.Chunk values.
func ChunkSource(source, filePath string, language *string) []types.Chunk {
	if trimSpaceCheck(source) == "" {
		return nil
	}

	boundaries := ChunkAst(source, filePath, language, DesiredChunkLengthChars)
	if boundaries == nil {
		boundaries = ChunkStructural(source, language, DesiredChunkLengthChars)
	}
	if boundaries == nil {
		boundaries = ChunkLines(source, DesiredChunkLengthChars)
	}

	prefixNewlineCounts := make([]uint32, len(source)+1)
	for i := 0; i < len(source); i++ {
		prev := prefixNewlineCounts[i]
		if source[i] == '\n' {
			prefixNewlineCounts[i+1] = prev + 1
		} else {
			prefixNewlineCounts[i+1] = prev
		}
	}

	chunks := make([]types.Chunk, 0, len(boundaries))
	for _, boundary := range boundaries {
		endIndex := boundary.End - 1
		if endIndex < boundary.Start {
			endIndex = boundary.Start
		}
		if endIndex >= len(source) {
			endIndex = len(source) - 1
		}
		if boundary.Start >= len(source) {
			continue
		}
		text := source[boundary.Start : endIndex+1]
		chunks = append(chunks, types.Chunk{
			Content:   text,
			FilePath:  filePath,
			StartLine: int(prefixNewlineCounts[boundary.Start]) + 1,
			EndLine:   int(prefixNewlineCounts[endIndex]) + 1,
			Language:  language,
		})
	}
	return chunks
}

// ChunkFile detects language from the path then chunks.
func ChunkFile(source, filePath string) []types.Chunk {
	return ChunkSource(source, filePath, DetectLanguage(filePath))
}
