package tokenizer

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// TokenizerJson mirrors HuggingFace tokenizer.json fields used by BERT WordPiece.
type TokenizerJson struct {
	Normalizer *NormalizerConfig `json:"normalizer"`
	Model      ModelConfig       `json:"model"`
}

// NormalizerConfig is the BertNormalizer section.
type NormalizerConfig struct {
	Type               string `json:"type"`
	CleanText          *bool  `json:"clean_text"`
	HandleChineseChars *bool  `json:"handle_chinese_chars"`
	StripAccents       *bool  `json:"strip_accents"`
	Lowercase          *bool  `json:"lowercase"`
}

// ModelConfig is the WordPiece model section.
type ModelConfig struct {
	Type                     string         `json:"type"`
	Vocab                    map[string]int `json:"vocab"`
	UnkToken                 string         `json:"unk_token"`
	ContinuingSubwordPrefix  string         `json:"continuing_subword_prefix"`
	MaxInputCharsPerWord     int            `json:"max_input_chars_per_word"`
}

// BertWordPieceTokenizer encodes text with BERT WordPiece rules.
type BertWordPieceTokenizer interface {
	Encode(text string) []string
	Count(text string) int
}

type bertWordPieceTokenizer struct {
	vocab                map[string]struct{}
	unkToken             string
	prefix               string
	maxInputCharsPerWord int
	clean                bool
	handleChineseChars   bool
	lowercase            bool
	stripAccents         bool
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// CreateBertWordPieceTokenizer builds a tokenizer from parsed tokenizer.json.
func CreateBertWordPieceTokenizer(json TokenizerJson) BertWordPieceTokenizer {
	normalizer := json.Normalizer
	if normalizer == nil {
		normalizer = &NormalizerConfig{}
	}
	model := json.Model
	lowercase := boolOr(normalizer.Lowercase, true)
	stripAccents := lowercase
	if normalizer.StripAccents != nil {
		stripAccents = *normalizer.StripAccents
	}
	vocab := make(map[string]struct{}, len(model.Vocab))
	for k := range model.Vocab {
		vocab[k] = struct{}{}
	}
	unkToken := model.UnkToken
	if unkToken == "" {
		unkToken = "[UNK]"
	}
	prefix := model.ContinuingSubwordPrefix
	if prefix == "" {
		prefix = "##"
	}
	maxInputCharsPerWord := model.MaxInputCharsPerWord
	if maxInputCharsPerWord == 0 {
		maxInputCharsPerWord = 100
	}

	return &bertWordPieceTokenizer{
		vocab:                vocab,
		unkToken:             unkToken,
		prefix:               prefix,
		maxInputCharsPerWord: maxInputCharsPerWord,
		clean:                boolOr(normalizer.CleanText, true),
		handleChineseChars:   boolOr(normalizer.HandleChineseChars, true),
		lowercase:            lowercase,
		stripAccents:         stripAccents,
	}
}

func (t *bertWordPieceTokenizer) Encode(text string) []string {
	normalized := text
	if t.clean {
		normalized = cleanText(normalized)
	}
	if t.handleChineseChars {
		normalized = tokenizeChineseChars(normalized)
	}
	if t.lowercase {
		normalized = strings.ToLower(normalized)
		if t.stripAccents {
			normalized = removeAccents(normalized)
		}
	}

	var tokens []string
	for _, basic := range bertPreTokenize(normalized) {
		tokens = append(tokens, wordpieceTokenize(basic, t.vocab, t.unkToken, t.prefix, t.maxInputCharsPerWord)...)
	}
	return tokens
}

func (t *bertWordPieceTokenizer) Count(text string) int {
	return len(t.Encode(text))
}

func isControl(r rune) bool {
	// TAB/LF/CR are whitespace in Bert clean_text, not stripped controls.
	if r == 0x09 || r == 0x0a || r == 0x0d {
		return false
	}
	return r == 0 || r == 0xfffd || (r >= 0x00 && r <= 0x1f) || (r >= 0x7f && r <= 0x9f)
}

func isWhitespace(r rune) bool {
	return r == 0x20 || r == 0x09 || r == 0x0a || r == 0x0d
}

func isPunctuation(r rune) bool {
	cp := int(r)
	if (cp >= 33 && cp <= 47) ||
		(cp >= 58 && cp <= 64) ||
		(cp >= 91 && cp <= 96) ||
		(cp >= 123 && cp <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

func isChineseChar(cp rune) bool {
	return (cp >= 0x4e00 && cp <= 0x9fff) ||
		(cp >= 0x3400 && cp <= 0x4dbf) ||
		(cp >= 0x20000 && cp <= 0x2a6df) ||
		(cp >= 0x2a700 && cp <= 0x2b73f) ||
		(cp >= 0x2b740 && cp <= 0x2b81f) ||
		(cp >= 0x2b820 && cp <= 0x2ceaf) ||
		(cp >= 0xf900 && cp <= 0xfaff) ||
		(cp >= 0x2f800 && cp <= 0x2fa1f)
}

func cleanText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if isControl(r) {
			continue
		}
		if isWhitespace(r) {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func tokenizeChineseChars(text string) string {
	var b strings.Builder
	b.Grow(len(text) + 8)
	for _, r := range text {
		if isChineseChar(r) {
			b.WriteByte(' ')
			b.WriteRune(r)
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func removeAccents(text string) string {
	decomposed := norm.NFD.String(text)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mark, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func whitespaceTokenize(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	return strings.Fields(trimmed)
}

func splitOnPunctuation(text string) []string {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var output [][]rune
	startNewWord := true

	for _, r := range runes {
		if isPunctuation(r) {
			output = append(output, []rune{r})
			startNewWord = true
			continue
		}
		if startNewWord {
			output = append(output, nil)
			startNewWord = false
		}
		output[len(output)-1] = append(output[len(output)-1], r)
	}

	out := make([]string, 0, len(output))
	for _, parts := range output {
		if len(parts) == 0 {
			continue
		}
		out = append(out, string(parts))
	}
	return out
}

func bertPreTokenize(text string) []string {
	words := whitespaceTokenize(text)
	var splitTokens []string
	for _, word := range words {
		splitTokens = append(splitTokens, splitOnPunctuation(word)...)
	}
	return whitespaceTokenize(strings.Join(splitTokens, " "))
}

func wordpieceTokenize(
	token string,
	vocab map[string]struct{},
	unkToken string,
	prefix string,
	maxInputCharsPerWord int,
) []string {
	if token == "" {
		return nil
	}
	chars := []rune(token)
	if len(chars) > maxInputCharsPerWord {
		return []string{unkToken}
	}

	var subTokens []string
	start := 0

	for start < len(chars) {
		end := len(chars)
		var curSubstr string
		found := false

		for start < end {
			substr := string(chars[start:end])
			if start > 0 {
				substr = prefix + substr
			}
			if _, ok := vocab[substr]; ok {
				curSubstr = substr
				found = true
				break
			}
			end--
		}

		if !found {
			return []string{unkToken}
		}

		subTokens = append(subTokens, curSubstr)
		start = end
	}

	return subTokens
}
