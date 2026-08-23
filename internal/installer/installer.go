package installer

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/takara-ai/miru-code/internal/cliui"
	"github.com/takara-ai/miru-code/internal/installer/stylepacks"
	"github.com/takara-ai/miru-code/internal/installer/stylepacks/ste"
	"github.com/takara-ai/miru-code/internal/setup"
)

//go:embed templates/*.md
var agentTemplatesFS embed.FS

// WriteResult is the outcome of applying one integration.
type WriteResult struct {
	Path   string
	Action InstallAction
	Note   string
}

// ApplyCtx holds shared context for one install/uninstall pass.
type ApplyCtx struct {
	SelectedAgents   []AgentTarget
	CavemanSeenPaths map[string]struct{}
	SteSeenPaths     map[string]struct{}
	AllAgents        []AgentTarget
	IsDetected       func(AgentTarget) bool
}

// IntegrationID identifies an install integration.
type IntegrationID string

const (
	IntegrationMCP          IntegrationID = "mcp"
	IntegrationInstructions IntegrationID = "instructions"
	IntegrationSubagent     IntegrationID = "subagent"
	IntegrationHooks        IntegrationID = "hooks"
	IntegrationRules        IntegrationID = "rules"
	IntegrationCaveman      IntegrationID = "caveman"
	IntegrationSTE          IntegrationID = "ste"
)

type integration struct {
	ID             IntegrationID
	Label          string
	Description    string
	Experimental   bool
	DefaultChecked bool
	PlanPath       func(AgentTarget) string
	Apply          func(AgentTarget, InstallMode, *ApplyCtx) (*WriteResult, error)
}

func applyMCP(agent AgentTarget, mode InstallMode, _ *ApplyCtx) (*WriteResult, error) {
	if agent.MCP == nil {
		return nil, nil
	}
	mcp := agent.MCP
	if mcp.Format == FormatTOML {
		var action InstallAction
		var err error
		if mode == ModeInstall {
			action, err = MergeTomlBlock(mcp.Path)
		} else {
			action, err = RemoveTomlBlock(mcp.Path)
		}
		return &WriteResult{Path: mcp.Path, Action: action}, err
	}
	if mode != ModeInstall {
		action, err := RemoveJSONMember(mcp.Path, mcp.Key, mcp.MemberKey)
		return &WriteResult{Path: mcp.Path, Action: action}, err
	}
	entry := mcp.Entry
	if entry == nil {
		entry = StdioServerConfig(true)
	}
	action, err := MergeJSONMember(mcp.Path, mcp.Key, mcp.MemberKey, entry)
	return &WriteResult{Path: mcp.Path, Action: action}, err
}

func applyInstructions(agent AgentTarget, mode InstallMode, _ *ApplyCtx) (*WriteResult, error) {
	if agent.InstructionsPath == "" {
		return nil, nil
	}
	var action InstallAction
	var err error
	if mode == ModeInstall {
		action, err = ReplaceOrAppendMarked(agent.InstructionsPath, Instructions)
	} else {
		action, err = RemoveMarked(agent.InstructionsPath)
	}
	return &WriteResult{Path: agent.InstructionsPath, Action: action}, err
}

func applyCursorRules(agent AgentTarget, mode InstallMode, _ *ApplyCtx) (*WriteResult, error) {
	if agent.CursorRulesPath == "" {
		return nil, nil
	}
	path := agent.CursorRulesPath
	if mode == ModeUninstall {
		if _, err := os.Stat(path); err != nil {
			return &WriteResult{Path: path, Action: ActionNotFound}, nil
		}
		_ = os.Remove(path)
		return &WriteResult{Path: path, Action: ActionRemoved}, nil
	}
	existed := false
	if _, err := os.Stat(path); err == nil {
		existed = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &WriteResult{Path: path, Action: ActionError}, err
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(CursorRulesMDC)+"\n"), 0o644); err != nil {
		return &WriteResult{Path: path, Action: ActionError}, err
	}
	if existed {
		return &WriteResult{Path: path, Action: ActionUpdated}, nil
	}
	return &WriteResult{Path: path, Action: ActionCreated}, nil
}

func applySubagent(agent AgentTarget, mode InstallMode, _ *ApplyCtx) (*WriteResult, error) {
	if agent.SubagentPath == "" || agent.SubagentID == "" {
		return nil, nil
	}
	dest := agent.SubagentPath
	if mode == ModeUninstall {
		if _, err := os.Stat(dest); err != nil {
			return &WriteResult{Path: dest, Action: ActionNotFound}, nil
		}
		_ = os.Remove(dest)
		return &WriteResult{Path: dest, Action: ActionRemoved}, nil
	}
	content, err := loadInstalledAgentTemplate(agent.SubagentID)
	if err != nil {
		return &WriteResult{Path: dest, Action: ActionError}, err
	}
	existed := false
	if _, err := os.Stat(dest); err == nil {
		existed = true
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return &WriteResult{Path: dest, Action: ActionError}, err
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return &WriteResult{Path: dest, Action: ActionError}, err
	}
	if existed {
		return &WriteResult{Path: dest, Action: ActionUpdated}, nil
	}
	return &WriteResult{Path: dest, Action: ActionCreated}, nil
}

func loadInstalledAgentTemplate(id string) (string, error) {
	data, err := agentTemplatesFS.ReadFile("templates/" + id + ".md")
	if err != nil {
		return "", err
	}
	frontmatter := strings.TrimSpace(string(data))
	native := DefaultNativeTools
	switch id {
	case "copilot":
		native = NativeToolNames{ExplorationDenied: "grep_search, codebase_search, glob, or read_file", Grep: "grep_search", Read: "read_file"}
	case "gemini":
		native = NativeToolNames{ExplorationDenied: "grep_search, glob, or read_file", Grep: "grep_search", Read: "read_file"}
	case "kiro":
		native = NativeToolNames{ExplorationDenied: "built-in grep, file search, or broad file reads", Grep: "Grep", Read: "Read"}
	case "opencode":
		native = NativeToolNames{ExplorationDenied: "grep, glob, or read-for-exploration", Grep: "grep", Read: "read"}
	}
	return frontmatter + "\n\n" + BuildSubagentBody(native) + "\n", nil
}

func applyHooks(agent AgentTarget, mode InstallMode, _ *ApplyCtx) (*WriteResult, error) {
	if agent.HooksPath == "" || agent.HooksFormat == "" {
		return nil, nil
	}
	// Simplified: write/remove a marker note file for hook-guard command.
	path := agent.HooksPath
	exe, err := ResolveMiruExecutable()
	if err != nil {
		exe = "miru"
	}
	command := exe + " hook-guard"
	if mode == ModeUninstall {
		action, err := removeMiruHooks(path)
		return &WriteResult{Path: path, Action: action}, err
	}
	action, err := mergeMiruHooks(path, agent.HooksFormat, command)
	return &WriteResult{Path: path, Action: action}, err
}

func mergeMiruHooks(path, format, command string) (InstallAction, error) {
	existed := false
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		parsed, ok := parseJSONObject(string(data))
		if !ok {
			return ActionError, nil
		}
		root = parsed
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	entry := map[string]any{"command": command, "matcher": "Grep|Glob|Shell|SemanticSearch"}
	switch format {
	case "claude":
		hooks["PreToolUse"] = []any{
			map[string]any{
				"matcher": "Grep|Glob|Bash",
				"hooks": []any{
					map[string]any{"type": "command", "command": command, "statusMessage": "Miru search policy"},
				},
			},
		}
	case "cursor":
		hooks["preToolUse"] = []any{entry}
	case "gemini":
		hooks["BeforeTool"] = []any{
			map[string]any{"type": "command", "command": command, "matcher": "grep_search|glob_file_search|codebase_search", "timeout": 15000},
		}
	default:
		hooks["preToolUse"] = []any{entry}
	}
	root["hooks"] = hooks
	data, err := PrettyJSON(root)
	if err != nil {
		return ActionError, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(ensureTrailingNewline(string(data))), 0o644); err != nil {
		return ActionError, err
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}

func removeMiruHooks(path string) (InstallAction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionNotFound, nil
		}
		return ActionError, err
	}
	parsed, ok := parseJSONObject(string(data))
	if !ok {
		return ActionError, nil
	}
	hooks, _ := parsed["hooks"].(map[string]any)
	if hooks == nil {
		return ActionNotFound, nil
	}
	changed := false
	for key, val := range hooks {
		arr, ok := val.([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				filtered = append(filtered, item)
				continue
			}
			cmd := fmtSprint(m["command"])
			if strings.Contains(cmd, "hook-guard") || strings.Contains(cmd, "miru-search") {
				changed = true
				continue
			}
			// nested hooks arrays (claude)
			if nested, ok := m["hooks"].([]any); ok {
				kept := make([]any, 0, len(nested))
				for _, h := range nested {
					hm, _ := h.(map[string]any)
					if hm != nil && strings.Contains(fmtSprint(hm["command"]), "hook-guard") {
						changed = true
						continue
					}
					kept = append(kept, h)
				}
				if len(kept) == 0 {
					changed = true
					continue
				}
				m["hooks"] = kept
			}
			filtered = append(filtered, m)
		}
		if len(filtered) == 0 {
			delete(hooks, key)
		} else {
			hooks[key] = filtered
		}
	}
	if !changed {
		return ActionNotFound, nil
	}
	if len(hooks) == 0 {
		delete(parsed, "hooks")
	} else {
		parsed["hooks"] = hooks
	}
	if len(parsed) == 0 {
		_ = os.Remove(path)
		return ActionRemoved, nil
	}
	out, err := PrettyJSON(parsed)
	if err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(ensureTrailingNewline(string(out))), 0o644); err != nil {
		return ActionError, err
	}
	return ActionRemoved, nil
}

func fmtSprint(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(
		strings.ReplaceAll(strings.ReplaceAll(toString(v), "<nil>", ""), "\"", ""),
	)
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := PrettyJSON(t)
		return string(b)
	}
}

func defaultApplyCtx(agent AgentTarget) *ApplyCtx {
	return &ApplyCtx{
		SelectedAgents:   []AgentTarget{agent},
		CavemanSeenPaths: map[string]struct{}{},
		SteSeenPaths:     map[string]struct{}{},
	}
}

func withCodexSkillsFeature(agent AgentTarget, result *WriteResult) (*WriteResult, error) {
	if agent.ID != "codex" || agent.MCP == nil {
		return result, nil
	}
	featureAction, err := EnsureCodexSkillsFeature(agent.MCP.Path)
	if err != nil {
		return result, err
	}
	if featureAction == ActionUnchanged {
		return result, nil
	}
	skillsNote := "enabled [features] skills = true in config.toml"
	if result.Action == ActionUnchanged {
		note := skillsNote
		if result.Note != "" {
			note = result.Note + "; " + skillsNote
		}
		return &WriteResult{Path: result.Path, Action: ActionUpdated, Note: note}, nil
	}
	note := "also sets [features] skills = true in config.toml"
	if result.Note != "" {
		note = result.Note + "; also sets [features] skills = true in config.toml"
	}
	return &WriteResult{Path: result.Path, Action: result.Action, Note: note}, nil
}

func applyCaveman(agent AgentTarget, mode InstallMode, ctx *ApplyCtx) (*WriteResult, error) {
	if ctx == nil {
		ctx = defaultApplyCtx(agent)
	}
	path := agent.CavemanSkillPath
	if path == "" {
		return nil, nil
	}

	skillDir := filepath.Dir(path)
	allAgents := ctx.AllAgents
	if allAgents == nil {
		allAgents = AgentTargets()
	}
	shared := IsSharedCavemanPath(path, allAgents)
	sharedCtx := SharedSkillCtx{
		SelectedAgents: ctx.SelectedAgents,
		AllAgents:      ctx.AllAgents,
		IsDetected:     ctx.IsDetected,
	}

	if mode == ModeUninstall {
		if shared {
			decision, err := ResolveSharedCavemanUninstall(path, skillDir, agent, sharedCtx)
			if err != nil {
				return nil, err
			}
			if decision.Kind == "defer" || decision.Kind == "keep" {
				return &WriteResult{Path: path, Action: ActionUnchanged, Note: decision.Note}, nil
			}
		}
		action, err := RemoveCavemanSkillFiles(path, skillDir)
		return &WriteResult{Path: path, Action: action}, err
	}

	if shared {
		if err := EnsureSharedCavemanOwnersOnInstall(skillDir, path, agent, sharedCtx); err != nil {
			return &WriteResult{Path: path, Action: ActionError}, err
		}
	}

	if _, seen := ctx.CavemanSeenPaths[path]; seen {
		return withCodexSkillsFeature(agent, &WriteResult{
			Path: path, Action: ActionUnchanged, Note: "shared skill path already written",
		})
	}
	ctx.CavemanSeenPaths[path] = struct{}{}

	existed := false
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		if string(data) == stylepacks.CavemanSkillMD {
			return withCodexSkillsFeature(agent, &WriteResult{Path: path, Action: ActionUnchanged})
		}
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return &WriteResult{Path: path, Action: ActionError}, err
	}
	if err := os.WriteFile(path, []byte(stylepacks.CavemanSkillMD), 0o644); err != nil {
		return &WriteResult{Path: path, Action: ActionError}, err
	}
	action := ActionCreated
	if existed {
		action = ActionUpdated
	}
	return withCodexSkillsFeature(agent, &WriteResult{Path: path, Action: action})
}

func stePackMatches(skillDir string) (bool, error) {
	skillPath := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return false, nil
	}
	if string(data) != ste.SteSkillMD {
		return false, nil
	}
	for _, file := range ste.SteReferenceFiles {
		refPath := filepath.Join(skillDir, file.RelativePath)
		refData, err := os.ReadFile(refPath)
		if err != nil {
			return false, nil
		}
		if string(refData) != file.Content {
			return false, nil
		}
	}
	return true, nil
}

func removeSteSkillFiles(skillDir, skillPath string) (InstallAction, error) {
	removedReference := false
	for _, file := range ste.SteReferenceFiles {
		referencePath := filepath.Join(skillDir, file.RelativePath)
		if _, err := os.Stat(referencePath); err == nil {
			if err := os.Remove(referencePath); err != nil {
				return ActionError, err
			}
			removedReference = true
		}
	}
	_ = os.Remove(filepath.Join(skillDir, "references"))

	skillAction, err := RemoveSkillMdAndOwners(skillPath, skillDir)
	if err != nil {
		return ActionError, err
	}
	if removedReference && skillAction == ActionNotFound {
		return ActionRemoved, nil
	}
	return skillAction, nil
}

func applySte(agent AgentTarget, mode InstallMode, ctx *ApplyCtx) (*WriteResult, error) {
	if ctx == nil {
		ctx = defaultApplyCtx(agent)
	}
	skillDir := agent.SteSkillDir
	if skillDir == "" {
		return nil, nil
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	allAgents := ctx.AllAgents
	if allAgents == nil {
		allAgents = AgentTargets()
	}
	shared := IsSharedSkillPath(skillPath, allAgents, steSkillPathOf)
	sharedCtx := SharedSkillCtx{
		SelectedAgents: ctx.SelectedAgents,
		AllAgents:      ctx.AllAgents,
		IsDetected:     ctx.IsDetected,
	}

	if mode == ModeUninstall {
		if shared {
			decision, err := ResolveSharedSkillUninstall(skillPath, skillDir, agent, sharedCtx, steSkillPathOf)
			if err != nil {
				return nil, err
			}
			if decision.Kind == "defer" || decision.Kind == "keep" {
				return &WriteResult{Path: skillPath, Action: ActionUnchanged, Note: decision.Note}, nil
			}
		}
		action, err := removeSteSkillFiles(skillDir, skillPath)
		return &WriteResult{Path: skillPath, Action: action}, err
	}

	if shared {
		if err := EnsureSharedSkillOwnersOnInstall(skillDir, skillPath, agent, sharedCtx, steSkillPathOf); err != nil {
			return &WriteResult{Path: skillPath, Action: ActionError}, err
		}
	}

	if _, seen := ctx.SteSeenPaths[skillPath]; seen {
		return withCodexSkillsFeature(agent, &WriteResult{
			Path: skillPath, Action: ActionUnchanged, Note: "shared skill path already written",
		})
	}
	ctx.SteSeenPaths[skillPath] = struct{}{}

	existed := false
	if _, err := os.Stat(skillPath); err == nil {
		existed = true
		if matches, _ := stePackMatches(skillDir); matches {
			return withCodexSkillsFeature(agent, &WriteResult{Path: skillPath, Action: ActionUnchanged})
		}
	}

	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		return &WriteResult{Path: skillPath, Action: ActionError}, err
	}
	if err := os.WriteFile(skillPath, []byte(ste.SteSkillMD), 0o644); err != nil {
		return &WriteResult{Path: skillPath, Action: ActionError}, err
	}
	for _, file := range ste.SteReferenceFiles {
		refPath := filepath.Join(skillDir, file.RelativePath)
		if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
			return &WriteResult{Path: skillPath, Action: ActionError}, err
		}
		if err := os.WriteFile(refPath, []byte(file.Content), 0o644); err != nil {
			return &WriteResult{Path: skillPath, Action: ActionError}, err
		}
	}
	action := ActionCreated
	if existed {
		action = ActionUpdated
	}
	return withCodexSkillsFeature(agent, &WriteResult{Path: skillPath, Action: action})
}

func integrationApplies(integ integration, agent AgentTarget) bool {
	return integ.PlanPath(agent) != ""
}

func integrationsForAgents(agents []AgentTarget) []integration {
	all := integrations()
	var out []integration
	for _, integ := range all {
		for _, agent := range agents {
			if integrationApplies(integ, agent) {
				out = append(out, integ)
				break
			}
		}
	}
	return out
}

func integrations() []integration {
	return []integration{
		{ID: IntegrationMCP, Label: "MCP server", Description: "Miru search tools via MCP", DefaultChecked: true,
			PlanPath: func(a AgentTarget) string {
				if a.MCP != nil {
					return a.MCP.Path
				}
				return ""
			}, Apply: applyMCP},
		{ID: IntegrationInstructions, Label: "Instructions", Description: "search policy in agent docs", DefaultChecked: true,
			PlanPath: func(a AgentTarget) string { return a.InstructionsPath }, Apply: applyInstructions},
		{ID: IntegrationSubagent, Label: "Sub-agent", Description: "miru-code sub-agent file", DefaultChecked: true,
			PlanPath: func(a AgentTarget) string { return a.SubagentPath }, Apply: applySubagent},
		{ID: IntegrationRules, Label: "Cursor rules", Description: "search policy in .cursor/rules", DefaultChecked: true,
			PlanPath: func(a AgentTarget) string { return a.CursorRulesPath }, Apply: applyCursorRules},
		{ID: IntegrationHooks, Label: "Search hooks", Description: "blocks built-in search; routes to Miru MCP", Experimental: true, DefaultChecked: false,
			PlanPath: func(a AgentTarget) string { return a.HooksPath }, Apply: applyHooks},
		{ID: IntegrationCaveman, Label: "Caveman", Description: "on-demand chat compression skill (/caveman)", Experimental: true, DefaultChecked: false,
			PlanPath: func(a AgentTarget) string { return a.CavemanSkillPath }, Apply: applyCaveman},
		{ID: IntegrationSTE, Label: "STE writing", Description: "on-demand clear technical English for docs (/ste)", Experimental: true, DefaultChecked: false,
			PlanPath: func(a AgentTarget) string {
				if a.SteSkillDir == "" {
					return ""
				}
				return filepath.Join(a.SteSkillDir, "SKILL.md")
			}, Apply: applySte},
	}
}

// RunInstallerOptions configures non-interactive install.
type RunInstallerOptions struct {
	Yes       bool
	AllAgents bool
	AgentIDs  []string
}

// RunInstaller installs or uninstalls Miru agent configuration.
func RunInstaller(mode InstallMode, opts RunInstallerOptions) error {
	install := mode == ModeInstall
	if opts.Yes || opts.AllAgents || len(opts.AgentIDs) > 0 {
		_ = os.Setenv("MIRU_INSTALL_NONINTERACTIVE", "1")
	}
	if err := RequireInteractiveTerminal("miru " + string(mode)); err != nil {
		return err
	}
	if install {
		if NonInteractiveInstall() || opts.Yes {
			// Best-effort: don't block non-interactive install on missing credentials.
			_ = setup.EnsureCredentials(false)
		} else if err := setup.EnsureCredentials(true); err != nil {
			return err
		}
	}

	cliui.WriteStdout("")
	title := " uninstaller"
	if install {
		title = " installer"
	}
	cliui.WriteStdout(cliui.BrandTitle() + title)
	cliui.Divider("", 0, os.Stdout)

	targets := AgentTargets()
	selectedIDs := opts.AgentIDs
	if len(selectedIDs) == 0 {
		selectedIDs = ParseInstallAgentsEnv()
	}
	if opts.AllAgents {
		selectedIDs = nil
		for _, a := range targets {
			selectedIDs = append(selectedIDs, a.ID)
		}
	}

	var chosenAgents []AgentTarget
	if len(selectedIDs) > 0 {
		want := map[string]struct{}{}
		for _, id := range selectedIDs {
			want[id] = struct{}{}
		}
		for _, a := range targets {
			if _, ok := want[a.ID]; ok {
				chosenAgents = append(chosenAgents, a)
			}
		}
	} else {
		items := make([]MultiSelectItem[AgentTarget], 0, len(targets))
		for _, a := range targets {
			label := a.DisplayName
			detected := IsAgentDetected(a)
			if detected {
				label += " (detected)"
			}
			items = append(items, MultiSelectItem[AgentTarget]{Label: label, Value: a, Checked: detected && install})
		}
		chosenAgents = PromptMultiSelect("Agents to "+string(mode), items)
		if chosenAgents == nil {
			cliui.Hint("Cancelled.")
			return nil
		}
	}

	if len(chosenAgents) == 0 {
		cliui.Hint("Nothing selected. Exiting.")
		return nil
	}

	ints := integrationsForAgents(chosenAgents)
	if len(ints) == 0 {
		cliui.Hint("No integrations apply to the selected agents. Exiting.")
		return nil
	}
	chosenInts := ints
	if !NonInteractiveInstall() {
		items := make([]MultiSelectItem[integration], 0, len(ints))
		for _, integ := range ints {
			label := integ.Label
			if integ.Experimental {
				label += " (experimental)"
			}
			label += " — " + integ.Description
			checked := install && integ.DefaultChecked
			if !install {
				checked = true
			}
			items = append(items, MultiSelectItem[integration]{Label: label, Value: integ, Checked: checked})
		}
		chosenInts = PromptMultiSelect("Integrations", items)
		if chosenInts == nil {
			cliui.Hint("Cancelled.")
			return nil
		}
	} else {
		// non-interactive: mcp+instructions+subagent+rules (not hooks)
		filtered := make([]integration, 0)
		for _, integ := range ints {
			if integ.DefaultChecked || !install {
				filtered = append(filtered, integ)
			}
		}
		chosenInts = filtered
	}

	if len(chosenInts) == 0 {
		cliui.Hint("Nothing selected. Exiting.")
		return nil
	}

	if !opts.Yes && !NonInteractiveInstall() {
		question := "Remove miru configuration?"
		if install {
			question = "Proceed?"
		}
		if !PromptConfirm(question, install) {
			cliui.Hint("Cancelled.")
			return nil
		}
	}

	cliui.WriteStdout("")
	phase := "Removing"
	if install {
		phase = "Installing"
	}
	cliui.WriteStdout(cliui.Dim(phase))
	cliui.Divider("", 0, os.Stdout)

	applyCtx := &ApplyCtx{
		SelectedAgents:   chosenAgents,
		CavemanSeenPaths: map[string]struct{}{},
		SteSeenPaths:     map[string]struct{}{},
	}

	for _, agent := range chosenAgents {
		cliui.WriteStdout(" " + agent.DisplayName)
		for _, integ := range chosenInts {
			if !integrationApplies(integ, agent) {
				continue
			}
			result, err := integ.Apply(agent, mode, applyCtx)
			if err != nil {
				cliui.WriteStdout("   ✗ " + integ.Label + " error: " + err.Error())
				continue
			}
			if result == nil {
				continue
			}
			line := "   · " + integ.Label + " " + string(result.Action)
			if result.Note != "" {
				line += " — " + result.Note
			}
			cliui.WriteStdout(line)
			cliui.WriteStdout("      " + result.Path)
		}
	}
	cliui.WriteStdout("")
	if install {
		cliui.Success("Done. Restart agents to apply changes.")
	} else {
		cliui.Success("Done. Configuration removed.")
	}
	cliui.WriteStdout("")
	return nil
}
