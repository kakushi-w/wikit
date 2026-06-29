#!/bin/sh
# wikit installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/kakushi-w/wikit/main/install.sh | sh
#
# Downloads the latest release binary for this platform and installs it to
# ~/.local/bin (added to PATH). No root required.
set -eu

REPO="kakushi-w/wikit"

os=$(uname -s)
case "$os" in
  Linux)  goos=linux ;;
  Darwin) goos=darwin ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) goarch=amd64 ;;
  aarch64|arm64) goarch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="wikit-${goos}-${goarch}"
echo "Looking up latest $asset from $REPO..."

url=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep -o "\"browser_download_url\": *\"[^\"]*${asset}\"" \
  | head -n1 | sed 's/.*"\(https[^"]*\)"$/\1/')

if [ -z "${url:-}" ]; then
  echo "Could not find a release asset named $asset." >&2
  echo "Make sure the repository is public and has a published release." >&2
  exit 1
fi

tmp=$(mktemp)
echo "Downloading $url"
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"

# Let the binary install itself (copies to ~/.local/bin and updates PATH).
"$tmp" install
rm -f "$tmp"
