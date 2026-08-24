<#
.SYNOPSIS
    【生产模式】单组分独立启动 PrivShield 核心 Agent 容器 (Windows 11 PowerShell 原生支持)
    Launch Standalone PrivShield Core/ML Agent in Production Docker Container for Windows 11 / PowerShell

.DESCRIPTION
    以生产安全加固配置（资源限制、JSON 日志、持久化卷、restart=always）启动 PrivShield 容器。

.PARAMETER Target
    构建目标: 'core' (默认轻量版) 或 'ml' (含 PyTorch/Transformers/ONNX 完整版)

.PARAMETER RestPort
    REST API 宿主机映射端口 (默认: 8079)

.PARAMETER GrpcPort
    gRPC RPC 宿主机映射端口 (默认: 50051)

.PARAMETER NoBuild
    跳过构建，直接使用本地已有镜像

.EXAMPLE
    .\scripts\prod\docker-start-agent.ps1
    .\scripts\prod\docker-start-agent.ps1 -Target ml
    .\scripts\prod\docker-start-agent.ps1 -RestPort 9090
#>

[CmdletBinding()]
param (
    [Parameter(Position = 0)]
    [ValidateSet("core", "ml", "help", "--help", "-h")]
    [string]$Target = "core",

    [Parameter()]
    [string]$RestPort = "8079",

    [Parameter()]
    [string]$GrpcPort = "50051",

    [Parameter()]
    [switch]$NoBuild = $false
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Target -in @("help", "--help", "-h")) {
    Write-Host "用法 / Usage: .\scripts\prod\docker-start-agent.ps1 [core|ml] [-RestPort 8079] [-GrpcPort 50051] [-NoBuild]"
    Write-Host ""
    Write-Host "构建目标 / Targets:"
    Write-Host "  core   (默认) 轻量 Core 镜像（仅含 FastAPI/gRPC 基础依赖）"
    Write-Host "  ml     完整 ML 镜像（包含 PyTorch/Transformers/ONNX 等重量级依赖）"
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
Write-Host "🛡️  【生产模式】独立启动 PrivShield 核心 Agent 容器" -ForegroundColor Cyan
Write-Host "   • 镜像类型 : $Target" -ForegroundColor Cyan
Write-Host "   • REST 端口 : $RestPort" -ForegroundColor Cyan
Write-Host "   • gRPC 端口 : $GrpcPort" -ForegroundColor Cyan
Write-Host "============================================================================" -ForegroundColor Cyan

Set-Location $ProjectRoot

# 3. 准备持久化目录
$DataBudgetDir = Join-Path $ProjectRoot ".data\budget"
$LogsDir = Join-Path $ProjectRoot ".logs"
if (-not (Test-Path $DataBudgetDir)) { New-Item -ItemType Directory -Path $DataBudgetDir -Force | Out-Null }
if (-not (Test-Path $LogsDir)) { New-Item -ItemType Directory -Path $LogsDir -Force | Out-Null }

# 4. 构建镜像（若未指定 -NoBuild）
if ($Target -eq "ml") {
    $ImageName = "privshield:1.8.0-ml"
    $Cpus = "4.0"
    $Memory = "8g"
    if (-not $NoBuild) {
        Write-Host "📦 正在构建生产级 ML 镜像 ($ImageName)..." -ForegroundColor Yellow
        & docker build --target ml -t $ImageName .
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
} else {
    $ImageName = "privshield:1.8.0"
    $Cpus = "2.0"
    $Memory = "2g"
    if (-not $NoBuild) {
        Write-Host "📦 正在构建生产级 Core 镜像 ($ImageName)..." -ForegroundColor Yellow
        & docker build --target core -t $ImageName .
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
}

# 5. 清理旧同名容器
$ContainerName = "PrivShield-prod-agent"
& docker rm -f $ContainerName 2>$null | Out-Null

# 6. 启动生产容器
Write-Host ""
Write-Host "🚀 正在启动生产级 Agent 物理容器 ($ContainerName)..." -ForegroundColor Cyan

& docker run -d `
  --name $ContainerName `
  --restart=always `
  --security-opt=no-new-privileges:true `
  --cpus=$Cpus `
  --memory=$Memory `
  -p "${RestPort}:8079" `
  -p "${GrpcPort}:50051" `
  -e PRIVACY_REST_HOST="0.0.0.0" `
  -e PRIVACY_REST_PORT="8079" `
  -e PRIVACY_GRPC_HOST="0.0.0.0" `
  -e PRIVACY_GRPC_PORT="50051" `
  -e PRIVACY_LOG_FORMAT="json" `
  -e PRIVACY_PROFILE="/etc/PrivShield/privacy-profile.yaml" `
  -e PRIVACY_BUDGET_DB="/data/budget/budget.db" `
  -v "${DataBudgetDir}:/data/budget" `
  -v "${LogsDir}:/var/log/privacy" `
  $ImageName

if ($LASTEXITCODE -ne 0) {
    Write-Error "❌ 容器启动失败！"
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "✅ PrivShield 生产级 Agent 已成功启动！" -ForegroundColor Green
Write-Host "   - REST API : http://127.0.0.1:$RestPort (或 https://127.0.0.1:$RestPort)" -ForegroundColor Green
Write-Host "   - gRPC RPC : 127.0.0.1:$GrpcPort" -ForegroundColor Green
Write-Host "   - 容器名称 : $ContainerName" -ForegroundColor Green
Write-Host "   - 日志查看 : docker logs -f $ContainerName" -ForegroundColor Green
Write-Host "   - 容器停止 : .\scripts\prod\docker-stop-agent.ps1" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
