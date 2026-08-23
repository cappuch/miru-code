package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadFromFile reads a HuggingFace tokenizer.json and builds a WordPiece tokenizer.
func LoadFromFile(path string) (BertWordPieceTokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadFromJSON(data)
}

// LoadFromJSON parses tokenizer.json bytes and builds a WordPiece tokenizer.
func LoadFromJSON(data []byte) (BertWordPieceTokenizer, error) {
	var tj TokenizerJson
	if err := json.Unmarshal(data, &tj); err != nil {
		return nil, err
	}
	if tj.Model.Type != "WordPiece" {
		typ := tj.Model.Type
		if typ == "" {
			typ = "unknown"
		}
		return nil, fmt.Errorf("unsupported tokenizer model type: %s", typ)
	}
	return CreateBertWordPieceTokenizer(tj), nil
}
