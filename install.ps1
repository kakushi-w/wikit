# wikit installer for Windows.
#
#   irm https://raw.githubusercontent.com/kakushi-w/wikit/main/install.ps1 | iex
#
# Downloads the latest release binary and installs it to
# %LOCALAPPDATA%\Programs\wikit (added to your user PATH). No admin required.
$ErrorActionPreference = "Stop"

$repo = "kakushi-w/wikit"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$asset = "wikit-windows-$arch.exe"

Write-Host "Looking up latest $asset from $repo..."
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "User-Agent" = "wikit-install" }
$dl = ($release.assets | Where-Object { $_.name -eq $asset }).browser_download_url

if (-not $dl) {
    Write-Error "Could not find a release asset named $asset. Make sure the repository is public and has a published release."
}

$tmp = Join-Path $env:TEMP $asset
Write-Host "Downloading $dl"
Invoke-WebRequest -Uri $dl -OutFile $tmp -UseBasicParsing

# Let the binary install itself (copies to %LOCALAPPDATA%\Programs\wikit and
# updates PATH).
& $tmp install
Remove-Item $tmp -ErrorAction SilentlyContinue
