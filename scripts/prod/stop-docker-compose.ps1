<#
.SYNOPSIS
    【生产模式】PrivShield Docker Compose 停服脚本 (Windows 11 PowerShell 原生支持)
    Gracefully stop PrivShield Production Docker Compose Stack for Windows 11 / PowerShell

.DESCRIPTION
    执行步骤总览：
      1. 定位 Docker Compose 编排目录
      2. 执行 docker compose down 停止全部服务容器（含 llm/monitoring 可选组件）
      3. 根据 -Volumes 参数决定是否连同持久化数据卷一并清理
      4. 输出停服成功结果

.PARAMETER Volumes
    同时清理持久化数据卷 (慎用)

.PARAMETER WithPostgres
    同时停止 Phase B PostgreSQL 容器
#>

[CmdletBinding()]
param (
    [switch]$Volumes,
    [switch]$WithPostgres,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host "用法 / Usage: .\scripts\prod\stop-docker-compose.ps1 [-Volumes] [-WithPostgres]"
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$ComposeDir = Join-Path $ProjectRoot "deploy\docker-compose"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛑 正在停止 PrivShield 生产容器集群..." -ForegroundColor Yellow
Write-Host "============================================================================" -ForegroundColor Cyan

Set-Location $ComposeDir

$VolArg = if ($Volumes) { "--volumes" } else { "" }
$Profiles = @("--profile", "llm", "--profile", "monitoring")
if ($WithPostgres) { $Profiles += @("--profile", "phase-b") }
& docker compose @Profiles down $VolArg

Write-Host ""
Write-Host "✅ PrivShield 生产容器集群已成功停止。" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
