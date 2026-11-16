# Records the demo backend through the DevLatch proxy and writes the CI fixture.
#   .\scripts\make-demo-ci.ps1                          approved contract + traffic
#   .\scripts\make-demo-ci.ps1 -Mutate rename_user_name traffic only (simulates a PR that renames a field)
param(
  [string]$Out = 'examples\demo-ci',
  [string]$Mutate = '',
  [int]$Rounds = 25
)
$ErrorActionPreference = 'Stop'
$root = (Get-Location).Path

function Wait-Port([int]$Port) {
  for ($i = 0; $i -lt 40; $i++) {
    try {
      $c = New-Object Net.Sockets.TcpClient
      $c.Connect('127.0.0.1', $Port)
      $c.Close()
      return
    } catch { Start-Sleep -Milliseconds 250 }
  }
  throw "port $Port did not open"
}

New-Item -ItemType Directory -Force -Path 'bin' | Out-Null
go build -o bin\devlatch.exe .\cmd\devlatch
if ($LASTEXITCODE -ne 0) { throw 'build devlatch failed' }
go build -o bin\demo-backend.exe .\examples\demo-backend
if ($LASTEXITCODE -ne 0) { throw 'build demo-backend failed' }

$work = Join-Path $env:TEMP ('devlatch-rec-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Force -Path (Join-Path $work '.devlatch') | Out-Null
$be = $null
$rec = $null
try {
  $beArgs = @('-addr', '127.0.0.1:8000')
  if ($Mutate) { $beArgs += @('-mutate', $Mutate) }
  $be = Start-Process -FilePath "$root\bin\demo-backend.exe" -ArgumentList $beArgs -PassThru -WindowStyle Hidden
  Wait-Port 8000

  $recArgs = @('record', '--target', 'http://127.0.0.1:8000', '--port', '9000', '--duration', '15s', '--save-traffic', '.devlatch\traffic.jsonl')
  $rec = Start-Process -FilePath "$root\bin\devlatch.exe" -ArgumentList $recArgs -WorkingDirectory $work -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput "$work\record.out" -RedirectStandardError "$work\record.err"
  $null = $rec.Handle
  Wait-Port 9000

  $base = 'http://127.0.0.1:9000/api/v1'
  for ($r = 1; $r -le $Rounds; $r++) {
    $id = (($r - 1) % 20) + 1
    Invoke-RestMethod "$base/users" | Out-Null
    Invoke-RestMethod "$base/users/$id" | Out-Null
    Invoke-RestMethod "$base/orders" | Out-Null
    Invoke-RestMethod "$base/stats/daily" | Out-Null
  }
  $rec.WaitForExit()
  if ($null -ne $rec.ExitCode -and $rec.ExitCode -ne 0) { throw ('record exited with ' + $rec.ExitCode + ': ' + (Get-Content "$work\record.err" -Raw)) }
  if (-not (Test-Path "$work\.devlatch\traffic.jsonl") -or (Get-Item "$work\.devlatch\traffic.jsonl").Length -eq 0) { throw ('record saved no traffic: ' + (Get-Content "$work\record.err" -Raw)) }

  if (-not $Mutate) {
    Push-Location $work
    & "$root\bin\devlatch.exe" approve
    $code = $LASTEXITCODE
    Pop-Location
    if ($code -ne 0) { throw 'approve failed' }
  }

  $dest = Join-Path $root "$Out\.devlatch"
  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  Copy-Item "$work\.devlatch\traffic.jsonl" $dest -Force
  if (-not $Mutate) { Copy-Item "$work\.devlatch\contracts.json" $dest -Force }
  Write-Host "Wrote fixture to $Out\.devlatch"
}
finally {
  if ($be -and -not $be.HasExited) { Stop-Process -Id $be.Id -Force }
  if ($rec -and -not $rec.HasExited) { Stop-Process -Id $rec.Id -Force }
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}