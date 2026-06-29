//go:build !windows && !darwin && !(linux && (amd64 || arm64))

package sevenzip

// No bundled binary for this platform; Bin() falls back to a 7z on PATH.
var embeddedBin []byte
var embeddedName = ""
