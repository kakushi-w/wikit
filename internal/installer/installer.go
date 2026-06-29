// Package installer implements `wikit install` / `wikit uninstall`: it copies the
// running binary into a per-user location and puts that location on PATH, so the
// user can invoke `wikit` from anywhere without manual setup. No admin rights
// are required (it installs per-user).
package installer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// targetDir returns the install directory and the binary filename for the
// current platform. Both are per-user, so no elevation is needed.
func targetDir() (dir, binName string) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, "Programs", "wikit"), "wikit.exe"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin"), "wikit"
}

// Install copies the running executable into the per-user bin directory and adds
// that directory to PATH.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)

	dir, binName := targetDir()
	dest := filepath.Join(dir, binName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	if !samePath(exe, dest) {
		data, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o755); err != nil {
			return fmt.Errorf("cannot write %s: %w", dest, err)
		}
	}
	fmt.Printf("Installed wikit to %s\n", dest)

	// Allow callers (install scripts, tests) to manage PATH themselves.
	if os.Getenv("WIKIT_NO_PATH") != "" {
		fmt.Printf("Skipped PATH update (WIKIT_NO_PATH set). Ensure %s is on your PATH.\n", dir)
		return nil
	}

	added, hint, err := ensureOnPath(dir)
	if err != nil {
		fmt.Printf("Installed, but could not update PATH automatically: %v\n", err)
		fmt.Printf("Add this directory to your PATH manually: %s\n", dir)
		return nil
	}
	if added {
		fmt.Println("Added it to your PATH.")
	} else {
		fmt.Println("It is already on your PATH.")
	}
	if hint != "" {
		fmt.Println(hint)
	}
	fmt.Println("Then run:  wikit version")
	return nil
}

// Uninstall removes the installed binary. PATH entries are left in place (they
// are harmless) but reported so the user can remove them if desired.
func Uninstall() error {
	dir, binName := targetDir()
	dest := filepath.Join(dir, binName)
	if err := os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Nothing to remove (no binary at %s).\n", dest)
			return nil
		}
		return err
	}
	fmt.Printf("Removed %s\n", dest)
	fmt.Printf("If you want, also remove %s from your PATH.\n", dir)
	return nil
}

// ensureOnPath makes dir part of the user's PATH. It returns whether a change was
// made and a hint for activating it in the current session.
func ensureOnPath(dir string) (added bool, hint string, err error) {
	if runtime.GOOS == "windows" {
		return ensureOnPathWindows(dir)
	}
	return ensureOnPathUnix(dir)
}

func ensureOnPathWindows(dir string) (bool, string, error) {
	// Read the user-scoped PATH via PowerShell (avoids setx's 1024-char
	// truncation), append if missing, and write it back. .NET's
	// SetEnvironmentVariable broadcasts the change to new processes.
	cur, err := psOutput(`[Environment]::GetEnvironmentVariable('Path','User')`)
	if err != nil {
		return false, "", err
	}
	for _, p := range strings.Split(cur, ";") {
		if strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			return false, "Open a new terminal, then run wikit from anywhere.", nil
		}
	}
	newPath := cur
	if newPath != "" && !strings.HasSuffix(newPath, ";") {
		newPath += ";"
	}
	newPath += dir
	script := `[Environment]::SetEnvironmentVariable('Path', ` + psQuote(newPath) + `, 'User')`
	if _, err := psOutput(script); err != nil {
		return false, "", err
	}
	return true, "Open a NEW terminal window for the PATH change to take effect.", nil
}

func ensureOnPathUnix(dir string) (bool, string, error) {
	// If already visible in the current PATH, assume rc files are fine.
	for _, p := range strings.Split(os.Getenv("PATH"), ":") {
		if p == dir {
			return false, "", nil
		}
	}
	home, _ := os.UserHomeDir()
	line := `export PATH="` + dir + `:$PATH"`
	marker := "# added by wikit installer"
	block := "\n" + marker + "\n" + line + "\n"

	// Update whichever common shell rc files exist, plus ~/.profile.
	candidates := []string{
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
	}
	wrote := false
	for _, rc := range candidates {
		data, err := os.ReadFile(rc)
		if err != nil {
			if os.IsNotExist(err) {
				// Only create ~/.profile if nothing else exists; skip others.
				if filepath.Base(rc) != ".profile" {
					continue
				}
			} else {
				continue
			}
		}
		if bytes.Contains(data, []byte(marker)) || bytes.Contains(data, []byte(line)) {
			wrote = true
			continue
		}
		f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		_, _ = f.WriteString(block)
		_ = f.Close()
		wrote = true
	}
	if !wrote {
		return false, "", fmt.Errorf("no shell profile found")
	}
	return true, fmt.Sprintf("Restart your shell or run:  source ~/.profile   (or open a new terminal)"), nil
}

func psOutput(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\r\n"), err
}

// psQuote single-quotes a string for PowerShell (doubling embedded quotes).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func samePath(a, b string) bool {
	ax, err1 := filepath.Abs(a)
	bx, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ax, bx)
	}
	return ax == bx
}
