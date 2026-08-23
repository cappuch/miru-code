package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/takara-ai/miru-code/internal/installer"
)

const redirectPrefix =
	"Use Miru MCP instead of built-in search tools (pass project root as `repo`). " +
		"Prefer Miru `locate` for exact literals and `search` for meaning-based questions."

var (
	miruToolRe     = regexp.MustCompile(`(?i)miru`)
	grepToolRe     = regexp.MustCompile(`(?i)^(grep|grep_search|Grep)$`)
	globToolRe     = regexp.MustCompile(`(?i)^(glob|glob_file_search|Glob)$`)
	semanticToolRe = regexp.MustCompile(`(?i)^(SemanticSearch|codebase_search)$`)
	shellToolRe    = regexp.MustCompile(`(?i)^(Shell|Bash|execute_bash|shell)$`)
)

// IsLiteralGrepPattern reports patterns that look like exact literals.
func IsLiteralGrepPattern(pattern string) bool {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return false
	}
	if len(trimmed) >= 2 {
		q := trimmed[0]
		if (q == '\'' || q == '"' || q == '`') && trimmed[len(trimmed)-1] == q {
			return true
		}
	}
	if matched, _ := regexp.MatchString(`^[A-Z][A-Z0-9_]{2,}$`, trimmed); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`(?i)^[a-f0-9-]{36}$`, trimmed); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_.]*$`, trimmed); matched && len(trimmed) <= 48 {
		return true
	}
	return false
}

// IsExplorationShell reports shell commands that look like codebase exploration.
func IsExplorationShell(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false
	}
	if matched, _ := regexp.MatchString(`\b(git|npm|bun|pnpm|yarn|cargo|go|make|cmake|docker|kubectl|pytest|jest|vitest)\b`, cmd); matched {
		return false
	}
	matched, _ := regexp.MatchString(`\b(rg|ripgrep|grep|find|ag|ack|fd|locate)\b`, cmd)
	return matched
}

// HookPayload is a normalized hook event.
type HookPayload struct {
	ToolName       string
	ToolInput      map[string]any
	HookEventName  string
	AgentActionName string
	ToolInfo       map[string]any
}

// GuardDecision is allow/deny for a hook.
type GuardDecision struct {
	Block  bool
	Reason string
}

// IsMcpDescriptorGlob reports MCP descriptor path globs.
func IsMcpDescriptorGlob(pattern string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(pattern, `\`, `/`))
	return strings.Contains(normalized, "/mcps/") ||
		strings.Contains(normalized, "mcps/**/tools") ||
		regexp.MustCompile(`/mcps/[^/]+/tools`).MatchString(normalized)
}

func grepPatternFromInput(toolInput map[string]any) string {
	for _, key := range []string{"pattern", "query", "regex", "needle", "search_term"} {
		if v, ok := toolInput[key]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}

func globPatternFromInput(toolInput map[string]any) string {
	for _, key := range []string{"glob_pattern", "pattern", "glob", "path"} {
		if v, ok := toolInput[key]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}

func suggestedMiruQuery(toolName string, toolInput map[string]any) string {
	if grepToolRe.MatchString(toolName) {
		if p := strings.TrimSpace(grepPatternFromInput(toolInput)); p != "" {
			return p
		}
	}
	if globToolRe.MatchString(toolName) {
		if p := strings.TrimSpace(globPatternFromInput(toolInput)); p != "" {
			return "files matching " + p
		}
	}
	if semanticToolRe.MatchString(toolName) {
		q := strings.TrimSpace(fmt.Sprint(toolInput["query"]))
		if q == "" || q == "<nil>" {
			q = strings.TrimSpace(fmt.Sprint(toolInput["search_term"]))
		}
		if q != "" && q != "<nil>" {
			return q
		}
	}
	if shellToolRe.MatchString(toolName) {
		c := strings.TrimSpace(fmt.Sprint(toolInput["command"]))
		if c == "" || c == "<nil>" {
			c = strings.TrimSpace(fmt.Sprint(toolInput["cmd"]))
		}
		if c != "" && c != "<nil>" {
			return c
		}
	}
	return "your question about how the code works"
}

// SearchGuardBlockReason builds the deny reason string.
func SearchGuardBlockReason(toolName string, toolInput map[string]any) string {
	query := suggestedMiruQuery(toolName, toolInput)
	if grepToolRe.MatchString(toolName) && IsLiteralGrepPattern(grepPatternFromInput(toolInput)) {
		q := strings.Trim(query, "'\"`")
		return fmt.Sprintf("%s Try Miru `locate` with literal %q. %s", redirectPrefix, q, installer.SearchGuardExpandHint)
	}
	return fmt.Sprintf("%s Try Miru `search` with query %q. %s", redirectPrefix, query, installer.SearchGuardExpandHint)
}

// NormalizeHookPayload maps host-specific stdin JSON into HookPayload.
func NormalizeHookPayload(raw map[string]any) HookPayload {
	if agentAction, ok := raw["agent_action_name"].(string); ok {
		toolInfo, _ := raw["tool_info"].(map[string]any)
		if toolInfo == nil {
			toolInfo = map[string]any{}
		}
		if agentAction == "pre_run_command" {
			return HookPayload{
				HookEventName: "windsurf_pre_run_command",
				ToolName:      "Shell",
				ToolInput:     map[string]any{"command": toolInfo["command_line"]},
			}
		}
	}
	toolInput, _ := raw["tool_input"].(map[string]any)
	if toolInput == nil {
		toolInput = map[string]any{}
	}
	toolInfo, _ := raw["tool_info"].(map[string]any)
	toolName, _ := raw["tool_name"].(string)
	event, _ := raw["hook_event_name"].(string)
	agentAction, _ := raw["agent_action_name"].(string)
	return HookPayload{
		ToolName:        toolName,
		ToolInput:       toolInput,
		HookEventName:   event,
		AgentActionName: agentAction,
		ToolInfo:        toolInfo,
	}
}

// EvaluateSearchGuard decides allow/deny for a hook payload.
func EvaluateSearchGuard(payload HookPayload) GuardDecision {
	toolName := payload.ToolName
	toolInput := payload.ToolInput
	if toolInput == nil {
		toolInput = map[string]any{}
	}
	if toolName == "" || miruToolRe.MatchString(toolName) {
		return GuardDecision{Block: false}
	}
	if grepToolRe.MatchString(toolName) {
		return GuardDecision{Block: true, Reason: SearchGuardBlockReason(toolName, toolInput)}
	}
	if globToolRe.MatchString(toolName) {
		if IsMcpDescriptorGlob(globPatternFromInput(toolInput)) {
			return GuardDecision{Block: false}
		}
		return GuardDecision{Block: true, Reason: SearchGuardBlockReason(toolName, toolInput)}
	}
	if semanticToolRe.MatchString(toolName) {
		return GuardDecision{Block: true, Reason: SearchGuardBlockReason(toolName, toolInput)}
	}
	if shellToolRe.MatchString(toolName) {
		command := fmt.Sprint(toolInput["command"])
		if command == "<nil>" {
			command = fmt.Sprint(toolInput["cmd"])
		}
		if IsExplorationShell(command) {
			return GuardDecision{Block: true, Reason: SearchGuardBlockReason(toolName, toolInput)}
		}
	}
	return GuardDecision{Block: false}
}

// HookResponseFormat selects the deny response encoding.
func HookResponseFormat(payload HookPayload) string {
	switch payload.HookEventName {
	case "PreToolUse":
		return "claude"
	case "BeforeTool":
		return "gemini"
	case "preToolUse", "windsurf_pre_run_command":
		return "stderr"
	default:
		return "cursor"
	}
}

// ClaudeHookResponse encodes a Claude deny response.
func ClaudeHookResponse(reason string) string {
	b, _ := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	})
	return string(b)
}

// GeminiHookResponse encodes a Gemini deny response.
func GeminiHookResponse(reason string) string {
	b, _ := json.Marshal(map[string]any{"decision": "deny", "reason": reason})
	return string(b)
}

// CursorHookResponse encodes a Cursor deny response.
func CursorHookResponse(reason string) string {
	b, _ := json.Marshal(map[string]any{
		"permission":    "deny",
		"agent_message": reason,
		"user_message":  "Miru: use MCP search/expand instead of built-in grep/glob.",
	})
	return string(b)
}

// RunSearchGuardFromStdin reads hook JSON from stdin and returns process exit code.
// 0 = allow (or Claude/Gemini deny JSON on stdout), 2 = Cursor/stderr deny.
func RunSearchGuardFromStdin(r io.Reader) int {
	data, err := io.ReadAll(r)
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return 0
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0
	}
	payload := NormalizeHookPayload(raw)
	decision := EvaluateSearchGuard(payload)
	if !decision.Block {
		return 0
	}
	format := HookResponseFormat(payload)
	switch format {
	case "claude":
		fmt.Fprint(os.Stdout, ClaudeHookResponse(decision.Reason))
		return 0
	case "gemini":
		fmt.Fprint(os.Stdout, GeminiHookResponse(decision.Reason))
		return 0
	case "stderr":
		fmt.Fprint(os.Stderr, decision.Reason)
		return 2
	default:
		fmt.Fprint(os.Stdout, CursorHookResponse(decision.Reason))
		return 2
	}
}
