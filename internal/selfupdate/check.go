package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type checkCache struct {
	CheckedAt int64  `json:"checked_at"` // unix seconds
	Latest    string `json:"latest"`
}

const checkInterval = 24 * time.Hour

// NotifyIfNewer prints a one-line notice to stderr when a newer release exists.
// It caches the result for a day so normal runs don't hit the network every
// time, and silently does nothing on any error or when update is unconfigured.
func NotifyIfNewer(repo, current string) {
	if repo == "" || repo == "OWNER/REPO" {
		return
	}
	latestTag, ok := cachedLatest(repo)
	if !ok {
		return
	}
	if isNewer(current, latestTag) {
		fmt.Fprintf(os.Stderr, "\nA new version of wikit is available: %s (current %s). Run `wikit update` to upgrade.\n", latestTag, current)
	}
}

func cachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "wikit", "update-check.json")
}

// cachedLatest returns the latest tag, using a day-old cache when available and
// otherwise querying GitHub and refreshing the cache.
func cachedLatest(repo string) (string, bool) {
	path := cachePath()
	if data, err := os.ReadFile(path); err == nil {
		var c checkCache
		if json.Unmarshal(data, &c) == nil {
			if time.Since(time.Unix(c.CheckedAt, 0)) < checkInterval && c.Latest != "" {
				return c.Latest, true
			}
		}
	}

	rel, err := latest(repo)
	if err != nil {
		return "", false
	}
	c := checkCache{CheckedAt: time.Now().Unix(), Latest: rel.TagName}
	if data, err := json.Marshal(c); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o644)
	}
	return rel.TagName, true
}
