<#
.SYNOPSIS
    【生产模式】PrivShield Docker Compose 停服脚本 (Windows 11 PowerShell 原生支持)
    Gracefully stop PrivShield Production Docker Compose Stack for Windows 11 / PowerShell

.PARAMETER Volumes
    同时清理持久化数据卷 (慎用)
#>

[CmdletBinding()]
param (
    [switch]$Volumes,
    [switch]$Help
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($Help) {
    Write-Host "用法 / Usage: .\scripts\prod\stop-docker-compose.ps1 [-Volumes]"
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
& docker compose --profile llm --profile monitoring down $VolArg

Write-Host ""
Write-Host "✅ PrivShield 生产容器集群已成功停止。" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
