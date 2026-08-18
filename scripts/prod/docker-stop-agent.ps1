<#
.SYNOPSIS
    【生产模式】停止并清理 PrivShield 生产级 Agent 容器 (Windows 11 PowerShell 原生支持)
    Stop and remove Standalone Production PrivShield Agent container for Windows 11 / PowerShell

.EXAMPLE
    .\scripts\prod\docker-stop-agent.ps1
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ContainerName = "PrivShield-prod-agent"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛑 【生产模式】正在安全停止 PrivShield 生产级 Agent 容器 ($ContainerName)..." -ForegroundColor Cyan
Write-Host "============================================================================" -ForegroundColor Cyan

$Running = & docker ps -a --format '{{.Names}}' | Where-Object { $_ -eq $ContainerName }
if ($Running) {
    & docker stop $ContainerName 2>$null | Out-Null
    & docker rm $ContainerName 2>$null | Out-Null
    Write-Host "✅ 容器 $ContainerName 已成功停止并清理。" -ForegroundColor Green
} else {
    Write-Host "ℹ️  容器 $ContainerName 未在运行。" -ForegroundColor Yellow
}

Write-Host "============================================================================" -ForegroundColor Cyan
