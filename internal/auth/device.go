package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/takara-ai/miru-code/internal/env"
)

const (
	defaultAuthBaseURL    = "https://auth.takara.ai"
	defaultDeviceCodePath = "/oauth/device/code"
	defaultTokenPath      = "/oauth/token"
	defaultClientID       = "miru-code"
	expirySkew            = 60 * time.Second
)

// DeviceAuthConfig configures the device-code OAuth endpoints.
type DeviceAuthConfig struct {
	BaseURL        string
	ClientID       string
	Scope          string
	Audience       string
	DeviceCodePath string
	TokenPath      string
}

// DeviceAuthorizationStart is the response from the device-code endpoint.
type DeviceAuthorizationStart struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

// DeviceAuthorizationTokens are tokens returned after approval / refresh.
type DeviceAuthorizationTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    string
	TokenType    string
	Scope        string
}

type oauthTokenSuccess struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type oauthTokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ResolveDeviceAuthConfig reads MIRU_AUTH_* overrides.
func ResolveDeviceAuthConfig() DeviceAuthConfig {
	return DeviceAuthConfig{
		BaseURL: strings.TrimRight(env.EnvFirstString([]string{"MIRU_AUTH_BASE_URL"}, defaultAuthBaseURL), "/"),
		ClientID: env.EnvFirstString([]string{"MIRU_AUTH_CLIENT_ID"}, defaultClientID),
		Scope: strings.TrimSpace(os.Getenv("MIRU_AUTH_SCOPE")),
		Audience: strings.TrimSpace(os.Getenv("MIRU_AUTH_AUDIENCE")),
		DeviceCodePath: firstNonEmpty(strings.TrimSpace(os.Getenv("MIRU_DEVICE_CODE_PATH")), defaultDeviceCodePath),
		TokenPath:      firstNonEmpty(strings.TrimSpace(os.Getenv("MIRU_TOKEN_PATH")), defaultTokenPath),
	}
}

func firstNonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func resolveURL(baseURL, path string) string {
	u, err := url.Parse(baseURL + "/")
	if err != nil {
		return baseURL + path
	}
	rel, err := url.Parse(path)
	if err != nil {
		return baseURL + path
	}
	return u.ResolveReference(rel).String()
}

func tokenExpiryToISO(expiresIn int) string {
	if expiresIn <= 0 {
		return ""
	}
	return time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339Nano)
}

func normalizeTokenSuccess(payload oauthTokenSuccess) DeviceAuthorizationTokens {
	return DeviceAuthorizationTokens{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    tokenExpiryToISO(payload.ExpiresIn),
		TokenType:    payload.TokenType,
		Scope:        payload.Scope,
	}
}

func parseTokenError(payload oauthTokenError) string {
	code := strings.TrimSpace(payload.Error)
	desc := strings.TrimSpace(payload.ErrorDescription)
	if code != "" && desc != "" {
		return code + ": " + desc
	}
	if code != "" {
		return code
	}
	if desc != "" {
		return desc
	}
	return "unknown_error"
}

func postForm(client *http.Client, endpoint string, body url.Values) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(body.Encode()))
}

// StartDeviceAuthorization begins the device-code flow.
func StartDeviceAuthorization(config *DeviceAuthConfig, client *http.Client) (DeviceAuthorizationStart, error) {
	cfg := ResolveDeviceAuthConfig()
	if config != nil {
		cfg = *config
	}
	body := url.Values{"client_id": {cfg.ClientID}}
	if cfg.Scope != "" {
		body.Set("scope", cfg.Scope)
	}
	if cfg.Audience != "" {
		body.Set("audience", cfg.Audience)
	}
	resp, err := postForm(client, resolveURL(cfg.BaseURL, cfg.DeviceCodePath), body)
	if err != nil {
		return DeviceAuthorizationStart{}, err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(text)
		if msg == "" {
			msg = resp.Status
		}
		return DeviceAuthorizationStart{}, fmt.Errorf("Device authorization failed: %s", msg)
	}
	var payload map[string]any
	if len(text) > 0 {
		if err := json.Unmarshal(text, &payload); err != nil {
			return DeviceAuthorizationStart{}, err
		}
	}
	deviceCode := strings.TrimSpace(asString(payload["device_code"]))
	userCode := strings.TrimSpace(asString(payload["user_code"]))
	verificationURI := strings.TrimSpace(asString(payload["verification_uri"]))
	if verificationURI == "" {
		verificationURI = strings.TrimSpace(asString(payload["verification_url"]))
	}
	if deviceCode == "" || userCode == "" || verificationURI == "" {
		return DeviceAuthorizationStart{}, fmt.Errorf("Device authorization response was missing required fields.")
	}
	expiresIn := asInt(payload["expires_in"])
	if expiresIn <= 0 {
		expiresIn = 600
	}
	interval := asInt(payload["interval"])
	if interval < 0 {
		interval = 5
	}
	complete := ""
	if v, ok := payload["verification_uri_complete"].(string); ok {
		complete = v
	}
	return DeviceAuthorizationStart{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: complete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// DeviceAuthorizationCheck is one non-blocking poll result.
type DeviceAuthorizationCheck struct {
	Status string // success | pending | slow_down | denied | expired
	Tokens DeviceAuthorizationTokens
}

// CheckDeviceAuthorizationOnce polls the token endpoint once (no sleep).
func CheckDeviceAuthorizationOnce(start DeviceAuthorizationStart, config *DeviceAuthConfig, client *http.Client) (DeviceAuthorizationCheck, error) {
	cfg := ResolveDeviceAuthConfig()
	if config != nil {
		cfg = *config
	}
	body := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {start.DeviceCode},
		"client_id":   {cfg.ClientID},
	}
	resp, err := postForm(client, resolveURL(cfg.BaseURL, cfg.TokenPath), body)
	if err != nil {
		return DeviceAuthorizationCheck{}, err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var success oauthTokenSuccess
		if len(text) > 0 {
			_ = json.Unmarshal(text, &success)
		}
		if strings.TrimSpace(success.AccessToken) == "" {
			return DeviceAuthorizationCheck{}, fmt.Errorf("Device login succeeded but did not return an access token.")
		}
		return DeviceAuthorizationCheck{Status: "success", Tokens: normalizeTokenSuccess(success)}, nil
	}
	var errPayload oauthTokenError
	if len(text) > 0 {
		_ = json.Unmarshal(text, &errPayload)
	}
	switch errPayload.Error {
	case "authorization_pending":
		return DeviceAuthorizationCheck{Status: "pending"}, nil
	case "slow_down":
		return DeviceAuthorizationCheck{Status: "slow_down"}, nil
	case "access_denied":
		return DeviceAuthorizationCheck{Status: "denied"}, nil
	case "expired_token":
		return DeviceAuthorizationCheck{Status: "expired"}, nil
	default:
		return DeviceAuthorizationCheck{}, fmt.Errorf("Device login failed: %s", parseTokenError(errPayload))
	}
}

// PollDeviceAuthorization blocks until approval, denial, expiry, or timeout.
func PollDeviceAuthorization(start DeviceAuthorizationStart, config *DeviceAuthConfig, client *http.Client) (DeviceAuthorizationTokens, error) {
	deadline := time.Now().Add(time.Duration(start.ExpiresIn) * time.Second)
	interval := time.Duration(start.Interval) * time.Second
	if interval < 0 {
		interval = 5 * time.Second
	}
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		check, err := CheckDeviceAuthorizationOnce(start, config, client)
		if err != nil {
			return DeviceAuthorizationTokens{}, err
		}
		switch check.Status {
		case "success":
			return check.Tokens, nil
		case "pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "denied":
			return DeviceAuthorizationTokens{}, fmt.Errorf("Device login was denied.")
		case "expired":
			return DeviceAuthorizationTokens{}, fmt.Errorf("Device login expired before it was completed.")
		}
	}
	return DeviceAuthorizationTokens{}, fmt.Errorf("Device login timed out before it was completed.")
}

// RefreshDeviceAuthorization refreshes stored device-code credentials.
func RefreshDeviceAuthorization(credentials StoredCredentials, config *DeviceAuthConfig, client *http.Client) (DeviceAuthorizationTokens, error) {
	if strings.TrimSpace(credentials.RefreshToken) == "" {
		return DeviceAuthorizationTokens{}, fmt.Errorf("Stored device credentials cannot be refreshed because no refresh token exists.")
	}
	cfg := ResolveDeviceAuthConfig()
	if config != nil {
		cfg = *config
	}
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {credentials.RefreshToken},
		"client_id":     {cfg.ClientID},
	}
	resp, err := postForm(client, resolveURL(cfg.BaseURL, cfg.TokenPath), body)
	if err != nil {
		return DeviceAuthorizationTokens{}, err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errPayload oauthTokenError
		_ = json.Unmarshal(text, &errPayload)
		return DeviceAuthorizationTokens{}, fmt.Errorf("Device token refresh failed: %s", parseTokenError(errPayload))
	}
	var success oauthTokenSuccess
	_ = json.Unmarshal(text, &success)
	if strings.TrimSpace(success.AccessToken) == "" {
		return DeviceAuthorizationTokens{}, fmt.Errorf("Token refresh succeeded but did not return an access token.")
	}
	if success.RefreshToken == "" {
		success.RefreshToken = credentials.RefreshToken
	}
	return normalizeTokenSuccess(success), nil
}

// DeviceCredentialsNeedRefresh reports whether device credentials are near expiry.
func DeviceCredentialsNeedRefresh(credentials StoredCredentials) bool {
	if credentials.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, credentials.ExpiresAt)
	if err != nil {
		expiresAt, err = time.Parse(time.RFC3339, credentials.ExpiresAt)
	}
	if err != nil {
		return true
	}
	return !expiresAt.After(time.Now().Add(expirySkew))
}

// OpenBrowserForDeviceLogin opens the verification URL in the system browser.
func OpenBrowserForDeviceLogin(rawURL string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start() == nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}
