<#
.SYNOPSIS
    【开发模式】停止并清理 PrivShield 容器 (Windows 11 PowerShell 原生支持)
    Stop and remove PrivShield Docker container for Windows 11 / PowerShell

.DESCRIPTION
    执行步骤总览：
      1. 执行 docker rm -f 强制停止并删除 PrivShield 开发容器
      2. 输出清理完成结果

.EXAMPLE
    .\scripts\dev\docker-stop-agent.ps1
#>

[CmdletBinding()]
param ()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛑 [Docker Mode] 正在停止 PrivShield 容器 (Windows 11)..." -ForegroundColor Yellow
Write-Host "============================================================================" -ForegroundColor Cyan

& docker rm -f PrivShield 2>$null | Out-Null

Write-Host "✅ PrivShield 容器已成功停止与清理！" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
