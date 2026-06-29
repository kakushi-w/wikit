//go:build linux && arm64

package sevenzip

import _ "embed"

//go:embed assets/7zz-linux-arm64
var embeddedBin []byte

var embeddedName = "7zz"
