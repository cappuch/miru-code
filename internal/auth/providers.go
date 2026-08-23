package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/takara-ai/miru-code/internal/cliui"
	"github.com/takara-ai/miru-code/internal/spinner"
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
	// Interactive setup defaults to device code — API keys stay behind --key.
	if opts.Device || opts.Interactive {
		return KindDeviceCode, nil
	}
	return "", fmt.Errorf("Choose an auth mode with `miru setup --device` or `miru setup --key TOKEN`.")
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
		fmt.Fprint(os.Stderr, "Takara API key (input hidden): ")
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
	cliui.WriteStderr("")
	spin := spinner.New("Authenticating")
	spin.Start()
	start, err := StartDeviceAuthorization(nil, nil)
	if err != nil {
		spin.Stop("")
		if opts.AllowManualFallback && opts.Interactive {
			cliui.Warn(err.Error())
			if promptConfirm("Enter an API key instead?", true) {
				return authenticateWithAPIKey(opts)
			}
		}
		return AuthenticatedCredentials{}, err
	}

	verificationURL := start.VerificationURI
	if start.VerificationURIComplete != "" {
		verificationURL = start.VerificationURIComplete
	}

	shouldOpenBrowser := opts.Interactive &&
		(os.Getenv("MIRU_OPEN_BROWSER") == "" || os.Getenv("MIRU_OPEN_BROWSER") == "1")
	if shouldOpenBrowser {
		_ = OpenBrowserForDeviceLogin(verificationURL)
	}

	visit := cliui.Dim("  Visit " + verificationURL)
	if start.VerificationURIComplete == "" {
		visit += "\n" + cliui.Dim("  Code: "+start.UserCode)
	}
	spin.Follow(visit)

	tokens, err := PollDeviceAuthorization(start, nil, nil)
	if err != nil {
		spin.Stop("")
		if opts.AllowManualFallback && opts.Interactive {
			cliui.Warn(err.Error())
			if promptConfirm("Enter an API key instead?", true) {
				return authenticateWithAPIKey(opts)
			}
		}
		return AuthenticatedCredentials{}, err
	}
	spin.Succeed("")
	return AuthenticatedCredentials{
		Kind:         KindDeviceCode,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
		TokenType:    tokens.TokenType,
		Scope:        tokens.Scope,
	}, nil
}
