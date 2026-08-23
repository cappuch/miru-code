package setup

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/takara-ai/miru-code/internal/auth"
	"github.com/takara-ai/miru-code/internal/cliui"
	"github.com/takara-ai/miru-code/internal/credentials"
	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/env"
)

// RunSetupOptions configures RunSetup.
type RunSetupOptions struct {
	APIKey              string
	Device              bool
	Force               bool
	SkipValidation      bool
	SageMaker           bool
	SageMakerARN        string
	Profile             string
	AllowManualFallback *bool
	Interactive         *bool
}

// RunSetupResult is the outcome of RunSetup.
type RunSetupResult struct {
	Path       string
	NewlySaved bool
}

// ParsedSetupCLIArgs are flags for `miru setup`.
type ParsedSetupCLIArgs struct {
	APIKey       string
	Device       bool
	Force        bool
	Clear        bool
	SageMaker    bool
	SageMakerARN string
	Profile      string
}

// SetupCLIArgError is a mutually exclusive flag conflict.
type SetupCLIArgError string

const (
	ErrClearWithKey         SetupCLIArgError = "clear_with_key"
	ErrSageMakerWithKey     SetupCLIArgError = "sagemaker_with_key"
	ErrDeviceWithKey        SetupCLIArgError = "device_with_key"
	ErrDeviceWithSageMaker  SetupCLIArgError = "device_with_sagemaker"
)

// ParseSetupCLIArgs parses argv after the `setup` command.
func ParseSetupCLIArgs(rest []string) (ParsedSetupCLIArgs, SetupCLIArgError) {
	var args ParsedSetupCLIArgs
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--force":
			args.Force = true
		case arg == "--clear":
			args.Clear = true
		case arg == "--device":
			args.Device = true
		case (arg == "--key" || arg == "-k") && i+1 < len(rest):
			i++
			args.APIKey = rest[i]
		case arg == "--sagemaker":
			args.SageMaker = true
		case arg == "--arn" && i+1 < len(rest):
			i++
			args.SageMakerARN = rest[i]
		case arg == "--profile" && i+1 < len(rest):
			i++
			args.Profile = rest[i]
		}
	}
	if args.SageMakerARN != "" {
		args.SageMaker = true
	}
	if args.Clear && (args.APIKey != "" || args.Device || args.SageMaker) {
		return args, ErrClearWithKey
	}
	if args.SageMaker && args.APIKey != "" {
		return args, ErrSageMakerWithKey
	}
	if args.Device && args.APIKey != "" {
		return args, ErrDeviceWithKey
	}
	if args.Device && args.SageMaker {
		return args, ErrDeviceWithSageMaker
	}
	return args, ""
}

var sagemakerARNRe = regexp.MustCompile(`^arn:aws:sagemaker:([a-z0-9-]+):\d+:endpoint/(.+)$`)

// ParsedSageMakerARN is a parsed endpoint ARN.
type ParsedSageMakerARN struct {
	Region       string
	EndpointName string
}

// ParseSageMakerEndpointARN validates and parses an endpoint ARN.
func ParseSageMakerEndpointARN(arn string) (ParsedSageMakerARN, error) {
	m := sagemakerARNRe.FindStringSubmatch(strings.TrimSpace(arn))
	if m == nil {
		return ParsedSageMakerARN{}, fmt.Errorf("invalid SageMaker endpoint ARN (expected arn:aws:sagemaker:<region>:<account>:endpoint/<name>)")
	}
	return ParsedSageMakerARN{Region: m[1], EndpointName: m[2]}, nil
}

func promptText(label, def string) string {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return def
	}
	v := strings.TrimSpace(scanner.Text())
	if v == "" {
		return def
	}
	return v
}

// RunSageMakerSetup configures SageMaker credentials.
func RunSageMakerSetup(opts RunSetupOptions) (RunSetupResult, error) {
	cliui.WriteStdout("")
	cliui.PrintBrandBanner(os.Stderr)
	cliui.Divider("─", 48, os.Stderr)
	cliui.WriteStdout("Miru will connect directly to your self-hosted SageMaker embedding endpoint.")
	cliui.Hint("Miru only inherits AWS credentials from a profile you've already configured —")
	cliui.Hint("it never creates or writes to ~/.aws. Run `aws configure --profile <name>` first.")
	cliui.Hint("This replaces any stored Takara credentials — only one embedding mode is active at a time.")
	cliui.WriteStdout("")

	arnInput := opts.SageMakerARN
	if arnInput == "" {
		if !CanPromptForCredentials() {
			return RunSetupResult{}, fmt.Errorf("An endpoint ARN is required. Pass --arn <arn>.")
		}
		for {
			arnInput = promptText("SageMaker endpoint ARN", "")
			if arnInput == "" {
				cliui.Fail("Endpoint ARN cannot be empty.")
				continue
			}
			if _, err := ParseSageMakerEndpointARN(arnInput); err != nil {
				cliui.Fail(err.Error())
				continue
			}
			break
		}
	} else if _, err := ParseSageMakerEndpointARN(arnInput); err != nil {
		return RunSetupResult{}, err
	}

	profile := opts.Profile
	if profile == "" {
		if !CanPromptForCredentials() {
			return RunSetupResult{}, fmt.Errorf("An AWS profile name is required. Pass --profile <name>, or run `miru setup --sagemaker` interactively.")
		}
		for {
			def := os.Getenv("AWS_PROFILE")
			if def == "" {
				def = "miru"
			}
			profile = promptText("AWS profile name (must already exist in ~/.aws)", def)
			if profile != "" {
				break
			}
			cliui.Fail("AWS profile name cannot be empty.")
		}
	}

	_ = credentials.BeginModeSwitch("sagemaker")
	_ = os.Setenv("MIRU_SAGEMAKER_ENDPOINT_ARN", arnInput)
	_ = os.Setenv("AWS_PROFILE", profile)

	parsed, err := ParseSageMakerEndpointARN(arnInput)
	if err != nil {
		return RunSetupResult{}, err
	}
	smCfg := embed.SageMakerConfig{
		EndpointName:        parsed.EndpointName,
		Region:              parsed.Region,
		Normalize:           true,
		Truncate:            true,
		TruncationDirection: "Right",
	}
	cliui.Info("Validating SageMaker endpoint…")
	if valid, status, message := embed.ValidateSageMakerConnection(smCfg); !valid {
		if status != 0 {
			cliui.Fail(fmt.Sprintf("SageMaker validation failed (%d): %s", status, message))
		} else {
			cliui.Fail("SageMaker validation failed: " + message)
		}
		return RunSetupResult{}, fmt.Errorf("%s", message)
	}
	cliui.Success("SageMaker endpoint responded successfully.")

	had, _ := credentials.ReadStoredCredentials()
	path, err := credentials.SaveSageMaker(auth.SaveSageMakerInput{EndpointARN: arnInput, Profile: profile})
	if err != nil {
		return RunSetupResult{}, err
	}
	cliui.WriteStdout("")
	cliui.Success("Saved SageMaker config to " + path)
	if had != nil {
		cliui.Hint("Removed the stored Takara credentials — Miru now embeds only via SageMaker.")
	} else {
		cliui.Hint("Miru will embed via this SageMaker endpoint (Takara is not used).")
	}
	cliui.WriteStdout("")
	return RunSetupResult{Path: path, NewlySaved: true}, nil
}

func interactiveFlag(opts RunSetupOptions) bool {
	if opts.Interactive != nil {
		return *opts.Interactive
	}
	return CanPromptForCredentials()
}

func allowManualFallback(opts RunSetupOptions) bool {
	if opts.AllowManualFallback != nil {
		return *opts.AllowManualFallback
	}
	return interactiveFlag(opts)
}

// RunSetup authenticates and saves credentials.
func RunSetup(opts RunSetupOptions) (RunSetupResult, error) {
	if opts.SageMaker || opts.SageMakerARN != "" {
		return RunSageMakerSetup(opts)
	}

	interactive := interactiveFlag(opts)
	if !opts.Force && env.HasTakaraAPIKeyInEnv() && opts.APIKey == "" && !opts.Device {
		path := credentials.ResolveCredentialsPath()
		stored, _ := credentials.ReadStoredCredentials()
		if stored != nil && stored.Kind == auth.KindSageMaker {
			key, err := env.ResolveEmbeddingAPIKey()
			if err != nil {
				return RunSetupResult{}, err
			}
			saved, err := credentials.SaveAPIKey(key)
			if err != nil {
				return RunSetupResult{}, err
			}
			cliui.WriteStdout("")
			cliui.Success("Saved credentials to " + saved)
			cliui.Hint("Removed the stored SageMaker endpoint — Miru now embeds only via Takara.")
			cliui.WriteStdout("")
			return RunSetupResult{Path: saved, NewlySaved: true}, nil
		}
		if stored != nil {
			cliui.Info(fmt.Sprintf("Credentials already configured (env + %s). Use --force to replace stored credentials.", path))
			return RunSetupResult{Path: path, NewlySaved: false}, nil
		}
		cliui.Info("API key already set via environment variable. Stored credentials unchanged.")
		return RunSetupResult{Path: path, NewlySaved: false}, nil
	}

	if !opts.Force {
		stored, _ := credentials.ReadStoredCredentials()
		if stored != nil && stored.Kind != auth.KindSageMaker && opts.APIKey == "" && !opts.Device {
			path := credentials.ResolveCredentialsPath()
			cliui.Info(fmt.Sprintf("Credentials already stored at %s. Use --force to replace.", path))
			credentials.SetStoredCredentialsEnvToken(auth.CredentialAccessToken(*stored))
			return RunSetupResult{Path: path, NewlySaved: false}, nil
		}
	}

	cliui.WriteStderr("")
	cliui.PrintBrandBanner(os.Stderr)
	cliui.Divider("─", 48, os.Stderr)
	cliui.WriteStdout("Miru needs Takara credentials for code embeddings.")
	cliui.Hint("Device code login is the default. Manual API key entry is still available.")
	cliui.Hint("This replaces any stored SageMaker endpoint — only one embedding mode is active at a time.")
	cliui.WriteStdout("")

	_ = credentials.BeginModeSwitch("takara")
	hadSageMaker := false
	if stored, _ := credentials.ReadStoredCredentials(); stored != nil && stored.Kind == auth.KindSageMaker {
		hadSageMaker = true
	}

	creds, err := auth.AuthenticateWithProvider(auth.AuthenticateOptions{
		APIKey:              opts.APIKey,
		Device:              opts.Device,
		SkipValidation:      opts.SkipValidation,
		AllowManualFallback: allowManualFallback(opts),
		Interactive:         interactive,
	})
	if err != nil {
		return RunSetupResult{}, err
	}
	path, err := credentials.SaveFromAuthenticated(creds)
	if err != nil {
		return RunSetupResult{}, err
	}
	token := creds.APIKey
	if creds.Kind == auth.KindDeviceCode {
		token = creds.AccessToken
	}
	credentials.SetStoredCredentialsEnvToken(token)
	cliui.WriteStdout("")
	cliui.Success("Saved credentials to " + path)
	if hadSageMaker {
		cliui.Hint("Removed the stored SageMaker endpoint — Miru now embeds only via Takara.")
	} else {
		cliui.Hint("MCP loads this key from credentials.json automatically.")
	}
	cliui.WriteStdout("")
	return RunSetupResult{Path: path, NewlySaved: true}, nil
}

// RunClearCredentials removes stored credentials.
func RunClearCredentials() error {
	result, err := credentials.ClearStoredCredentials()
	if err != nil {
		return err
	}
	if result.Cleared {
		cliui.Success("Removed stored credentials from " + result.Path)
		return nil
	}
	cliui.Info("No stored credentials at " + result.Path)
	return nil
}

// CanPromptForCredentials is true when stdin is a TTY.
func CanPromptForCredentials() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// HasCredentials reports whether embedding credentials are available.
func HasCredentials() bool {
	if env.IsSageMakerConfigured() {
		return true
	}
	_, err := env.ResolveEmbeddingAPIKey()
	return err == nil
}

// EnsureCredentials loads/stores credentials or errors in non-interactive mode.
func EnsureCredentials(interactive bool) error {
	if HasCredentials() {
		return nil
	}
	var refreshErr error
	if _, err := credentials.LoadStoredCredentials(); err != nil {
		refreshErr = err
	}
	if HasCredentials() {
		return nil
	}
	if interactive {
		cliui.WriteStdout("")
		if refreshErr != nil {
			cliui.Info("Stored credentials could not be used: " + refreshErr.Error())
			cliui.Hint("Starting a fresh device-code login.")
		} else {
			cliui.Info("No Takara credentials found.")
			cliui.Hint("Starting the same device-code login flow as `miru setup`.")
		}
		fallback := false
		inter := true
		_, err := RunSetup(RunSetupOptions{
			Device:              true,
			Force:               true,
			AllowManualFallback: &fallback,
			Interactive:         &inter,
		})
		if err != nil {
			return err
		}
		_, err = env.ResolveEmbeddingAPIKey()
		return err
	}
	if refreshErr != nil {
		return refreshErr
	}
	cliui.WriteStdout("")
	cliui.PrintBrandBanner(os.Stderr)
	cliui.WriteStdout("")
	return fmt.Errorf("Takara credentials required. If you're an agent with Miru MCP tools available, call the `auth` tool to sign in. Otherwise run `miru setup` or `miru setup --key TOKEN` in an interactive terminal, or set TAKARA_API_KEY in your environment.")
}
