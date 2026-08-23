package tokencount

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "embed"

	"github.com/takara-ai/miru-code/internal/tokenizer"
)

const tokenizerJSONEnv = "MIRU_TOKENIZER_JSON"

// Default logical path reported when using the embedded tokenizer.json.
const defaultTokenizerJSONPath = "assets/tokenizer/tokenizer.json"

//go:embed tokenizer.json
var embeddedTokenizerJSON []byte

var (
	mu         sync.Mutex
	cached     tokenizer.BertWordPieceTokenizer
	cachedPath string
)

func resolveTokenizerJSONPath() string {
	fromEnv := strings.TrimSpace(os.Getenv(tokenizerJSONEnv))
	if fromEnv != "" {
		if abs, err := filepath.Abs(fromEnv); err == nil {
			return abs
		}
		return fromEnv
	}
	return defaultTokenizerJSONPath
}

func loadTokenizer() (tokenizer.BertWordPieceTokenizer, error) {
	mu.Lock()
	defer mu.Unlock()

	if cached != nil {
		return cached, nil
	}

	path := resolveTokenizerJSONPath()
	fromEnv := strings.TrimSpace(os.Getenv(tokenizerJSONEnv)) != ""

	var (
		tok tokenizer.BertWordPieceTokenizer
		err error
	)

	if fromEnv {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf(
				"tokenizer not found at %s. Set %s or add tokenizer/tokenizer.json to the package",
				path, tokenizerJSONEnv,
			)
		}
		tok, err = tokenizer.LoadFromFile(path)
		if err != nil {
			return nil, err
		}
		cached = tok
		cachedPath = path
		return cached, nil
	}

	// Prefer on-disk assets path when present (dev checkout); otherwise embedded bytes.
	if _, statErr := os.Stat(path); statErr == nil {
		tok, err = tokenizer.LoadFromFile(path)
		if err != nil {
			return nil, err
		}
		cached = tok
		cachedPath = path
		return cached, nil
	}

	tok, err = tokenizer.LoadFromJSON(embeddedTokenizerJSON)
	if err != nil {
		return nil, err
	}
	cached = tok
	cachedPath = defaultTokenizerJSONPath
	return cached, nil
}

func mustLoad() tokenizer.BertWordPieceTokenizer {
	tok, err := loadTokenizer()
	if err != nil {
		panic(err)
	}
	return tok
}

// ResetCache clears the cached tokenizer (for tests).
func ResetCache() {
	mu.Lock()
	defer mu.Unlock()
	cached = nil
	cachedPath = ""
}

// TokenizerJSONPath returns the path used for the loaded tokenizer.
func TokenizerJSONPath() string {
	mustLoad()
	mu.Lock()
	defer mu.Unlock()
	if cachedPath != "" {
		return cachedPath
	}
	return resolveTokenizerJSONPath()
}

// TokenCountMethod returns the counting method identifier.
func TokenCountMethod() string {
	return "wordpiece"
}

// CountTokens returns the WordPiece token count for text.
func CountTokens(text string) int {
	return mustLoad().Count(text)
}
