<#
.SYNOPSIS
    【Docker 模式】单组分启动 PrivShield (Windows 11 PowerShell 原生支持)
    Launch PrivShield in Docker container for Windows 11 / PowerShell

.DESCRIPTION
    构建并启动 PrivShield 容器 (core 或 ml 镜像)，暴露 REST (8079) 与 gRPC (50051) 端口。

.PARAMETER Target
    构建目标: 'core' (默认轻量版) 或 'ml' (含 PyTorch/Transformers/ONNX 完整版)

.EXAMPLE
    .\scripts\dev\docker-start-agent.ps1
    .\scripts\dev\docker-start-agent.ps1 -Target ml
    .\scripts\dev\docker-start-agent.ps1 core
#>

[CmdletBinding()]
param (
    [Parameter(Position = 0)]
    [ValidateSet("core", "ml", "help", "--help", "-h")]
    [string]$Target = "core"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Target -in @("help", "--help", "-h")) {
    Write-Host "用法 / Usage: .\scripts\dev\docker-start-agent.ps1 [core|ml]"
    Write-Host ""
    Write-Host "构建目标 / Targets:"
    Write-Host "  core   (默认) 构建并启动轻量 Core 镜像（仅含 FastAPI/gRPC 基础依赖）"
    Write-Host "  ml     构建并启动完整 ML 镜像（包含 PyTorch/Transformers/ONNX 等重量级依赖）"
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "..\..")

# 1. 检查 Docker 可用性
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "❌ [错误] 未检测到 docker 命令，请先安装 Docker Desktop: https://docs.docker.com/desktop/setup/install/windows-install/"
    exit 1
}

# 2. 检查 Docker Daemon 是否运行
try {
    $null = & docker info 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Error "❌ [错误] Docker 守护进程未启动，请先打开 Docker Desktop 并等待 Engine 就绪！"
        exit 1
    }
} catch {
    Write-Error "❌ [错误] 无法连接 Docker 守护进程，请确认 Docker Desktop 正在运行！"
    exit 1
}

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🚀 [Docker Mode] 正在构建并启动 PrivShield" -ForegroundColor Cyan
Write-Host "   • 平台环境 : Windows 11 (PowerShell / Docker Desktop)" -ForegroundColor Cyan
Write-Host "   • 构建目标 : $Target" -ForegroundColor Cyan
Write-Host "============================================================================" -ForegroundColor Cyan

Set-Location $ProjectRoot

# 3. 构建镜像
if ($Target -eq "ml") {
    Write-Host "📦 构建含有 PyTorch / Transformers / ONNX 的 ML 镜像..." -ForegroundColor Yellow
    & docker build --target ml -t privshield:0.1.0-ml .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $ImageName = "privshield:0.1.0-ml"
} else {
    Write-Host "📦 构建轻量 Core 镜像..." -ForegroundColor Yellow
    & docker build --target core -t privshield:0.1.0 .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $ImageName = "privshield:0.1.0"
}

# 4. 清理旧同名容器
& docker rm -f PrivShield 2>$null | Out-Null

# 5. 启动容器
& docker run -d `
  --name PrivShield `
  -p 8079:8079 `
  -p 50051:50051 `
  -e PRIVACY_REST_HOST="0.0.0.0" `
  -e PRIVACY_GRPC_HOST="0.0.0.0" `
  -e PRIVACY_LOG_LEVEL="INFO" `
  $ImageName

if ($LASTEXITCODE -ne 0) {
    Write-Error "❌ 容器启动失败！"
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "✅ PrivShield (Docker) 已成功启动！" -ForegroundColor Green
Write-Host "   - REST API : http://127.0.0.1:8079" -ForegroundColor Green
Write-Host "   - gRPC RPC : 127.0.0.1:50051" -ForegroundColor Green
Write-Host "   - 查看日志 : docker logs -f PrivShield" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
