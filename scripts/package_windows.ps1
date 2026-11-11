# Package the Windows amd64 binary as a zip. The Inno Setup script is
# scaffolding only; this script does not compile an installer and does
# not sign anything.
$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)
$ver = "0.14.1"
$types = Get-Content "internal/core/types.go" -Raw
if ($types -match 'const Version = "([^"]+)"') { $ver = $Matches[1] }
$exe = "build/iconcraft-windows-amd64.exe"
if (-not (Test-Path $exe)) {
  $env:CGO_ENABLED = "0"
  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  go build -trimpath -o $exe ./cmd/iconcraft
}
$stage = "dist/windows/IconCraft-windows-amd64-$ver"
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null
Copy-Item $exe (Join-Path $stage "IconCraft.exe")
Copy-Item "PLATFORMS.md" (Join-Path $stage "PLATFORMS.md")
Copy-Item "packaging/windows/README.txt" (Join-Path $stage "README.txt")
New-Item -ItemType Directory -Force -Path "dist/windows" | Out-Null
$zip = "dist/windows/IconCraft-windows-amd64-$ver.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path $stage -DestinationPath $zip
Write-Host "windows zip: $zip"
Write-Host "Inno Setup script (not compiled): packaging/windows/iconcraft.iss"
