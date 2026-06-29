//go:build darwin

package sevenzip

import _ "embed"

//go:embed assets/7zz-macos
var embeddedBin []byte

var embeddedName = "7zz"
