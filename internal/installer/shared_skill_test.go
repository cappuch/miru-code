package installer

import "testing"

func TestIsSharedCavemanPath(t *testing.T) {
	agents := []AgentTarget{
		{ID: "claude", CavemanSkillPath: "/home/.claude/skills/caveman/SKILL.md"},
		{ID: "cursor", CavemanSkillPath: "/home/.agents/skills/caveman/SKILL.md"},
		{ID: "gemini", CavemanSkillPath: "/home/.agents/skills/caveman/SKILL.md"},
	}
	if !IsSharedCavemanPath("/home/.agents/skills/caveman/SKILL.md", agents) {
		t.Fatal("expected shared agents path to be shared")
	}
	if IsSharedCavemanPath("/home/.claude/skills/caveman/SKILL.md", agents) {
		t.Fatal("expected native claude path not to be shared")
	}
}

func TestIntegrationsForAgentsFiltersCaveman(t *testing.T) {
	agents := []AgentTarget{{ID: "cursor", CavemanSkillPath: "/tmp/caveman/SKILL.md", SteSkillDir: "/tmp/ste"}}
	got := integrationsForAgents(agents)
	foundCaveman := false
	for _, integ := range got {
		if integ.ID == IntegrationCaveman {
			foundCaveman = true
		}
	}
	if !foundCaveman {
		t.Fatal("expected caveman integration for agent with caveman path")
	}
}

func TestIsCopilotInstalled(t *testing.T) {
	if IsCopilotInstalled("/nonexistent-home-dir-miru-test") {
		t.Fatal("expected false for empty home")
	}
}
