<#
.SYNOPSIS
    【开发模式】启动 Layer-3 LLM 推理服务 (Windows 11 PowerShell 原生支持)
    Launch vLLM Layer-3 LLM inference container for Windows 11 / PowerShell

.DESCRIPTION
    使用 Docker Compose 在独立容器中拉起 vLLM GPU 推理服务（服务名: vllm，端口: 8000）。

    执行步骤总览：
      1. 检查 Docker 可用性与 Docker Engine 连通性
      2. 检查本地大模型权重目录存在性并输出下载指引
      3. 使用 Docker Compose profile 'llm' 启动 vLLM 独立容器
      4. 输出容器状态与 OpenAI 兼容接口访问地址

.EXAMPLE
    .\scripts\dev\docker-start-llm.ps1
#>

[CmdletBinding()]
param (
    [Parameter(Position = 0)]
    [ValidateSet("start", "help", "--help", "-h")]
    [string]$Action = "start"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Action -in @("help", "--help", "-h")) {
    Write-Host "用法 / Usage: .\scripts\dev\docker-start-llm.ps1"
    Write-Host ""
    Write-Host "功能说明 / Description:"
    Write-Host "  使用 Docker Compose 在独立容器中启动 vLLM 大模型服务 (端口 8000)。"
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
Write-Host "🤖 [Docker Mode] 正在使用 Docker Compose 启动 vLLM 本地大模型服务" -ForegroundColor Cyan
Write-Host "   • 平台环境 : Windows 11 (PowerShell / Docker Desktop)" -ForegroundColor Cyan
Write-Host "============================================================================" -ForegroundColor Cyan

# 3. 检查模型目录
$ModelDir = Join-Path $ProjectRoot ".models\Qwen3.5-0.8B-Privacy-Classifier-Smoother"
if (-not (Test-Path $ModelDir)) {
    Write-Host "⚠️  [提示] 本地大模型权重目录不存在: $ModelDir" -ForegroundColor Yellow
    Write-Host "   建议先执行模型下载命令以获取微调权重:" -ForegroundColor Yellow
    Write-Host "   python scripts/models/download_model.py" -ForegroundColor Yellow
    Write-Host ""
}

$ComposeDir = Join-Path $ProjectRoot "deploy\docker-compose"
Set-Location $ComposeDir

# 4. 启动 vLLM 服务
& docker compose --profile llm up -d vllm
if ($LASTEXITCODE -ne 0) {
    Write-Error "❌ vLLM 容器启动失败！"
    exit $LASTEXITCODE
}

Write-Host ""
Write-Host "✅ vLLM 大模型推理容器已启动！" -ForegroundColor Green
Write-Host "   - OpenAI 兼容接口 : http://127.0.0.1:8000/v1" -ForegroundColor Green
Write-Host "   - 查看日志        : docker logs -f PrivShield-vllm" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
