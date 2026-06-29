//go:build windows

package sevenzip

import _ "embed"

//go:embed assets/7zr.exe
var embeddedBin []byte

var embeddedName = "7zr.exe"
