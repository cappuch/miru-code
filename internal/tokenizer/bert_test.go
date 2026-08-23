package tokenizer

import (
	"testing"
)

func TestCreateBertWordPieceTokenizerBasic(t *testing.T) {
	tok := CreateBertWordPieceTokenizer(TokenizerJson{
		Normalizer: &NormalizerConfig{
			CleanText:          boolPtr(true),
			HandleChineseChars: boolPtr(true),
			Lowercase:          boolPtr(true),
			StripAccents:       boolPtr(true),
		},
		Model: ModelConfig{
			Type: "WordPiece",
			Vocab: map[string]int{
				"hello": 1,
				"world": 2,
				"##ing": 3,
				"run":   4,
				"[UNK]": 0,
			},
			UnkToken:                "[UNK]",
			ContinuingSubwordPrefix: "##",
			MaxInputCharsPerWord:    100,
		},
	})

	got := tok.Encode("Hello world")
	want := []string{"hello", "world"}
	if !slicesEqual(got, want) {
		t.Fatalf("Encode(Hello world) = %#v, want %#v", got, want)
	}
	if tok.Count("Hello world") != 2 {
		t.Fatalf("Count = %d, want 2", tok.Count("Hello world"))
	}

	// "run" + "##ing" covers "runing" (greedy left-to-right WordPiece).
	got = tok.Encode("runing")
	want = []string{"run", "##ing"}
	if !slicesEqual(got, want) {
		t.Fatalf("Encode(runing) = %#v, want %#v", got, want)
	}

	got = tok.Encode("hello!")
	want = []string{"hello", "!"}
	// "!" may be UNK if not in vocab
	if len(got) < 1 || got[0] != "hello" {
		t.Fatalf("Encode(hello!) = %#v, want hello + punct/unk", got)
	}
}

func TestCleanTextAndAccents(t *testing.T) {
	tok := CreateBertWordPieceTokenizer(TokenizerJson{
		Normalizer: &NormalizerConfig{
			CleanText:          boolPtr(true),
			HandleChineseChars: boolPtr(false),
			Lowercase:          boolPtr(true),
			StripAccents:       boolPtr(true),
		},
		Model: ModelConfig{
			Type: "WordPiece",
			Vocab: map[string]int{
				"cafe":  1,
				"[UNK]": 0,
			},
			UnkToken: "[UNK]",
		},
	})

	got := tok.Encode("Café")
	want := []string{"cafe"}
	if !slicesEqual(got, want) {
		t.Fatalf("Encode(Café) = %#v, want %#v", got, want)
	}
}

func TestChineseCharSpacing(t *testing.T) {
	tok := CreateBertWordPieceTokenizer(TokenizerJson{
		Normalizer: &NormalizerConfig{
			CleanText:          boolPtr(true),
			HandleChineseChars: boolPtr(true),
			Lowercase:          boolPtr(true),
		},
		Model: ModelConfig{
			Type: "WordPiece",
			Vocab: map[string]int{
				"你":     1,
				"好":     2,
				"[UNK]": 0,
			},
			UnkToken: "[UNK]",
		},
	})

	got := tok.Encode("你好")
	want := []string{"你", "好"}
	if !slicesEqual(got, want) {
		t.Fatalf("Encode(你好) = %#v, want %#v", got, want)
	}
}

func TestLoadFromJSONRequiresWordPiece(t *testing.T) {
	_, err := LoadFromJSON([]byte(`{"model":{"type":"BPE","vocab":{}}}`))
	if err == nil {
		t.Fatal("expected error for non-WordPiece model")
	}
}

func boolPtr(v bool) *bool { return &v }

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
