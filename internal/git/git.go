package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clone clones a git URL into a temp directory and returns the local path.
func Clone(url string, ref *string) (string, error) {
	dir, err := os.MkdirTemp("", "miru-git-*")
	if err != nil {
		return "", err
	}
	args := []string{"clone", "--depth", "1"}
	if ref != nil && *ref != "" {
		args = append(args, "--branch", *ref)
	}
	args = append(args, url, dir)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("git clone: %w\n%s", err, string(out))
	}
	return dir, nil
}

// IsGitRepo reports whether path is inside a git working tree.
func IsGitRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// ResolveWorkTree returns the git top-level for path, or path itself.
func ResolveWorkTree(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return abs
	}
	return strings.TrimSpace(string(out))
}
