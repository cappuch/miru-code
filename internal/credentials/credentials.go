package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/takara-ai/miru-code/internal/auth"
	"github.com/takara-ai/miru-code/internal/env"
)

const credentialsFilename = "credentials.json"

var (
	activeStoredToken   string
	activeStoredTokenMu sync.Mutex
)

// SetStoredCredentialsEnvToken sets TAKARA_API_KEY and tracks it as the loaded token.
func SetStoredCredentialsEnvToken(token string) {
	activeStoredTokenMu.Lock()
	activeStoredToken = token
	activeStoredTokenMu.Unlock()
	_ = os.Setenv(env.TakaraAPIKeyEnv, token)
}

// ResolveCredentialsDir returns the platform Miru config directory.
func ResolveCredentialsDir() string {
	if override := strings.TrimSpace(os.Getenv("MIRU_CREDENTIALS_DIR")); override != "" {
		return override
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "miru")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "miru")
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, "miru")
	}
}

// ResolveMiruStateDir is an alias for ResolveCredentialsDir for non-secret state files.
func ResolveMiruStateDir() string {
	return ResolveCredentialsDir()
}

// ResolveCredentialsPath returns credentials.json path.
func ResolveCredentialsPath() string {
	return filepath.Join(ResolveCredentialsDir(), credentialsFilename)
}

// ReadStoredCredentials loads and validates credentials.json (nil if absent/invalid).
func ReadStoredCredentials() (*auth.StoredCredentials, error) {
	path := ResolveCredentialsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, nil
	}
	if probe.Version == auth.LegacyCredentialsVersion {
		var legacy auth.LegacyStoredCredentials
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, nil
		}
		if legacy.SageMaker != nil && strings.TrimSpace(legacy.SageMaker.EndpointARN) != "" {
			return &auth.StoredCredentials{
				Version:     auth.CredentialsVersion,
				Kind:        auth.KindSageMaker,
				EndpointARN: legacy.SageMaker.EndpointARN,
				Profile:     legacy.SageMaker.Profile,
			}, nil
		}
		if strings.TrimSpace(legacy.TakaraAPIKey) != "" {
			return &auth.StoredCredentials{
				Version: auth.CredentialsVersion,
				Kind:    auth.KindAPIKey,
				APIKey:  legacy.TakaraAPIKey,
			}, nil
		}
		return nil, nil
	}
	var stored auth.StoredCredentials
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, nil
	}
	if stored.Version != auth.CredentialsVersion || stored.Kind == "" {
		return nil, nil
	}
	switch stored.Kind {
	case auth.KindAPIKey:
		if strings.TrimSpace(stored.APIKey) == "" {
			return nil, nil
		}
		return &stored, nil
	case auth.KindSageMaker:
		if strings.TrimSpace(stored.EndpointARN) == "" {
			return nil, nil
		}
		return &stored, nil
	case auth.KindDeviceCode:
		if strings.TrimSpace(stored.AccessToken) == "" {
			return nil, nil
		}
		return &stored, nil
	default:
		return nil, nil
	}
}

func clearSageMakerEnv(profile string) {
	_ = os.Unsetenv("MIRU_SAGEMAKER_ENDPOINT_ARN")
	_ = os.Unsetenv("MIRU_SAGEMAKER_ENDPOINT_NAME")
	_ = os.Unsetenv("MIRU_SAGEMAKER_REGION")
	if profile != "" && os.Getenv("AWS_PROFILE") == profile {
		_ = os.Unsetenv("AWS_PROFILE")
	}
}

func clearTakaraEnv() {
	_ = os.Unsetenv(env.TakaraAPIKeyEnv)
}

func hydrateSageMakerEnv(c auth.StoredCredentials) {
	_ = os.Setenv("MIRU_SAGEMAKER_ENDPOINT_ARN", c.EndpointARN)
	if c.Profile != "" && os.Getenv("AWS_PROFILE") == "" {
		_ = os.Setenv("AWS_PROFILE", c.Profile)
	}
}

// BeginModeSwitch clears the other backend from env so validation cannot see a stale mode.
func BeginModeSwitch(to string) error {
	stored, _ := ReadStoredCredentials()
	if to == "takara" {
		profile := ""
		if stored != nil && stored.Kind == auth.KindSageMaker {
			profile = stored.Profile
		}
		clearSageMakerEnv(profile)
	} else {
		clearTakaraEnv()
	}
	return nil
}

func envUsesStoredToken() bool {
	activeStoredTokenMu.Lock()
	tok := activeStoredToken
	activeStoredTokenMu.Unlock()
	return tok != "" && os.Getenv(env.TakaraAPIKeyEnv) == tok
}

// LoadStoredCredentials hydrates TAKARA_API_KEY / SageMaker env from credentials.json.
func LoadStoredCredentials() (bool, error) {
	env.NormalizeTakaraAPIKeyEnv()
	stored, err := ReadStoredCredentials()
	if err != nil {
		return false, err
	}
	if stored == nil {
		return false, nil
	}

	if stored.Kind == auth.KindSageMaker {
		changed := false
		if strings.TrimSpace(os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN")) == "" {
			hydrateSageMakerEnv(*stored)
			changed = true
		}
		if env.HasTakaraAPIKeyInEnv() {
			clearTakaraEnv()
			changed = true
		}
		return changed, nil
	}

	needsSageMakerClear := os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN") != "" ||
		os.Getenv("MIRU_SAGEMAKER_ENDPOINT_NAME") != ""
	needsToken := !env.HasTakaraAPIKeyInEnv() || envUsesStoredToken()

	if !needsToken && !needsSageMakerClear {
		return false, nil
	}
	if needsSageMakerClear {
		clearSageMakerEnv("")
	}
	if !needsToken {
		return true, nil
	}

	if stored.Kind == auth.KindDeviceCode && auth.DeviceCredentialsNeedRefresh(*stored) {
		refreshed, err := auth.RefreshDeviceAuthorization(*stored, nil, nil)
		if err != nil {
			return false, err
		}
		_, err = SaveDeviceCode(auth.SaveDeviceCodeInput{
			AccessToken:  refreshed.AccessToken,
			RefreshToken: refreshed.RefreshToken,
			ExpiresAt:    refreshed.ExpiresAt,
			TokenType:    refreshed.TokenType,
			Scope:        refreshed.Scope,
		})
		if err != nil {
			return false, err
		}
		SetStoredCredentialsEnvToken(refreshed.AccessToken)
		return true, nil
	}
	SetStoredCredentialsEnvToken(auth.CredentialAccessToken(*stored))
	return true, nil
}

func writeCredentials(payload auth.StoredCredentials) (string, error) {
	dir := ResolveCredentialsDir()
	path := ResolveCredentialsPath()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o600)

	if payload.Kind == auth.KindSageMaker {
		clearTakaraEnv()
		activeStoredTokenMu.Lock()
		activeStoredToken = ""
		activeStoredTokenMu.Unlock()
		_ = os.Setenv("MIRU_SAGEMAKER_ENDPOINT_ARN", payload.EndpointARN)
		if payload.Profile != "" {
			_ = os.Setenv("AWS_PROFILE", payload.Profile)
		}
	} else {
		_ = os.Unsetenv("MIRU_SAGEMAKER_ENDPOINT_ARN")
		_ = os.Unsetenv("MIRU_SAGEMAKER_ENDPOINT_NAME")
		_ = os.Unsetenv("MIRU_SAGEMAKER_REGION")
		_ = os.Unsetenv("AWS_PROFILE")
		if payload.Kind == auth.KindAPIKey {
			SetStoredCredentialsEnvToken(payload.APIKey)
		}
	}
	return path, nil
}

// SaveAPIKey writes api_key credentials.
func SaveAPIKey(apiKey string) (string, error) {
	return writeCredentials(auth.StoredCredentials{
		Version: auth.CredentialsVersion,
		Kind:    auth.KindAPIKey,
		APIKey:  apiKey,
	})
}

// SaveDeviceCode writes device_code credentials.
func SaveDeviceCode(in auth.SaveDeviceCodeInput) (string, error) {
	return writeCredentials(auth.StoredCredentials{
		Version:      auth.CredentialsVersion,
		Kind:         auth.KindDeviceCode,
		AccessToken:  in.AccessToken,
		RefreshToken: in.RefreshToken,
		ExpiresAt:    in.ExpiresAt,
		TokenType:    in.TokenType,
		Scope:        in.Scope,
	})
}

// SaveSageMaker writes sagemaker credentials.
func SaveSageMaker(in auth.SaveSageMakerInput) (string, error) {
	return writeCredentials(auth.StoredCredentials{
		Version:     auth.CredentialsVersion,
		Kind:        auth.KindSageMaker,
		EndpointARN: in.EndpointARN,
		Profile:     in.Profile,
	})
}

// SaveFromAuthenticated persists Takara auth results.
func SaveFromAuthenticated(creds auth.AuthenticatedCredentials) (string, error) {
	if creds.Kind == auth.KindAPIKey {
		return SaveAPIKey(creds.APIKey)
	}
	return SaveDeviceCode(auth.SaveDeviceCodeInput{
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		ExpiresAt:    creds.ExpiresAt,
		TokenType:    creds.TokenType,
		Scope:        creds.Scope,
	})
}

// ClearResult is the outcome of ClearStoredCredentials.
type ClearResult struct {
	Cleared bool
	Path    string
}

// ClearStoredCredentials removes credentials.json and cleans related env.
func ClearStoredCredentials() (ClearResult, error) {
	path := ResolveCredentialsPath()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ClearResult{Cleared: false, Path: path}, nil
		}
		return ClearResult{}, err
	}
	stored, _ := ReadStoredCredentials()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return ClearResult{}, err
	}
	if stored != nil {
		if stored.Kind == auth.KindSageMaker {
			if os.Getenv("MIRU_SAGEMAKER_ENDPOINT_ARN") == stored.EndpointARN {
				_ = os.Unsetenv("MIRU_SAGEMAKER_ENDPOINT_ARN")
			}
			if stored.Profile != "" && os.Getenv("AWS_PROFILE") == stored.Profile {
				_ = os.Unsetenv("AWS_PROFILE")
			}
		} else if os.Getenv(env.TakaraAPIKeyEnv) == auth.CredentialAccessToken(*stored) {
			_ = os.Unsetenv(env.TakaraAPIKeyEnv)
		}
	}
	activeStoredTokenMu.Lock()
	activeStoredToken = ""
	activeStoredTokenMu.Unlock()
	return ClearResult{Cleared: true, Path: path}, nil
}
