# 启动本地开发服务器：使用 build.ps1 交叉编译到 dist/<os>-<arch>/ 的二进制。
$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

$exe = Join-Path $PSScriptRoot "dist\windows-amd64\onlineTest.exe"
if (-not (Test-Path -LiteralPath $exe)) {
    Write-Host "未找到 $exe" -ForegroundColor Yellow
    Write-Host "请先运行 .\build.ps1 -Platforms windows 构建（输出到 dist\windows-amd64\）" -ForegroundColor Yellow
    exit 1
}

$env:ONLINETEST_ADMIN_PASSWORD = "admin123"
& $exe @args
exit $LASTEXITCODE
