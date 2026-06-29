package sevenzip

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// embeddedBin / embeddedName are provided by the platform-specific embed files
// (embed_windows.go, embed_linux_amd64.go, ...). When no binary is bundled for
// the platform, embeddedBin is empty and Bin() falls back to PATH.

// extractEmbedded writes the bundled binary for this platform to a stable cache
// location and returns its path, or "" if none is bundled.
func extractEmbedded() (string, error) {
	if len(embeddedBin) == 0 {
		return "", nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "wikit", "7z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Name the file by a content hash so a new wikit version never reuses a
	// stale extracted binary.
	sum := sha256.Sum256(embeddedBin)
	target := filepath.Join(dir, hex.EncodeToString(sum[:8])+"-"+embeddedName)
	if info, err := os.Stat(target); err != nil || info.Size() != int64(len(embeddedBin)) {
		if err := os.WriteFile(target, embeddedBin, 0o755); err != nil {
			return "", err
		}
	}
	return target, nil
}
