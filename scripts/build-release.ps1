param([string]$Version = "v0.1.0")
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$out = Join-Path $root "dist"
if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Path $out | Out-Null
$pkg = "github.com/rohitkumarsundararajan/devlatch/internal/version.Version"
$targets = @(
  @{os="windows"; arch="amd64"},
  @{os="linux"; arch="amd64"},
  @{os="linux"; arch="arm64"},
  @{os="darwin"; arch="amd64"},
  @{os="darwin"; arch="arm64"}
)
$env:CGO_ENABLED = "0"
foreach ($t in $targets) {
  $env:GOOS = $t.os; $env:GOARCH = $t.arch
  $exe = if ($t.os -eq "windows") { "devlatch.exe" } else { "devlatch" }
  $name = "devlatch_$($Version)_$($t.os)_$($t.arch)"
  $stage = Join-Path $out $name
  New-Item -ItemType Directory -Path $stage | Out-Null
  go build -trimpath -ldflags "-s -w -X $pkg=$Version" -o (Join-Path $stage $exe) ./cmd/devlatch
  if ($LASTEXITCODE -ne 0) { throw "build failed for $($t.os)/$($t.arch)" }
  Copy-Item LICENSE, README.md $stage
  if ($t.os -eq "windows") { Compress-Archive -Path "$stage\*" -DestinationPath "$stage.zip" }
  else { tar -czf "$stage.tar.gz" -C $out $name }
  Remove-Item -Recurse -Force $stage
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Get-ChildItem $out -File | Get-FileHash -Algorithm SHA256 | ForEach-Object { "$($_.Hash.ToLower())  $(Split-Path -Leaf $_.Path)" } | Set-Content (Join-Path $out "checksums.txt")
Get-ChildItem $out