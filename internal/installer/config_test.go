package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONConfigPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	original := "{\n// keep this comment\n\"big\":9007199254740993,\n\"mcp\":{\n\"other\": {\"url\":\"https://example.com//x\",},\n},\n}\n"
	if err := os.WriteFile(path, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"command": "miru"}
	action, err := MergeJSONMember(path, "mcp", "miru", value)
	if err != nil || action != ActionUpdated {
		t.Fatalf("%s %v", action, err)
	}
	data, _ := os.ReadFile(path)
	for _, fragment := range []string{"// keep this comment", "\"big\":9007199254740993", "\"other\": {\"url\":\"https://example.com//x\",}"} {
		if !strings.Contains(string(data), fragment) {
			t.Fatalf("lost %s", fragment)
		}
	}
	backups, _ := filepath.Glob(path + ".miru-backup-*")
	if len(backups) != 1 {
		t.Fatalf("backups: %v", backups)
	}
	backup, _ := os.ReadFile(backups[0])
	if string(backup) != original {
		t.Fatal("backup differs")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed")
	}
	action, err = MergeJSONMember(path, "mcp", "miru", value)
	if err != nil || action != ActionUnchanged {
		t.Fatalf("repeat: %s %v", action, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("repeat changed bytes")
	}
	action, err = RemoveJSONMember(path, "mcp", "miru")
	if err != nil || action != ActionRemoved {
		t.Fatalf("remove: %s %v", action, err)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "// keep this comment") || !strings.Contains(string(after), "9007199254740993") {
		t.Fatal("remove lost unrelated content")
	}
}

func TestJSONConfigRefusesDestructiveChanges(t *testing.T) {
	for _, original := range []string{`{"mcp": [1]}`, `{"mcp":{},"mcp":{}}`, `{"mcp":`, `[]`} {
		t.Run(original, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.jsonc")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			action, err := MergeJSONMember(path, "mcp", "miru", map[string]any{"command": "miru"})
			if err == nil || action != ActionError {
				t.Fatalf("%s %v", action, err)
			}
			after, _ := os.ReadFile(path)
			if string(after) != original {
				t.Fatal("changed invalid input")
			}
		})
	}
}

func TestJSONConfigSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeJSONMember(link, "mcp", "miru", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(link)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
}

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
