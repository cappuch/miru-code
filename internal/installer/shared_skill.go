package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// SkillOwnersFile is the sidecar next to a shared Agent Skill tracking which IDEs own it.
const SkillOwnersFile = "miru-owners.json"

// SharedSkillCtx holds context for shared-path skill install/uninstall.
type SharedSkillCtx struct {
	SelectedAgents []AgentTarget
	AllAgents      []AgentTarget
	IsDetected     func(AgentTarget) bool
}

// SkillPathOf returns the skill path for an agent.
type SkillPathOf func(AgentTarget) string

// SharedSkillUninstallDecision is the outcome of resolving a shared skill uninstall.
type SharedSkillUninstallDecision struct {
	Kind string // "defer", "keep", or "remove"
	Note string
}

func uniqueIDs(ids []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func resolvedAgents(ctx SharedSkillCtx) []AgentTarget {
	if ctx.AllAgents != nil {
		return ctx.AllAgents
	}
	return AgentTargets()
}

func agentsSharingPath(path string, allAgents []AgentTarget, pathOf SkillPathOf) []AgentTarget {
	var out []AgentTarget
	for _, agent := range allAgents {
		if pathOf(agent) == path {
			out = append(out, agent)
		}
	}
	return out
}

// IsSharedSkillPath reports whether multiple agents share the same skill path.
func IsSharedSkillPath(path string, allAgents []AgentTarget, pathOf SkillPathOf) bool {
	return len(agentsSharingPath(path, allAgents, pathOf)) > 1
}

// ReadSkillOwners reads miru-owners.json from a skill directory.
func ReadSkillOwners(skillDir string) ([]string, error) {
	ownersPath := filepath.Join(skillDir, SkillOwnersFile)
	data, err := os.ReadFile(ownersPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var parsed []any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil
	}
	var owners []string
	for _, item := range parsed {
		if s, ok := item.(string); ok {
			owners = append(owners, s)
		}
	}
	return owners, nil
}

// WriteSkillOwners writes miru-owners.json in a skill directory.
func WriteSkillOwners(skillDir string, owners []string) error {
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	sorted := append([]string(nil), owners...)
	sort.Strings(sorted)
	data, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(skillDir, SkillOwnersFile), append(data, '\n'), 0o644)
}

// EnsureSharedSkillOwnersOnInstall stamps shared-path ownership on install.
func EnsureSharedSkillOwnersOnInstall(skillDir, skillPath string, agent AgentTarget, ctx SharedSkillCtx, pathOf SkillPathOf) error {
	existing, err := ReadSkillOwners(skillDir)
	if err != nil {
		return err
	}
	if existing != nil {
		return WriteSkillOwners(skillDir, uniqueIDs(append(existing, agent.ID)))
	}

	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return WriteSkillOwners(skillDir, []string{agent.ID})
	}

	detect := ctx.IsDetected
	if detect == nil {
		detect = IsAgentDetected
	}
	var seeded []string
	for _, member := range agentsSharingPath(skillPath, resolvedAgents(ctx), pathOf) {
		if member.ID == agent.ID || detect(member) {
			seeded = append(seeded, member.ID)
		}
	}
	unique := uniqueIDs(seeded)
	if len(unique) <= 1 {
		return nil
	}
	return WriteSkillOwners(skillDir, unique)
}

func shouldKeepSharedSkill(path string, agent AgentTarget, ctx SharedSkillCtx, pathOf SkillPathOf) bool {
	detect := ctx.IsDetected
	if detect == nil {
		detect = IsAgentDetected
	}
	for _, sibling := range agentsSharingPath(path, resolvedAgents(ctx), pathOf) {
		if sibling.ID == agent.ID {
			continue
		}
		selected := false
		for _, s := range ctx.SelectedAgents {
			if s.ID == sibling.ID {
				selected = true
				break
			}
		}
		if selected {
			continue
		}
		if detect(sibling) {
			return true
		}
	}
	return false
}

// ResolveSharedSkillUninstall decides whether a shared-path skill uninstall should defer, keep, or remove.
func ResolveSharedSkillUninstall(path, skillDir string, agent AgentTarget, ctx SharedSkillCtx, pathOf SkillPathOf) (SharedSkillUninstallDecision, error) {
	selectedSharing := agentsSharingPath(path, ctx.SelectedAgents, pathOf)
	isLast := len(selectedSharing) == 0 || selectedSharing[len(selectedSharing)-1].ID == agent.ID
	if !isLast {
		return SharedSkillUninstallDecision{
			Kind: "defer",
			Note: "shared skill; remove deferred to last selected IDE",
		}, nil
	}

	owners, err := ReadSkillOwners(skillDir)
	if err != nil {
		return SharedSkillUninstallDecision{}, err
	}
	if owners != nil {
		removing := map[string]struct{}{}
		for _, selected := range selectedSharing {
			removing[selected.ID] = struct{}{}
		}
		var remaining []string
		for _, ownerID := range owners {
			if _, ok := removing[ownerID]; !ok {
				remaining = append(remaining, ownerID)
			}
		}
		if len(remaining) > 0 {
			if err := WriteSkillOwners(skillDir, remaining); err != nil {
				return SharedSkillUninstallDecision{}, err
			}
			return SharedSkillUninstallDecision{
				Kind: "keep",
				Note: "shared skill kept — still owned by another IDE",
			}, nil
		}
		return SharedSkillUninstallDecision{Kind: "remove"}, nil
	}

	if shouldKeepSharedSkill(path, agent, ctx, pathOf) {
		return SharedSkillUninstallDecision{
			Kind: "keep",
			Note: "shared skill kept — another IDE still uses this path",
		}, nil
	}
	return SharedSkillUninstallDecision{Kind: "remove"}, nil
}

// RemoveSkillMdAndOwners removes SKILL.md and the owners sidecar; leaves sibling skill folders untouched.
func RemoveSkillMdAndOwners(path, skillDir string) (InstallAction, error) {
	removedSomething := false

	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return ActionError, err
		}
		removedSomething = true
	}

	ownersPath := filepath.Join(skillDir, SkillOwnersFile)
	if _, err := os.Stat(ownersPath); err == nil {
		if err := os.Remove(ownersPath); err != nil {
			return ActionError, err
		}
		removedSomething = true
	}

	_ = os.Remove(skillDir)

	if removedSomething {
		return ActionRemoved, nil
	}
	return ActionNotFound, nil
}

func cavemanPathOf(agent AgentTarget) string {
	return agent.CavemanSkillPath
}

// IsSharedCavemanPath reports whether multiple agents share the same Caveman skill path.
func IsSharedCavemanPath(path string, allAgents []AgentTarget) bool {
	return IsSharedSkillPath(path, allAgents, cavemanPathOf)
}

// EnsureSharedCavemanOwnersOnInstall stamps shared Caveman ownership on install.
func EnsureSharedCavemanOwnersOnInstall(skillDir, skillPath string, agent AgentTarget, ctx SharedSkillCtx) error {
	return EnsureSharedSkillOwnersOnInstall(skillDir, skillPath, agent, ctx, cavemanPathOf)
}

// ResolveSharedCavemanUninstall resolves shared Caveman uninstall.
func ResolveSharedCavemanUninstall(path, skillDir string, agent AgentTarget, ctx SharedSkillCtx) (SharedSkillUninstallDecision, error) {
	return ResolveSharedSkillUninstall(path, skillDir, agent, ctx, cavemanPathOf)
}

// RemoveCavemanSkillFiles removes Caveman SKILL.md and owners sidecar.
func RemoveCavemanSkillFiles(path, skillDir string) (InstallAction, error) {
	return RemoveSkillMdAndOwners(path, skillDir)
}

func steSkillPathOf(agent AgentTarget) string {
	if agent.SteSkillDir == "" {
		return ""
	}
	return filepath.Join(agent.SteSkillDir, "SKILL.md")
}
