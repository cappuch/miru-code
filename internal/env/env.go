package env

import (
	"os"
	"strconv"
	"strings"

	"github.com/takara-ai/miru-code/internal/autherr"
)

// EnvInt reads an int env var with fallback and minimum.
func EnvInt(name string, fallback, min int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return fallback
	}
	return n
}

// EnvFirstString returns the first non-empty env value among names.
func EnvFirstString(names []string, fallback string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return fallback
}

// EnvOptionalInt returns the first valid int >= min among names, or nil.
func EnvOptionalInt(names []string, min int) *int {
	for _, name := range names {
		raw := os.Getenv(name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < min {
			continue
		}
		v := n
		return &v
	}
	return nil
}

const TakaraAPIKeyEnv = "TAKARA_API_KEY"

var takaraAPIKeyPlaceholders = map[string]struct{}{
	"${TAKARA_API_KEY}": {},
	"$TAKARA_API_KEY":   {},
}

// IsUsableTakaraAPIKey reports a real key (not empty/placeholder).
func IsUsableTakaraAPIKey(value string) bool {
	key := strings.TrimSpace(value)
	if key == "" {
		return false
	}
	_, placeholder := takaraAPIKeyPlaceholders[key]
	return !placeholder
}

// NormalizeTakaraAPIKeyEnv drops placeholder values so credentials.json can supply the key.
func NormalizeTakaraAPIKeyEnv() {
	if !IsUsableTakaraAPIKey(os.Getenv(TakaraAPIKeyEnv)) {
		_ = os.Unsetenv(TakaraAPIKeyEnv)
	}
}

// HasTakaraAPIKeyInEnv reports whether a usable Takara key is already in the environment.
func HasTakaraAPIKeyInEnv() bool {
	return IsUsableTakaraAPIKey(os.Getenv(TakaraAPIKeyEnv))
}

// ResolveEmbeddingAPIKey returns TAKARA_API_KEY or a CredentialsError when missing.
func ResolveEmbeddingAPIKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(TakaraAPIKeyEnv))
	if !IsUsableTakaraAPIKey(key) {
		return "", autherr.New(
			"Takara credentials required. If you're an agent with Miru MCP tools available, call the "+
				"`auth` tool to sign in. Otherwise run `miru setup`, or set TAKARA_API_KEY in your MCP "+
				"server env or .env.local.",
			nil,
		)
	}
	return key, nil
}

// IsSageMakerConfigured reports whether SageMaker embedding env is set.
func IsSageMakerConfigured() bool {
	return strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN")) != "" ||
		strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_NAME")) != ""
}
