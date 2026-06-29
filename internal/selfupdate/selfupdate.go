// Package selfupdate implements `wikit update`: it queries the latest GitHub
// release, downloads the binary for the current platform, verifies it against
// the release's SHA256SUMS (when present), and atomically replaces the running
// executable. It also provides a cached, non-intrusive "new version available"
// check used after a backup run.
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const apiTimeout = 20 * time.Second

// AssetName is the release asset filename for the current platform, matching the
// dist/ naming (wikit-<os>-<arch>[.exe]).
func AssetName() string {
	name := "wikit-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// latest fetches the latest release metadata for owner/repo.
func latest(repo string) (*ghRelease, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wikit-self-update")
	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// Run performs `wikit update`. When checkOnly is true it reports availability
// without installing.
func Run(repo, current string, checkOnly bool) error {
	if repo == "" || repo == "OWNER/REPO" {
		return fmt.Errorf("update repository is not configured (build with -ldflags \"-X main.updateRepo=owner/repo\" or set WIKIT_UPDATE_REPO)")
	}
	fmt.Printf("Current version: %s\nChecking %s for updates...\n", current, repo)
	rel, err := latest(repo)
	if err != nil {
		return err
	}
	if !isNewer(current, rel.TagName) {
		fmt.Printf("Already up to date (latest is %s).\n", rel.TagName)
		return nil
	}
	fmt.Printf("New version available: %s\n", rel.TagName)
	if checkOnly {
		fmt.Println("Run `wikit update` to install it.")
		return nil
	}

	asset := AssetName()
	var dlURL string
	var sumsURL string
	for _, a := range rel.Assets {
		if a.Name == asset {
			dlURL = a.URL
		}
		if a.Name == "SHA256SUMS" {
			sumsURL = a.URL
		}
	}
	if dlURL == "" {
		return fmt.Errorf("release %s has no asset %q for this platform", rel.TagName, asset)
	}

	fmt.Printf("Downloading %s...\n", asset)
	data, err := download(dlURL)
	if err != nil {
		return err
	}

	if sumsURL != "" {
		fmt.Println("Verifying checksum...")
		if err := verify(data, asset, sumsURL); err != nil {
			return err
		}
	} else {
		fmt.Println("Warning: release has no SHA256SUMS; skipping checksum verification.")
	}

	if err := replaceSelf(data); err != nil {
		return err
	}
	fmt.Printf("Updated to %s.\n", rel.TagName)
	return nil
}

func download(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "wikit-self-update")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// verify checks data against the asset's line in a SHA256SUMS file.
func verify(data []byte, asset, sumsURL string) error {
	sums, err := download(sumsURL)
	if err != nil {
		return fmt.Errorf("fetch SHA256SUMS: %w", err)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("SHA256SUMS has no entry for %s", asset)
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	return nil
}

// replaceSelf atomically swaps the running executable with new contents.
func replaceSelf(data []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	newPath := exe + ".new"
	if err := os.WriteFile(newPath, data, 0o755); err != nil {
		return fmt.Errorf("cannot write to %s (no permission?): %w", dir, err)
	}

	if runtime.GOOS == "windows" {
		// A running .exe can be renamed but not overwritten; move it aside first.
		oldPath := exe + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(exe, oldPath); err != nil {
			_ = os.Remove(newPath)
			return err
		}
		if err := os.Rename(newPath, exe); err != nil {
			_ = os.Rename(oldPath, exe) // roll back
			return err
		}
		return nil
	}

	return os.Rename(newPath, exe)
}

// CleanupOld removes the leftover <exe>.old from a previous Windows update.
func CleanupOld() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}

// isNewer reports whether candidate is a newer version than current. A "dev" or
// unparseable current is always considered older. Falls back to string inequality.
func isNewer(current, candidate string) bool {
	if current == "dev" || current == "" {
		return true
	}
	c := parseSemver(current)
	n := parseSemver(candidate)
	if c == nil || n == nil {
		return strings.TrimPrefix(candidate, "v") != strings.TrimPrefix(current, "v")
	}
	for i := 0; i < 3; i++ {
		if n[i] != c[i] {
			return n[i] > c[i]
		}
	}
	return false
}

func parseSemver(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i != -1 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return nil
	}
	out := []int{0, 0, 0}
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}
