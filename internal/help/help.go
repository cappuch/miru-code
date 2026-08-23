package help

import (
	"fmt"
	"os"
	"strings"

	"github.com/takara-ai/miru-code/internal/agents"
	"github.com/takara-ai/miru-code/internal/cliui"
	"github.com/takara-ai/miru-code/internal/concurrency"
	"github.com/takara-ai/miru-code/internal/embed"
	"github.com/takara-ai/miru-code/internal/env"
)

// PrintMainHelp prints the top-level help.
func PrintMainHelp() {
	cliui.Header()
	cliui.Section("Usage")
	cliui.WriteStdout("  miru                         Start MCP server (stdio)")
	cliui.WriteStdout("  miru <command> [options]")
	cliui.Divider("", 0, os.Stdout)
	cliui.Section("Commands")
	cliui.CommandRow("search", "Hybrid search over a codebase")
	cliui.CommandRow("locate", "Exact substring location in the index")
	cliui.CommandRow("expand", "Adjacent chunks in the same file as a hit")
	cliui.CommandRow("find-related", "Find chunks related to a file:line")
	cliui.CommandRow("setup", "Authenticate with Takara and save local credentials")
	cliui.CommandRow("install", "Configure miru across coding agents")
	cliui.CommandRow("uninstall", "Remove miru agent configuration")
	cliui.CommandRow("benchmark", "Turn MCP benchmark mode on or off")
	cliui.CommandRow("init", "Write a project-local sub-agent file")
	cliui.CommandRow("clear", "Remove cached index for a path")
	cliui.CommandRow("help", "Show help for a command")
	cliui.Divider("", 0, os.Stdout)
	cliui.Section("Quick start")
	cliui.WriteStdout("  miru setup && miru install")
	cliui.WriteStdout(`  miru search "auth middleware" ./src`)
	cliui.WriteStdout("")
	cliui.Hint("miru <command> -h  ·  miru -v for version")
	cliui.WriteStdout("")
}

// PrintEnvironmentHelp prints Takara/env variable help (without SageMaker).
func PrintEnvironmentHelp() {
	cliui.Section("Environment")
	cliui.WriteStdout("  " + env.TakaraAPIKeyEnv)
	cliui.WriteStdout("      Takara bearer token for embeddings")
	cliui.WriteStdout("  MIRU_OPENAI_BASE_URL")
	cliui.WriteStdout("      Default: " + embed.DefaultEmbeddingBaseURL)
	cliui.WriteStdout("  MIRU_OPENAI_EMBEDDING_MODEL")
	cliui.WriteStdout("      Default: " + embed.DefaultEmbeddingModel)
	cliui.WriteStdout("  MIRU_CONCURRENCY")
	cliui.WriteStdout(fmt.Sprintf("      Parallel workers (default: CPUs − %d)", concurrency.DefaultReserveCores))
	cliui.WriteStdout("  MIRU_TOKENIZER_JSON")
	cliui.WriteStdout("      Path to tokenizer.json")
	cliui.WriteStdout("")
}

// PrintSageMakerHelp prints self-hosted SageMaker env and setup hints.
func PrintSageMakerHelp() {
	cliui.Section("Self-hosted (AWS SageMaker)")
	cliui.WriteStdout("  MIRU_SAGEMAKER_ENDPOINT_ARN")
	cliui.WriteStdout("      arn:aws:sagemaker:<region>:<account-id>:endpoint/<name> — set this to")
	cliui.WriteStdout("      bypass Takara entirely and embed via your own SageMaker endpoint.")
	cliui.WriteStdout("  MIRU_SAGEMAKER_ENDPOINT_NAME / MIRU_SAGEMAKER_REGION")
	cliui.WriteStdout("      Alternative to the ARN when you'd rather name the endpoint + region")
	cliui.WriteStdout("      directly (falls back to AWS_REGION / AWS_DEFAULT_REGION).")
	cliui.WriteStdout("  MIRU_SAGEMAKER_NORMALIZE / MIRU_SAGEMAKER_TRUNCATE")
	cliui.WriteStdout("      Default: true")
	cliui.WriteStdout("  MIRU_SAGEMAKER_TRUNCATION_DIRECTION")
	cliui.WriteStdout(`      "Left" | "Right" (default: Right)`)
	cliui.WriteStdout("  MIRU_SAGEMAKER_PROMPT_NAME")
	cliui.WriteStdout("      Optional prompt_name passed to the endpoint")
	cliui.WriteStdout("  AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN / AWS_PROFILE")
	cliui.WriteStdout("      Standard AWS credential resolution — nothing Miru-specific to set")
	cliui.WriteStdout("")
	cliui.Hint("Enterprise self-hosted setup: docs/self-hosted-sagemaker.md")
	cliui.Hint("Then: miru setup --sagemaker --arn <arn> --profile <name>")
	cliui.Hint("Setup confirms auth, saves SageMaker config, and removes any stored Takara API key.")
	cliui.WriteStdout("")
}

// PrintEnvHelp prints environment + SageMaker help.
func PrintEnvHelp() {
	PrintEnvironmentHelp()
	PrintSageMakerHelp()
}

// PrintFullHelp prints main + env help.
func PrintFullHelp() {
	PrintMainHelp()
	PrintEnvHelp()
}

// PrintCommandHelp prints help for one command.
func PrintCommandHelp(command string) {
	switch command {
	case "env", "environment":
		PrintEnvironmentHelp()
	case "search":
		cliui.CommandHeader("search", "Hybrid semantic + keyword search.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru search <query> [path] [options]")
		cliui.Section("Options")
		cliui.WriteStdout("  -k, --top-k N       Number of results (default: 5)")
		cliui.WriteStdout("  --content TYPE      code | docs | config | all (default: code config)")
		cliui.WriteStdout("  --json              JSON output (default when piped)")
		cliui.WriteStdout("")
	case "locate":
		cliui.CommandHeader("locate", "Exact substring location over the Miru index.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru locate <literal> [path] [options]")
		cliui.Section("Options")
		cliui.WriteStdout("  --mode MODE         count | locations | lines (default: lines)")
		cliui.WriteStdout("  --limit N           Optional hit cap")
		cliui.WriteStdout("  --ignore-case       Case-insensitive match")
		cliui.WriteStdout("  --match-variants    Also match identifier case variants")
		cliui.WriteStdout("  --include GLOB      Only search matching files")
		cliui.WriteStdout("  --exclude GLOB      Skip matching files")
		cliui.WriteStdout("  --context N         Context lines (mode=lines)")
		cliui.WriteStdout("  --content TYPE      code | docs | config | all")
		cliui.WriteStdout("  --json              JSON output")
		cliui.WriteStdout("")
	case "expand":
		cliui.CommandHeader("expand", "More context in the same file as a search hit.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru expand <file> <line> [path] [--before N] [--after N]")
		cliui.WriteStdout("")
	case "find-related":
		cliui.CommandHeader("find-related", "Semantic neighbors of a file location.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru find-related <file> <line> [path] [options]")
		cliui.WriteStdout("")
	case "setup":
		cliui.CommandHeader("setup", "Authenticate with Takara or self-hosted SageMaker credentials.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru setup [--device] [--key TOKEN] [--force] [--clear]")
		cliui.WriteStdout("  miru setup --sagemaker --arn ENDPOINT_ARN --profile NAME")
		cliui.Section("Options (Takara)")
		cliui.WriteStdout("  --device            Start device-code login (default when interactive)")
		cliui.WriteStdout("  --key, -k TOKEN     Non-interactive: store a bearer token directly")
		cliui.WriteStdout("  --force             Replace existing stored credentials")
		cliui.WriteStdout("  --clear             Remove stored credentials")
		cliui.Section("Options (SageMaker)")
		cliui.WriteStdout("  --sagemaker         Switch setup to self-hosted SageMaker mode")
		cliui.WriteStdout("  --arn ARN           Endpoint ARN (implies --sagemaker)")
		cliui.WriteStdout("  --profile NAME      AWS profile to inherit credentials from")
		cliui.WriteStdout("")
		cliui.Hint("Enterprise guide: docs/self-hosted-sagemaker.md (Marketplace + invoke-user runbook).")
		cliui.Hint("Takara and SageMaker are mutually exclusive — setup for one removes the other.")
		cliui.Hint("SageMaker setup invokes the endpoint once (auth + embedding check), then saves.")
		cliui.Hint("Then: miru setup --sagemaker --arn <arn> --profile miru")
		cliui.Hint("miru setup --sagemaker -h for SageMaker environment variables.")
		cliui.WriteStdout("")
	case "install":
		cliui.CommandHeader("install", "Interactive global agent setup.")
		cliui.WriteStdout("Configures MCP server, instructions, and sub-agent files.")
		cliui.WriteStdout("Non-interactive: MIRU_INSTALL_AGENTS=all miru install --yes")
		cliui.WriteStdout("MCP command is the Go miru binary absolute path (never bunx).")
		cliui.WriteStdout("")
	case "uninstall":
		cliui.CommandHeader("uninstall", "Remove miru configuration from agents.")
		cliui.WriteStdout("")
	case "benchmark":
		cliui.CommandHeader("benchmark", "Toggle MCP benchmark mode on installed agents.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru benchmark on|off|status|clear")
		cliui.WriteStdout("(Go port: status reports not fully wired; prefer install --benchmark.)")
		cliui.WriteStdout("")
	case "init":
		cliui.CommandHeader("init", "Project-local sub-agent file.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru init --agent AGENT [--force]")
		cliui.Section("Agents")
		ids := make([]string, len(agents.AgentIDs))
		for i, id := range agents.AgentIDs {
			ids[i] = string(id)
		}
		cliui.WriteStdout("  " + strings.Join(ids, ", "))
		cliui.WriteStdout("")
	case "clear":
		cliui.CommandHeader("clear", "Drop the on-disk index cache.")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru clear [path]")
		cliui.WriteStdout("")
	case "mcp":
		cliui.CommandHeader("mcp", "Stdio MCP server (default with no subcommand).")
		cliui.Section("Usage")
		cliui.WriteStdout("  miru [--ref BRANCH] [--content TYPE ...] [--benchmark]")
		cliui.WriteStdout("")
	default:
		cliui.Fail("Unknown command: " + command)
		cliui.WriteStdout("")
		PrintMainHelp()
		os.Exit(1)
	}
}

// FormatUnknownAgent formats an unknown agent error.
func FormatUnknownAgent(agent string) string {
	ids := make([]string, len(agents.AgentIDs))
	for i, id := range agents.AgentIDs {
		ids[i] = string(id)
	}
	return fmt.Sprintf("Unknown agent %q. Choose one of: %s", agent, strings.Join(ids, ", "))
}
