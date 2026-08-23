package installer

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallAction describes a config write outcome.
type InstallAction string

const (
	ActionCreated   InstallAction = "created"
	ActionUpdated   InstallAction = "updated"
	ActionUnchanged InstallAction = "unchanged"
	ActionNotFound  InstallAction = "not-found"
	ActionRemoved   InstallAction = "removed"
	ActionError     InstallAction = "error"
	ActionSkipped   InstallAction = "skipped"
)

// InstallMode is install or uninstall.
type InstallMode string

const (
	ModeInstall   InstallMode = "install"
	ModeUninstall InstallMode = "uninstall"
)

// MiruStart / MiruEnd mark instruction blocks.
const (
	MiruStart = "<!-- miru:start -->"
	MiruEnd   = "<!-- miru:end -->"
)

// McpConfigFormat is json or toml.
type McpConfigFormat string

const (
	FormatJSON McpConfigFormat = "json"
	FormatTOML McpConfigFormat = "toml"
)

// McpConfig describes where/how to write an MCP server entry.
type McpConfig struct {
	Path      string
	Key       string
	MemberKey string
	Entry     map[string]any
	Format    McpConfigFormat
}

// NativeSkillVendor is a vendor that uses native skill dirs instead of ~/.agents/skills.
type NativeSkillVendor string

const (
	NativeSkillClaude NativeSkillVendor = "claude"
	NativeSkillKiro   NativeSkillVendor = "kiro"
)

// AgentTarget is one coding agent integration target.
type AgentTarget struct {
	ID               string
	DisplayName      string
	Binary           string
	ConfigDir        string
	MCP              *McpConfig
	InstructionsPath string
	CursorRulesPath  string
	HooksPath        string
	HooksFormat      string
	SubagentPath     string
	SubagentID       string
	// CavemanSkillPath is the on-demand Caveman Agent Skill (…/skills/caveman/SKILL.md), or empty if unsupported.
	CavemanSkillPath string
	// SteSkillDir is the on-demand STE skill directory (…/skills/ste/), or empty if unsupported.
	SteSkillDir string
}

// ResolveMiruExecutable returns the absolute path to the miru binary.
// NEVER returns bunx — agent MCP configs must invoke the Go binary directly.
func ResolveMiruExecutable() (string, error) {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return resolved, nil
		}
		return exe, nil
	}
	if path, err := exec.LookPath("miru"); err == nil {
		return path, nil
	}
	return "", os.ErrNotExist
}

// StdioServerConfig builds a stdio MCP entry pointing at the Go miru binary.
func StdioServerConfig(withType bool) map[string]any {
	exe, err := ResolveMiruExecutable()
	if err != nil {
		exe = "miru"
	}
	entry := map[string]any{
		"command": exe,
		"args":    []any{},
	}
	if withType {
		entry["type"] = "stdio"
	}
	return entry
}

// OpenCodeServerConfig builds the OpenCode local MCP entry.
func OpenCodeServerConfig() map[string]any {
	exe, err := ResolveMiruExecutable()
	if err != nil {
		exe = "miru"
	}
	return map[string]any{
		"command": []any{exe},
		"type":    "local",
		"enabled": true,
	}
}

// CodexTOMLBlock returns the Codex mcp_servers.miru TOML block.
func CodexTOMLBlock(benchmark bool) string {
	exe, err := ResolveMiruExecutable()
	if err != nil {
		exe = "miru"
	}
	// Escape backslashes for TOML strings on Windows.
	exe = strings.ReplaceAll(exe, `\`, `\\`)
	if benchmark {
		return "[mcp_servers.miru]\ncommand = \"" + exe + "\"\nargs = [\"--benchmark\"]\n"
	}
	return "[mcp_servers.miru]\ncommand = \"" + exe + "\"\nargs = []\n"
}

// Instructions is the marked instructions block.
var Instructions = MiruStart + "\n" + InstructionsMarkdown + "\n" + MiruEnd + "\n"

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return os.Getenv("HOME")
	}
	return h
}

func skillMD(home, rootDir, skillName string) string {
	return filepath.Join(home, rootDir, "skills", skillName, "SKILL.md")
}

func skillDir(home, rootDir, skillName string) string {
	return filepath.Join(home, rootDir, "skills", skillName)
}

// CopilotHomeDir is the shared Copilot / VS Code / Visual Studio config root.
func CopilotHomeDir(home string) string {
	return filepath.Join(home, ".copilot")
}

// AgentsCavemanSkillPath is the cross-agent Caveman skill path under ~/.agents/skills.
func AgentsCavemanSkillPath(home string) string {
	return skillMD(home, ".agents", "caveman")
}

// NativeCavemanSkillPath is the Claude Code / Kiro native Caveman skill path.
func NativeCavemanSkillPath(home string, vendor NativeSkillVendor) string {
	return skillMD(home, "."+string(vendor), "caveman")
}

// AgentsSteSkillDir is the shared STE skill directory under ~/.agents/skills/ste.
func AgentsSteSkillDir(home string) string {
	return skillDir(home, ".agents", "ste")
}

// NativeSteSkillDir is the Claude Code / Kiro native STE skill directory.
func NativeSteSkillDir(home string, vendor NativeSkillVendor) string {
	return skillDir(home, "."+string(vendor), "ste")
}

// OpencodeConfigDir is the OpenCode config root ($XDG_CONFIG_HOME/opencode or ~/.config/opencode).
func OpencodeConfigDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	return filepath.Join(home, ".config", "opencode")
}

func copilotHooksPath(home string) string {
	return filepath.Join(CopilotHomeDir(home), "hooks", "miru-search.json")
}

func opencodePluginPath(home string) string {
	return filepath.Join(OpencodeConfigDir(home), "plugins", "miru-search-guard.ts")
}

func opencodeMcpPath() string {
	base := OpencodeConfigDir(homeDir())
	jsonc := filepath.Join(base, "opencode.jsonc")
	json := filepath.Join(base, "opencode.json")
	if _, err := os.Stat(jsonc); err == nil {
		return jsonc
	}
	if _, err := os.Stat(json); err == nil {
		return json
	}
	return jsonc
}

func vscodeMcpPath() string {
	h := homeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(h, "Library", "Application Support", "Code", "User", "mcp.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = h
		}
		return filepath.Join(appData, "Code", "User", "mcp.json")
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(h, ".config")
		}
		return filepath.Join(xdg, "Code", "User", "mcp.json")
	}
}

func visualStudioMcpPath() string {
	profile := os.Getenv("USERPROFILE")
	if profile == "" {
		profile = homeDir()
	}
	return filepath.Join(profile, ".mcp.json")
}

func jsonMcp(path, key string, entry map[string]any) *McpConfig {
	return &McpConfig{Path: path, Key: key, MemberKey: "miru", Entry: entry, Format: FormatJSON}
}

// AgentTargets returns all installer agent targets (paths resolved at call time).
func AgentTargets() []AgentTarget {
	h := homeDir()
	sharedCaveman := AgentsCavemanSkillPath(h)
	sharedSte := AgentsSteSkillDir(h)
	opencodeDir := OpencodeConfigDir(h)
	return []AgentTarget{
		{
			ID: "claude", DisplayName: "Claude Code", Binary: "claude",
			ConfigDir: filepath.Join(h, ".claude"),
			MCP: jsonMcp(filepath.Join(h, ".claude.json"), "mcpServers", StdioServerConfig(true)),
			InstructionsPath: filepath.Join(h, ".claude", "CLAUDE.md"),
			HooksPath: filepath.Join(h, ".claude", "settings.json"), HooksFormat: "claude",
			SubagentPath: filepath.Join(h, ".claude", "agents", "miru-code.md"), SubagentID: "claude",
			CavemanSkillPath: NativeCavemanSkillPath(h, NativeSkillClaude),
			SteSkillDir:      NativeSteSkillDir(h, NativeSkillClaude),
		},
		{
			ID: "cursor", DisplayName: "Cursor", Binary: "cursor",
			ConfigDir: filepath.Join(h, ".cursor"),
			MCP: jsonMcp(filepath.Join(h, ".cursor", "mcp.json"), "mcpServers", StdioServerConfig(true)),
			CursorRulesPath: filepath.Join(h, ".cursor", "rules", "miru-code.mdc"),
			HooksPath: filepath.Join(h, ".cursor", "hooks.json"), HooksFormat: "cursor",
			SubagentPath: filepath.Join(h, ".cursor", "agents", "miru-code.md"), SubagentID: "cursor",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "gemini", DisplayName: "Gemini CLI", Binary: "gemini",
			ConfigDir: filepath.Join(h, ".gemini"),
			MCP: jsonMcp(filepath.Join(h, ".gemini", "settings.json"), "mcpServers", StdioServerConfig(true)),
			InstructionsPath: filepath.Join(h, ".gemini", "GEMINI.md"),
			HooksPath: filepath.Join(h, ".gemini", "settings.json"), HooksFormat: "gemini",
			SubagentPath: filepath.Join(h, ".gemini", "agents", "miru-code.md"), SubagentID: "gemini",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "kiro", DisplayName: "Kiro", Binary: "kiro",
			ConfigDir: filepath.Join(h, ".kiro"),
			MCP: jsonMcp(filepath.Join(h, ".kiro", "settings", "mcp.json"), "mcpServers", StdioServerConfig(true)),
			InstructionsPath: filepath.Join(h, ".kiro", "steering", "miru.md"),
			HooksPath: filepath.Join(h, ".kiro", "settings", "hooks.json"), HooksFormat: "kiro",
			SubagentPath: filepath.Join(h, ".kiro", "agents", "miru-code.md"), SubagentID: "kiro",
			CavemanSkillPath: NativeCavemanSkillPath(h, NativeSkillKiro),
			SteSkillDir:      NativeSteSkillDir(h, NativeSkillKiro),
		},
		{
			ID: "opencode", DisplayName: "OpenCode", Binary: "opencode",
			ConfigDir: opencodeDir,
			MCP: jsonMcp(opencodeMcpPath(), "mcp", OpenCodeServerConfig()),
			InstructionsPath: filepath.Join(opencodeDir, "AGENTS.md"),
			HooksPath:        opencodePluginPath(h),
			HooksFormat:      "opencode",
			SubagentPath:     filepath.Join(opencodeDir, "agents", "miru-code.md"), SubagentID: "opencode",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "copilot", DisplayName: "GitHub Copilot",
			ConfigDir: filepath.Join(h, ".config", "github-copilot"),
			MCP: jsonMcp(filepath.Join(CopilotHomeDir(h), "mcp-config.json"), "mcpServers", StdioServerConfig(false)),
			HooksPath:    copilotHooksPath(h),
			HooksFormat:  "vscode",
			SubagentPath: filepath.Join(CopilotHomeDir(h), "agents", "miru-code.agent.md"), SubagentID: "copilot",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "codex", DisplayName: "Codex", Binary: "codex",
			ConfigDir: filepath.Join(h, ".codex"),
			MCP: &McpConfig{Path: filepath.Join(h, ".codex", "config.toml"), Key: "mcp_servers", MemberKey: "miru", Entry: map[string]any{}, Format: FormatTOML},
			InstructionsPath: filepath.Join(h, ".codex", "AGENTS.md"),
			HooksPath:        filepath.Join(h, ".codex", "hooks.json"), HooksFormat: "claude",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "vscode", DisplayName: "VS Code", Binary: "code",
			MCP: jsonMcp(vscodeMcpPath(), "servers", StdioServerConfig(true)),
			HooksPath:        copilotHooksPath(h),
			HooksFormat:      "vscode",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "windsurf", DisplayName: "Windsurf / Devin Desktop", Binary: "windsurf",
			ConfigDir: filepath.Join(h, ".codeium", "windsurf"),
			HooksPath:        filepath.Join(h, ".codeium", "windsurf", "hooks.json"),
			HooksFormat:      "windsurf",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
		{
			ID: "visualstudio", DisplayName: "Visual Studio",
			MCP: jsonMcp(visualStudioMcpPath(), "servers", StdioServerConfig(true)),
			HooksPath:        copilotHooksPath(h),
			HooksFormat:      "vscode",
			CavemanSkillPath: sharedCaveman,
			SteSkillDir:      sharedSte,
		},
	}
}

// IsCopilotInstalled reports whether Copilot-specific config exists.
func IsCopilotInstalled(home string) bool {
	if _, err := os.Stat(filepath.Join(home, ".config", "github-copilot")); err == nil {
		return true
	}
	copilot := CopilotHomeDir(home)
	if _, err := os.Stat(filepath.Join(copilot, "mcp-config.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(copilot, "agents")); err == nil {
		return true
	}
	return false
}

// IsAgentDetected reports whether an agent appears installed.
func IsAgentDetected(agent AgentTarget) bool {
	if agent.ID == "windsurf" {
		if agent.ConfigDir != "" {
			if _, err := os.Stat(agent.ConfigDir); err == nil {
				return true
			}
		}
		if agent.Binary != "" {
			if _, err := exec.LookPath(agent.Binary); err == nil {
				return true
			}
		}
		return false
	}
	if agent.ID == "copilot" {
		return IsCopilotInstalled(homeDir())
	}
	if agent.Binary != "" {
		if _, err := exec.LookPath(agent.Binary); err == nil {
			return true
		}
	}
	if agent.ConfigDir != "" {
		if _, err := os.Stat(agent.ConfigDir); err == nil {
			return true
		}
	}
	return false
}

// PrettyJSON marshals with indent.
func PrettyJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
