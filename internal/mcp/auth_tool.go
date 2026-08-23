package mcp

import (
	"fmt"
	"os"
	"time"

	"github.com/takara-ai/miru-code/internal/auth"
	"github.com/takara-ai/miru-code/internal/autherr"
	"github.com/takara-ai/miru-code/internal/credentials"
)

const checkHint =
	`Once the user approves, call ` + "`auth`" + ` again with action "check" to finish signing in.`

// Appended to credentials failures so an agent knows Miru's own recovery step.
const recoveryHint =
	`Miru could not authorize its current credentials. Call the ` + "`auth`" + ` tool with action "start" ` +
		`to open the Takara device-login page, then action "check" once the user approves. If access ` +
		`is still denied after signing in, check the account token balance.`

const authToolDescription =
	"Sign in with Takara credentials via device-code login — no terminal required. " +
		"Only call this in direct response to a tool error mentioning missing, expired, rejected, or " +
		"invalid credentials — never speculatively, since it starts a real sign-in prompt for the " +
		"user. Call with no arguments (or action \"start\") to begin: it opens the device-login " +
		"page in the user's browser and returns that URL plus a short code. " + checkHint

// BrowserOpener opens a URL; the real one spawns a detached open/xdg-open/start.
type BrowserOpener func(url string) bool

func openIfAllowed(open BrowserOpener, url string) bool {
	raw := os.Getenv("MIRU_OPEN_BROWSER")
	if raw != "" && raw != "1" {
		return false
	}
	return open(url)
}

func approvalText(link, userCode string, opened bool) string {
	lead := fmt.Sprintf("Ask the user to open %s", link)
	if opened {
		lead = fmt.Sprintf("A browser tab is open at %s", link)
	}
	return fmt.Sprintf("%s. Code: %s. %s", lead, userCode, checkHint)
}

// toolErrorText formats tool-failure text, appending the sign-in step when relevant.
func toolErrorText(err error) ToolResult {
	message := err.Error()
	if autherr.Is(err) {
		return ToolText(message + "\n\n" + recoveryHint)
	}
	return ToolText(message)
}

type pendingDeviceAuth struct {
	start      auth.DeviceAuthorizationStart
	config     auth.DeviceAuthConfig
	startedAt  time.Time
}

func (p *pendingDeviceAuth) expired() bool {
	return time.Now().After(p.startedAt.Add(time.Duration(p.start.ExpiresIn) * time.Second))
}

type authToolState struct {
	pending     *pendingDeviceAuth
	openBrowser BrowserOpener
}

func (s *authToolState) start() (ToolResult, error) {
	pending := s.pending
	if pending == nil || pending.expired() {
		config := auth.ResolveDeviceAuthConfig()
		start, err := auth.StartDeviceAuthorization(&config, nil)
		if err != nil {
			return ToolText(err.Error()), nil
		}
		pending = &pendingDeviceAuth{start: start, config: config, startedAt: time.Now()}
		s.pending = pending
	}

	link := pending.start.VerificationURI
	if pending.start.VerificationURIComplete != "" {
		link = pending.start.VerificationURIComplete
	}
	// Reopening on a repeat call is deliberate: the user may have closed the first tab.
	opened := openIfAllowed(s.openBrowser, link)
	return ToolText(approvalText(link, pending.start.UserCode, opened)), nil
}

func (s *authToolState) check() (ToolResult, error) {
	if s.pending == nil {
		return ToolText(`No device login is pending. Call ` + "`auth`" + ` with action "start" first.`), nil
	}
	result, err := auth.CheckDeviceAuthorizationOnce(s.pending.start, &s.pending.config, nil)
	if err != nil {
		// Leave pending intact — a transient failure shouldn't force a restart.
		return ToolText(err.Error()), nil
	}
	switch result.Status {
	case "success":
		s.pending = nil
		_, _ = credentials.SaveDeviceCode(auth.SaveDeviceCodeInput{
			AccessToken:  result.Tokens.AccessToken,
			RefreshToken: result.Tokens.RefreshToken,
			ExpiresAt:    result.Tokens.ExpiresAt,
			TokenType:    result.Tokens.TokenType,
			Scope:        result.Tokens.Scope,
		})
		credentials.SetStoredCredentialsEnvToken(result.Tokens.AccessToken)
		return ToolText("Signed in successfully. Miru tools are now ready to use."), nil
	case "pending":
		return ToolText(`Still waiting for approval. Ask the user to confirm they clicked and approved, then call ` + "`auth`" + ` again with action "check".`), nil
	case "slow_down":
		return ToolText(`Checking too soon — wait a bit before calling ` + "`auth`" + ` again with action "check".`), nil
	case "denied":
		s.pending = nil
		return ToolText(`Sign-in was denied. Call ` + "`auth`" + ` with action "start" to try again.`), nil
	case "expired":
		s.pending = nil
		return ToolText(`The device code expired before it was approved. Call ` + "`auth`" + ` with action "start" to try again.`), nil
	default:
		return ToolText("Unexpected auth status."), nil
	}
}

func registerAuthTool(server *MiruMcpServer) {
	state := &authToolState{openBrowser: auth.OpenBrowserForDeviceLogin}
	server.RegisterTool("auth", ToolSchema{
		Description: authToolDescription,
		InputSchema: ObjectSchema(map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"start", "check"},
				"description": `"start" begins a device-code login (default); "check" completes it.`,
			},
		}, nil),
		Handler: func(args map[string]any) (ToolResult, error) {
			action := StringArg(args, "action")
			if action == "check" {
				return state.check()
			}
			return state.start()
		},
	})
}
