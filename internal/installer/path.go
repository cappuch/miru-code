package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// installShellPath persists the executable directory, even when the installer
// inherited a PATH that only temporarily contains it. A child process cannot
// update the terminal that launched it.
func installShellPath() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(home, ".local", "bin", "miru")
	if err := copyExecutable(exe, target); err != nil {
		return "", fmt.Errorf("install miru executable: %w", err)
	}
	installedExecutable = target
	return writeShellPath(home, os.Getenv("SHELL"), os.Getenv("ZDOTDIR"), filepath.Dir(target))
}

// Copy rather than symlink: go run executables live in temporary directories.
// Rename atomically so an existing executable can safely be upgraded in use.
func copyExecutable(source, target string) error {
	srcInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if dstInfo, err := os.Stat(target); err == nil && os.SameFile(srcInfo, dstInfo) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.CreateTemp(filepath.Dir(target), ".miru-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(dst.Name())
	_, copyErr := io.Copy(dst, src)
	if copyErr == nil {
		copyErr = dst.Chmod(0755)
	}
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(dst.Name(), target)
}

func writeShellPath(home, shell, zdotdir, binDir string) (string, error) {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	var profiles []string
	line := "case \":$PATH:\" in *:" + quote(binDir) + ":*) ;; *) export PATH=" + quote(binDir) + ":\"$PATH\" ;; esac"
	switch filepath.Base(shell) {
	case "", "zsh":
		if zdotdir == "" {
			zdotdir = home
		}
		profiles = []string{filepath.Join(zdotdir, ".zshrc")}
	case "bash":
		// Bash login shells read only the first existing login profile.
		login := filepath.Join(home, ".bash_profile")
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			candidate := filepath.Join(home, name)
			if _, err := os.Stat(candidate); err == nil {
				login = candidate
				break
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		profiles = []string{login, filepath.Join(home, ".bashrc")}
	default:
		return "", fmt.Errorf("unsupported shell %q; add %s to PATH in your shell configuration", shell, binDir)
	}
	for _, profile := range profiles {
		content, err := os.ReadFile(profile)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if strings.Contains(string(content), line+"\n") {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(profile), 0755); err != nil {
			return "", err
		}
		f, err := os.OpenFile(profile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		_, writeErr := f.WriteString("\n# Added by miru install\n" + line + "\n")
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return strings.Join(profiles, ", "), nil
}
