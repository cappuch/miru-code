package agents

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara-ai/miru-code/internal/installer"
)

// AgentID is a project-local sub-agent template id.
type AgentID string

const (
	AgentClaude   AgentID = "claude"
	AgentCopilot  AgentID = "copilot"
	AgentCursor   AgentID = "cursor"
	AgentGemini   AgentID = "gemini"
	AgentKiro     AgentID = "kiro"
	AgentOpenCode AgentID = "opencode"
)

// AgentIDs lists supported init --agent values.
var AgentIDs = []AgentID{
	AgentClaude, AgentCopilot, AgentCursor, AgentGemini, AgentKiro, AgentOpenCode,
}

//go:embed templates/*.md
var templatesFS embed.FS

var agentNativeTools = map[AgentID]installer.NativeToolNames{
	AgentClaude:   installer.DefaultNativeTools,
	AgentCursor:   installer.DefaultNativeTools,
	AgentCopilot:  {ExplorationDenied: "grep_search, codebase_search, glob, or read_file", Grep: "grep_search", Read: "read_file"},
	AgentGemini:   {ExplorationDenied: "grep_search, glob, or read_file", Grep: "grep_search", Read: "read_file"},
	AgentKiro:     {ExplorationDenied: "built-in grep, file search, or broad file reads", Grep: "Grep", Read: "Read"},
	AgentOpenCode: {ExplorationDenied: "grep, glob, or read-for-exploration", Grep: "grep", Read: "read"},
}

// AgentDestination returns the project-relative destination path.
func AgentDestination(agent AgentID) string {
	baseDir := "." + string(agent)
	if agent == AgentCopilot {
		baseDir = ".github"
	}
	return filepath.Join(baseDir, "agents", "miru-code.md")
}

// LoadAgentTemplate loads frontmatter + search policy body.
func LoadAgentTemplate(agent AgentID) (string, error) {
	data, err := templatesFS.ReadFile("templates/" + string(agent) + ".md")
	if err != nil {
		return "", err
	}
	frontmatter := strings.TrimSpace(string(data))
	native, ok := agentNativeTools[agent]
	if !ok {
		native = installer.DefaultNativeTools
	}
	body := installer.BuildSubagentBody(native)
	return frontmatter + "\n\n" + body + "\n", nil
}

// WriteAgentFile writes a project-local sub-agent file.
func WriteAgentFile(agent AgentID, force bool) (string, error) {
	dest := AgentDestination(agent)
	if !force {
		if _, err := os.Stat(dest); err == nil {
			return "", fmt.Errorf("%s already exists. Run with --force to overwrite.", dest)
		}
	}
	content, err := LoadAgentTemplate(agent)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// IsValidAgentID reports whether id is a known agent.
func IsValidAgentID(id string) bool {
	for _, a := range AgentIDs {
		if string(a) == id {
			return true
		}
	}
	return false
}
