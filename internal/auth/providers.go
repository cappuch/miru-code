package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/takara-ai/miru-code/internal/cliui"
)

// AuthenticateOptions configures interactive / non-interactive auth.
type AuthenticateOptions struct {
	APIKey              string
	Device              bool
	SkipValidation      bool
	AllowManualFallback bool
	Interactive         bool
}

// AuthenticateWithProvider runs device-code or API-key auth.
func AuthenticateWithProvider(opts AuthenticateOptions) (AuthenticatedCredentials, error) {
	mode, err := resolveAuthMode(opts)
	if err != nil {
		return AuthenticatedCredentials{}, err
	}
	if mode == KindDeviceCode {
		return authenticateWithDeviceCode(opts)
	}
	return authenticateWithAPIKey(opts)
}

func resolveAuthMode(opts AuthenticateOptions) (StoredCredentialKind, error) {
	if opts.APIKey != "" {
		return KindAPIKey, nil
	}
	if opts.Device {
		return KindDeviceCode, nil
	}
	if !opts.Interactive {
		return "", fmt.Errorf("Choose an auth mode with `miru setup --device` or `miru setup --key TOKEN`.")
	}
	cliui.WriteStderr("")
	cliui.Info("Choose how to authenticate.")
	useDevice := promptConfirm("Use device code login?", true)
	if useDevice {
		return KindDeviceCode, nil
	}
	return KindAPIKey, nil
}

func promptConfirm(question string, defaultYes bool) bool {
	suffix := " [Y/n] "
	if !defaultYes {
		suffix = " [y/N] "
	}
	fmt.Fprint(os.Stderr, question+suffix)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return defaultYes
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

func promptAPIKey() (string, error) {
	for {
		fmt.Fprint(os.Stderr, "Takara API key: ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return "", fmt.Errorf("API key cannot be empty.")
		}
		key := strings.TrimSpace(scanner.Text())
		if key != "" {
			return key, nil
		}
		cliui.Warn("API key cannot be empty.")
	}
}

func authenticateWithAPIKey(opts AuthenticateOptions) (AuthenticatedCredentials, error) {
	apiKey := opts.APIKey
	if apiKey == "" {
		var err error
		apiKey, err = promptAPIKey()
		if err != nil {
			return AuthenticatedCredentials{}, err
		}
	}
	if !opts.SkipValidation && strings.TrimSpace(apiKey) == "" {
		return AuthenticatedCredentials{}, fmt.Errorf("API key cannot be empty.")
	}
	return AuthenticatedCredentials{Kind: KindAPIKey, APIKey: apiKey}, nil
}

func authenticateWithDeviceCode(opts AuthenticateOptions) (AuthenticatedCredentials, error) {
	start, err := StartDeviceAuthorization(nil, nil)
	if err != nil {
		return AuthenticatedCredentials{}, err
	}
	cliui.WriteStderr("")
	cliui.Info(fmt.Sprintf("Open %s", start.VerificationURI))
	cliui.Hint(fmt.Sprintf("Code: %s", start.UserCode))
	if start.VerificationURIComplete != "" {
		cliui.Hint(fmt.Sprintf("Direct link: %s", start.VerificationURIComplete))
	}
	if opts.Interactive {
		openBrowser := os.Getenv("MIRU_OPEN_BROWSER")
		if openBrowser == "" || openBrowser == "1" {
			link := start.VerificationURI
			if start.VerificationURIComplete != "" {
				link = start.VerificationURIComplete
			}
			if OpenBrowserForDeviceLogin(link) {
				cliui.Hint("Opened the verification page in your browser.")
			}
		}
	}
	cliui.Info("Waiting for device authorization…")
	tokens, err := PollDeviceAuthorization(start, nil, nil)
	if err != nil {
		if opts.AllowManualFallback && opts.Interactive {
			cliui.Warn(err.Error())
			if promptConfirm("Enter an API key instead?", true) {
				return authenticateWithAPIKey(opts)
			}
		}
		return AuthenticatedCredentials{}, err
	}
	cliui.Success("Device login completed")
	return AuthenticatedCredentials{
		Kind:         KindDeviceCode,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
		TokenType:    tokens.TokenType,
		Scope:        tokens.Scope,
	}, nil
}
