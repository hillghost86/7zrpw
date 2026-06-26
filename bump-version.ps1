# 7zrpw version helper.
# Single source of version = client\versioninfo.json. This script reads the current
# version, asks whether to update, validates the format, and writes the new version
# back to ALL version fields in versioninfo.json (numeric parts + string parts).
# Called by build.bat; also supports -NewVersion for non-interactive use (tests/CI).
# NOTE: keep this file ASCII only -- a no-BOM .ps1 with non-ASCII chars is misparsed
# by Windows PowerShell 5.1 under a non-UTF8 system codepage.
param(
    [string]$Path = (Join-Path $PSScriptRoot 'client\versioninfo.json'),
    [string]$NewVersion
)
$ErrorActionPreference = 'Stop'

function Test-VerFormat([string]$v) {
    return $v -match '^v[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?$'
}

# Write version back into versioninfo.json via in-place regex replace, preserving the
# rest of the JSON structure and writing UTF-8 without BOM.
function Set-Version([string]$p, [string]$v) {
    $raw = Get-Content $p -Raw -Encoding UTF8
    $n = $v.TrimStart('v').Split('.')
    $bld = if ($n.Count -ge 4) { [int]$n[3] } else { 0 }
    $num = '"Major":' + [int]$n[0] + ',"Minor":' + [int]$n[1] + ',"Patch":' + [int]$n[2] + ',"Build":' + $bld
    $raw = $raw -replace '"Major":\d+,"Minor":\d+,"Patch":\d+,"Build":\d+', $num
    $raw = $raw -replace '("FileVersion":")v[^"]*"', ('${1}' + $v + '"')
    $raw = $raw -replace '("ProductVersion":")v[^"]*"', ('${1}' + $v + '"')
    [System.IO.File]::WriteAllText($p, $raw, (New-Object System.Text.UTF8Encoding($false)))
}

$raw = Get-Content $Path -Raw -Encoding UTF8
$cur = ([regex]'"FileVersion":"(v[^"]*)"').Match($raw).Groups[1].Value

# Non-interactive: apply directly (validate then write).
if ($NewVersion) {
    $NewVersion = $NewVersion.Trim()
    if (-not (Test-VerFormat $NewVersion)) {
        Write-Host "[ERROR] Invalid version format: $NewVersion (expect vX.Y.Z or vX.Y.Z.W)"
        exit 1
    }
    Set-Version $Path $NewVersion
    Write-Host "Version updated: $cur -> $NewVersion"
    exit 0
}

# Interactive mode.
Write-Host "============================================"
Write-Host " Current version: $cur"
Write-Host "============================================"
$ans = Read-Host " Update version? (Y/N) [N]"
if ($ans -notmatch '^(y|yes)$') {
    Write-Host "Keep current version: $cur"
    exit 0
}
while ($true) {
    $new = Read-Host " Enter new version (vX.Y.Z or vX.Y.Z.W, e.g. v0.1.6.1)"
    if ([string]::IsNullOrWhiteSpace($new)) {
        Write-Host "No input, keep current version: $cur"
        exit 0
    }
    $new = $new.Trim()
    if (Test-VerFormat $new) { break }
    Write-Host "[ERROR] Invalid format, expect vX.Y.Z or vX.Y.Z.W (digits). Try again."
}
Set-Version $Path $new
Write-Host "Version updated: $cur -> $new (versioninfo.json synced)"
exit 0
