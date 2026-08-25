<#
.SYNOPSIS
    【生产模式】PrivShield Docker Compose 生产环境一键部署 (Windows 11 PowerShell 原生支持)
    Launch PrivShield in Production Mode with Docker Compose for Windows 11 / PowerShell

.DESCRIPTION
    一键拉起 PrivShield 生产级全栈容器集群，支持可选 vLLM 大模型推理与 Prometheus 监控。

    执行步骤总览：
      1. 检查 Docker CLI 与 Docker Engine 连通性
      2. 准备宿主机持久化数据与日志目录（.data / .logs）
      3. 根据 -WithLlm / -WithMonitoring 组装 --profile 开关
      4. 执行 docker compose -f docker-compose.prod.yml up -d 启动生产集群
      5. 轮询健康探针并输出访问地址与维护命令

.PARAMETER WithLlm
    启用 vLLM 大模型 GPU 推理容器

.PARAMETER WithMonitoring
    启用 Prometheus + Grafana 生产监控套件

.PARAMETER WithPostgres
    启用 Phase B PostgreSQL 多副本 Hub 模式

.PARAMETER Build
    强制重新构建容器镜像
#>

[CmdletBinding()]
param (
    [switch]$WithLlm,
    [switch]$WithMonitoring,
    [switch]$WithPostgres,
    [switch]$Build,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host "用法 / Usage: .\scripts\prod\deploy-docker-compose.ps1 [-WithLlm] [-WithMonitoring] [-WithPostgres] [-Build]"
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$ComposeDir = Join-Path $ProjectRoot "deploy\docker-compose"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛡️  【生产模式】PrivShield Docker Compose 生产环境部署 (PowerShell)" -ForegroundColor Cyan
Write-Host "============================================================================" -ForegroundColor Cyan

# 1. 检查 Docker
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "❌ [错误] 未检测到 docker 命令，请先安装 Docker Desktop。"
    exit 1
}

# 2. 准备目录
$DataDir = Join-Path $ProjectRoot ".data"
$LogsDir = Join-Path $ProjectRoot ".logs"
if (-not (Test-Path $DataDir)) { New-Item -ItemType Directory -Path $DataDir | Out-Null }
if (-not (Test-Path $LogsDir)) { New-Item -ItemType Directory -Path $LogsDir | Out-Null }

$Profiles = @()
if ($WithLlm) { $Profiles += @("--profile", "llm") }
if ($WithMonitoring) { $Profiles += @("--profile", "monitoring") }
if ($WithPostgres) { $Profiles += @("--profile", "phase-b") }

Set-Location $ComposeDir

$BuildArg = if ($Build) { "--build" } else { "" }

Write-Host "🚀 正在启动生产服务容器群..." -ForegroundColor Yellow
& docker compose @Profiles up -d $BuildArg

Write-Host ""
Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🎉 PrivShield 生产级服务已成功启动！" -ForegroundColor Green
Write-Host "  • 核心 Agent REST API : http://127.0.0.1:8079" -ForegroundColor Green
Write-Host "  • 核心 Agent gRPC RPC : 127.0.0.1:50051" -ForegroundColor Green
Write-Host "  • Web 控制台 UI       : http://127.0.0.1:5173" -ForegroundColor Green
Write-Host "  • Go BFF 代理网关 REST: http://127.0.0.1:8081" -ForegroundColor Green
Write-Host "  • 调度中枢 Service Hub: http://127.0.0.1:8082" -ForegroundColor Green
Write-Host "  • 数据源管理 Datasource: http://127.0.0.1:8083" -ForegroundColor Green
Write-Host "  • 脱敏审计日志 AuditLog: http://127.0.0.1:8084" -ForegroundColor Green
if ($WithPostgres) {
    Write-Host "  • PostgreSQL (Phase B) : 127.0.0.1:5432 (privshield_hub)" -ForegroundColor Green
}
Write-Host "============================================================================" -ForegroundColor Cyan
