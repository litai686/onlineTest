<#
.SYNOPSIS
Builds the onlineTest server for Windows and Linux.

.DESCRIPTION
Cross-compiles a single binary per target with the embedded web assets.
By default builds windows/amd64 and linux/amd64 into dist/<os>-<arch>/.
CGO is disabled so the pure-Go SQLite driver works on every target.

.EXAMPLE
.\build.ps1

.EXAMPLE
.\build.ps1 -Platforms windows -Architectures amd64,arm64

.EXAMPLE
.\build.ps1 -Test
#>
[CmdletBinding()]
param(
    [string[]]$Platforms = @("windows", "linux"),

    [string[]]$Architectures = @("amd64"),

    [string]$OutputDir = "dist",

    [switch]$Test,

    [switch]$NoStrip,

    [switch]$NoClean
)

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go toolchain not found. Install Go 1.26.4+ and make sure 'go' is in PATH."
}

Write-Host "Using $(& go version)"

if ($Test) {
    Write-Host "`n==> go test ./..."
    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed."
    }
}

$targetPlatforms = @($Platforms | ForEach-Object { $_ -split ',' } | ForEach-Object { $_.Trim().ToLowerInvariant() } | Where-Object { $_ } | Select-Object -Unique)
$targetArchs = @($Architectures | ForEach-Object { $_ -split ',' } | ForEach-Object { $_.Trim().ToLowerInvariant() } | Where-Object { $_ } | Select-Object -Unique)

if ($targetPlatforms.Count -eq 0) {
    throw "At least one platform is required. Supported: windows, linux."
}
if ($targetArchs.Count -eq 0) {
    throw "At least one architecture is required. Supported: amd64, arm64."
}
$badPlatform = @($targetPlatforms | Where-Object { $_ -notin @("windows", "linux") })
if ($badPlatform.Count -gt 0) {
    throw "Unsupported platform: $($badPlatform -join ', '). Supported: windows, linux."
}
$badArch = @($targetArchs | Where-Object { $_ -notin @("amd64", "arm64") })
if ($badArch.Count -gt 0) {
    throw "Unsupported architecture: $($badArch -join ', '). Supported: amd64, arm64."
}

$outRoot = Join-Path $PSScriptRoot $OutputDir
New-Item -ItemType Directory -Force -Path $outRoot | Out-Null

$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
$oldCgo = $env:CGO_ENABLED
$results = @()

try {
    $env:CGO_ENABLED = "0"

    foreach ($targetOs in $targetPlatforms) {
        foreach ($targetArch in $targetArchs) {
            $targetDir = Join-Path $outRoot "$targetOs-$targetArch"
            $fileName = if ($targetOs -eq "windows") { "onlineTest.exe" } else { "onlineTest" }
            $output = Join-Path $targetDir $fileName
            if (-not $NoClean -and (Test-Path -LiteralPath $output)) {
                try {
                    Remove-Item -LiteralPath $output -Force
                }
                catch {
                    $holders = @(Get-Process -ErrorAction SilentlyContinue | Where-Object {
                        try { $_.Path -eq $output } catch { $false }
                    })
                    if ($holders.Count -gt 0) {
                        $ids = ($holders | ForEach-Object { $_.Id }) -join ", "
                        throw "目标文件正在运行（PID $ids），无法覆盖。请先停止服务再构建：Stop-Process -Id $ids"
                    }
                    throw
                }
            }
            New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

            $env:GOOS = $targetOs
            $env:GOARCH = $targetArch

            $buildArgs = @("build", "-trimpath", "-o", $output)
            if (-not $NoStrip) {
                $buildArgs += @("-ldflags", "-s -w")
            }
            $buildArgs += "./cmd/server"

            Write-Host "`n==> Building $targetOs/$targetArch"
            Write-Host "    go $($buildArgs -join ' ')"
            & go @buildArgs
            if ($LASTEXITCODE -ne 0) {
                throw "Build failed for $targetOs/$targetArch."
            }

            $file = Get-Item -LiteralPath $output
            $results += [pscustomobject]@{
                Platform = $targetOs
                Arch     = $targetArch
                File     = $output
                SizeMB   = [math]::Round($file.Length / 1MB, 2)
            }
        }
    }
}
finally {
    if ($null -eq $oldGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $oldGoos }
    if ($null -eq $oldGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $oldGoarch }
    if ($null -eq $oldCgo) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $oldCgo }
}

Write-Host "`nBuild complete:"
$results | Format-Table -AutoSize
