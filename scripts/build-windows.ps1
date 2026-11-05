# Builds NetPilot Windows release: NetPilot GUI + official upstream net-merge binaries + Inno Setup installer + portable zip
param(
    [string]$Version = '0.1.0',
    [string]$RepoUrl = ''
)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

function Invoke-Native {
    param([string]$What, [scriptblock]$Cmd)
    & $Cmd
    if ($LASTEXITCODE -ne 0) { throw "$What failed (exit $LASTEXITCODE)" }
}

if ($Version -notmatch '^\d+\.\d+\.\d+$') { throw "Version must look like 1.2.3, got '$Version'" }

if (Test-Path dist) { Remove-Item dist -Recurse -Force }
New-Item -ItemType Directory dist | Out-Null

# 1. Build frontend
Write-Host "Building frontend..."
Push-Location frontend
Invoke-Native 'npm build' { cmd /c "npm ci || npm install && npm run build" }
Pop-Location

# 2. Build NetPilot GUI
Write-Host "Building NetPilot GUI binary..."
Invoke-Native 'build netpilot' { go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=$Version" -o dist\netpilot.exe .\cmd\netpilot }

# 3. Pull latest official net-merge release binaries from net-merge/net-merge
Write-Host "Fetching latest official net-merge binaries..."
$tempDir = Join-Path $env:TEMP "net-merge-dl"
if (Test-Path $tempDir) { Remove-Item $tempDir -Recurse -Force }
New-Item -ItemType Directory $tempDir | Out-Null

Push-Location $tempDir
gh release download --repo net-merge/net-merge --pattern 'net-merge-portable.zip'
Expand-Archive -Path net-merge-portable.zip -DestinationPath extracted -Force
Pop-Location

Copy-Item "$tempDir\extracted\net-merge.exe" dist\net-merge.exe -Force
Copy-Item "$tempDir\extracted\net-merge-tray.exe" dist\net-merge-tray.exe -Force
Copy-Item "$tempDir\extracted\wintun.dll" dist\wintun.dll -Force
Copy-Item "$tempDir\extracted\wintun-LICENSE.txt" dist\wintun-LICENSE.txt -Force
Copy-Item packaging\netpilot.ico dist\netpilot.ico -Force
Remove-Item $tempDir -Recurse -Force

# 4. Inno Setup installer (.exe)
$iscc = $env:ISCC
if (-not $iscc) { $iscc = (Get-Command ISCC.exe -ErrorAction SilentlyContinue).Source }
if (-not $iscc) {
    $iscc = Get-ChildItem 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe', 'C:\Program Files\Inno Setup 6\ISCC.exe' -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty FullName
}
if ($iscc) {
    Write-Host "Building Inno Setup installer using $iscc..."
    $isccArgs = @("/DVersion=$Version")
    if ($RepoUrl) { $isccArgs += "/DRepoUrl=$RepoUrl" }
    Invoke-Native 'inno setup' { & $iscc @isccArgs installer\netpilot.iss }
} else {
    Write-Warning "ISCC.exe not found. Skipping Inno Setup installer build."
}

# 5. Portable ZIP
$portable = "dist\netpilot-$Version-windows-amd64-portable.zip"
Compress-Archive -Path dist\netpilot.exe, dist\net-merge.exe, dist\net-merge-tray.exe, dist\wintun.dll, dist\wintun-LICENSE.txt, dist\netpilot.ico, LICENSE, THIRD-PARTY-NOTICES.md, README.md -DestinationPath $portable -Force

Get-ChildItem dist\*.exe, dist\*.zip | ForEach-Object { '{0}  {1:N1} MB' -f $_.Name, ($_.Length / 1MB) }
