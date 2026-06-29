//go:build linux && amd64

package sevenzip

import _ "embed"

//go:embed assets/7zz-linux-amd64
var embeddedBin []byte

var embeddedName = "7zz"
