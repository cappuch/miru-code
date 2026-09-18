package miru

import (
	"context"
	"time"

	"github.com/takara-ai/miru-code/internal/auth"
	"github.com/takara-ai/miru-code/internal/credentials"
)

// DeviceLoginSession contains the information a UI needs to display a login.
// Device codes and bearer tokens remain private. Call Finish once after showing
// VerificationURL and UserCode; cancelling its context interrupts the login.
type DeviceLoginSession struct {
	VerificationURL string
	UserCode        string
	ExpiresAt       time.Time
	start           auth.DeviceAuthorizationStart
	config          auth.DeviceAuthConfig
}

// StartDeviceLogin begins Miru's native device authorization flow without opening
// a browser or writing credentials. MIRU_AUTH_* endpoint overrides are respected.
func StartDeviceLogin(ctx context.Context) (*DeviceLoginSession, error) {
	config := auth.ResolveDeviceAuthConfig()
	start, err := auth.StartDeviceAuthorizationContext(ctx, &config, nil)
	if err != nil {
		return nil, err
	}
	verificationURL := start.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = start.VerificationURI
	}
	return &DeviceLoginSession{
		VerificationURL: verificationURL, UserCode: start.UserCode,
		ExpiresAt: time.Now().Add(time.Duration(start.ExpiresIn) * time.Second),
		start:     start, config: config,
	}, nil
}

// Finish waits for approval, then saves the native refreshable credentials and
// activates them for searches in this process. Cancellation never saves tokens.
func (s *DeviceLoginSession) Finish(ctx context.Context) error {
	ctx, cancel := context.WithDeadline(ctx, s.ExpiresAt)
	defer cancel()
	tokens, err := auth.PollDeviceAuthorizationContext(ctx, s.start, &s.config, nil)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = credentials.SaveDeviceCode(auth.SaveDeviceCodeInput{
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		ExpiresAt: tokens.ExpiresAt, TokenType: tokens.TokenType, Scope: tokens.Scope,
	})
	if err != nil {
		return err
	}
	credentials.SetStoredCredentialsEnvToken(tokens.AccessToken)
	return nil
}
