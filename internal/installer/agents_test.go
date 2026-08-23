package installer

import (
	"path/filepath"
	"testing"
)

func TestSkillPathHelpers(t *testing.T) {
	home := "/home/user"
	if got := AgentsCavemanSkillPath(home); got != filepath.Join(home, ".agents", "skills", "caveman", "SKILL.md") {
		t.Fatalf("AgentsCavemanSkillPath = %q", got)
	}
	if got := NativeCavemanSkillPath(home, NativeSkillClaude); got != filepath.Join(home, ".claude", "skills", "caveman", "SKILL.md") {
		t.Fatalf("NativeCavemanSkillPath = %q", got)
	}
	if got := AgentsSteSkillDir(home); got != filepath.Join(home, ".agents", "skills", "ste") {
		t.Fatalf("AgentsSteSkillDir = %q", got)
	}
	if got := NativeSteSkillDir(home, NativeSkillKiro); got != filepath.Join(home, ".kiro", "skills", "ste") {
		t.Fatalf("NativeSteSkillDir = %q", got)
	}
	if got := CopilotHomeDir(home); got != filepath.Join(home, ".copilot") {
		t.Fatalf("CopilotHomeDir = %q", got)
	}
	if got := OpencodeConfigDir(home); got != filepath.Join(home, ".config", "opencode") {
		t.Fatalf("OpencodeConfigDir = %q", got)
	}
}
