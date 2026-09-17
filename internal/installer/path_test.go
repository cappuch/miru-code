package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyExecutable(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "temporary-build")
	target := filepath.Join(dir, "bin", "miru")
	for _, content := range []string{"first build", "updated build"} {
		if err := os.WriteFile(source, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := copyExecutable(source, target); err != nil {
			t.Fatal(err)
		}
		if err := copyExecutable(target, target); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != content {
			t.Fatalf("copy: %q, %v", got, err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0755 {
			t.Fatalf("mode: %v", info.Mode())
		}
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestShellPath(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			command, err := exec.LookPath(shell)
			if err != nil {
				t.Skip(shell + " unavailable")
			}
			home := t.TempDir()
			bin := filepath.Join(home, "bin with 'quotes' $dollars `ticks`")
			zdot := filepath.Join(home, "custom-zsh")
			profile := filepath.Join(zdot, ".zshrc")
			if shell == "bash" {
				profile = filepath.Join(home, ".profile")
			}
			if err := os.MkdirAll(filepath.Dir(profile), 0755); err != nil {
				t.Fatal(err)
			}
			original := "# user configuration without trailing newline"
			if err := os.WriteFile(profile, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err := writeShellPath(home, shell, zdot, bin); err != nil {
					t.Fatal(err)
				}
			}
			content, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(content), original+"\n") || strings.Count(string(content), "# Added by miru install") != 1 {
				t.Fatalf("profile not preserved or duplicated: %s", content)
			}
			profiles := []string{profile}
			if shell == "bash" {
				profiles = append(profiles, filepath.Join(home, ".bashrc"))
			}
			for _, p := range profiles {
				cmd := exec.Command(command, "-c", `. "$1"; . "$1"; printf '%s' "$PATH"`, "test", p)
				cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %s", err, output)
				}
				if string(output) != bin+":/usr/bin:/bin" {
					t.Fatalf("unexpected PATH: %s", output)
				}
			}
		})
	}
}
