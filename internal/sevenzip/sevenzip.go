// Package sevenzip wraps a 7-Zip command-line binary to create and list the .7z
// archives WikiComma uses for page revisions and forum threads. The 7z engine is
// bundled (embedded) so wikit needs nothing installed; if no embedded binary is
// available for the platform it falls back to a 7z/7za/7zz found on PATH.
//
// Archive container bytes are not reproducible across tools/runs (7z stores file
// timestamps), but the members and their contents are identical, which is what
// matters for compatibility.
package sevenzip

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

var (
	resolveOnce sync.Once
	resolvedBin string
	resolveErr  error
)

// Bin returns the path to a usable 7z binary, extracting the embedded one on
// first use.
func Bin() (string, error) {
	resolveOnce.Do(func() {
		if p, err := extractEmbedded(); err == nil && p != "" {
			resolvedBin = p
			return
		}
		for _, name := range []string{"7z", "7za", "7zz", "7zr"} {
			if p, err := exec.LookPath(name); err == nil {
				resolvedBin = p
				return
			}
		}
		resolveErr = fmt.Errorf("no 7z binary found (not embedded for this platform and not on PATH)")
	})
	return resolvedBin, resolveErr
}

// Add adds files matching fileSpec (a path possibly containing 7z wildcards) to
// the archive at archivePath, creating or updating it. When recursive is true,
// subdirectories are included (-r). This mirrors the original's two call sites:
// page revisions use "<dir>/*.txt" (non-recursive) and forum threads use
// "<dir>/*.*" (recursive). 7z stores members relative to the wildcard base, so
// e.g. pages/<name>/*.txt yields bare "<rev>.txt" members.
func Add(archivePath, fileSpec string, recursive bool) error {
	bin, err := Bin()
	if err != nil {
		return err
	}
	args := []string{"a", archivePath, fileSpec, "-y", "-bso0", "-bsp0"}
	if recursive {
		args = append(args, "-r")
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z add failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns the member paths inside an archive (using forward slashes),
// excluding directories. Used by the incremental scan to learn which revisions
// and forum posts are already archived.
func List(archivePath string) ([]string, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "l", "-slt", "-ba", archivePath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("7z list failed: %v", err)
	}

	var files []string
	var curPath string
	var isDir bool
	flush := func() {
		if curPath != "" && !isDir {
			files = append(files, strings.ReplaceAll(curPath, "\\", "/"))
		}
		curPath = ""
		isDir = false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		if v, ok := strings.CutPrefix(line, "Path = "); ok {
			flush()
			curPath = v
		} else if v, ok := strings.CutPrefix(line, "Attributes = "); ok {
			isDir = strings.HasPrefix(v, "D")
		}
	}
	flush()
	return files, nil
}
