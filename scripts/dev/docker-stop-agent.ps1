<#
.SYNOPSIS
    【Docker 模式】停止并清理 Privacy Local Agent 容器 (Windows 11 PowerShell 原生支持)
    Stop and remove Privacy Local Agent Docker container for Windows 11 / PowerShell

.EXAMPLE
    .\scripts\dev\docker-stop-agent.ps1
#>

[CmdletBinding()]
param ()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛑 [Docker Mode] 正在停止 Privacy Local Agent 容器 (Windows 11)..." -ForegroundColor Yellow
Write-Host "============================================================================" -ForegroundColor Cyan

& docker rm -f privacy-local-agent 2>$null | Out-Null

Write-Host "✅ Privacy Local Agent 容器已成功停止与清理！" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
