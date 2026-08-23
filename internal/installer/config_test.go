package installer

import "testing"

func TestCodexSkillsFeatureEnabled(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"", false},
		{"[features]\nskills = true\n", true},
		{"[features]\nskills = false\n", false},
		{"# comment\n[features]\n  skills = true\n", true},
		{"[other]\nfoo = 1\n[features]\nskills = true\n", true},
	}
	for _, tc := range cases {
		if got := CodexSkillsFeatureEnabled(tc.text); got != tc.want {
			t.Errorf("CodexSkillsFeatureEnabled(%q) = %v want %v", tc.text, got, tc.want)
		}
	}
}
