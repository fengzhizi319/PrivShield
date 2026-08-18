<#
.SYNOPSIS
    【开发模式】停止并清理 vLLM Layer-3 LLM 推理容器 (Windows 11 PowerShell 原生支持)
    Stop and remove vLLM Layer-3 LLM inference container for Windows 11 / PowerShell

.DESCRIPTION
    执行步骤总览：
      1. 切换到 Docker Compose 编排目录
      2. 停止并强制清理 PrivShield-vllm 容器
      3. 输出停止与清理结果

.EXAMPLE
    .\scripts\dev\docker-stop-llm.ps1
#>

[CmdletBinding()]
param ()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$ComposeDir = Join-Path $ProjectRoot "deploy\docker-compose"

Write-Host "============================================================================" -ForegroundColor Cyan
Write-Host "🛑 [Docker Mode] 正在停止 vLLM 大模型推理容器 (Windows 11)..." -ForegroundColor Yellow
Write-Host "============================================================================" -ForegroundColor Cyan

Set-Location $ComposeDir
& docker compose --profile llm stop vllm 2>$null | Out-Null
& docker rm -f PrivShield-vllm 2>$null | Out-Null

Write-Host "✅ vLLM 大模型推理容器已成功停止与清理！" -ForegroundColor Green
Write-Host "============================================================================" -ForegroundColor Cyan
