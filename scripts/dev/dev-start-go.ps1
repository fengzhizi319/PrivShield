# ============================================================================
# [DEV MODE] Launch Go gRPC Console with Vite HMR - Windows PowerShell
#
# Usage: powershell -ExecutionPolicy Bypass -File scripts\dev\dev-start-go.ps1
#
# Components:
#   1. PrivShield Engine (REST: 8079, gRPC: 50051)
#   2. Go gRPC Proxy     (API: 8081)
#   3. Vite Frontend     (UI: 5173, HMR hot-reload)
# ============================================================================

$ErrorActionPreference = "Stop"

$ScriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $ScriptDir)
$ConsoleDir  = Join-Path $ProjectRoot "console"

$AgentUrl   = "http://127.0.0.1:8079"
$ConsoleUrl = "http://127.0.0.1:8081"
$ViteUrl    = "http://localhost:5173"

# Refresh PATH so newly installed tools (Go) are visible
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
            [System.Environment]::GetEnvironmentVariable("Path", "User")

# ── 0. Prerequisites ────────────────────────────────────────────────
Write-Host ""
Write-Host "====== Prerequisites ======" -ForegroundColor Cyan

# Python: prefer conda privshield env
$PythonExe = $null
$CondaPy = "$env:USERPROFILE\miniconda3\envs\privshield\python.exe"
if (Test-Path $CondaPy) {
    $PythonExe = $CondaPy
    Write-Host "  Python: $PythonExe (conda privshield)" -ForegroundColor Green
} else {
    $PythonExe = (Get-Command python -ErrorAction SilentlyContinue).Source
    if ($PythonExe) {
        Write-Host "  Python: $PythonExe" -ForegroundColor Yellow
    } else {
        Write-Host "  ERROR: Python not found. Install Python 3.13+ first." -ForegroundColor Red
        exit 1
    }
}

# Go
$GoExe = (Get-Command go -ErrorAction SilentlyContinue).Source
if ($GoExe) {
    Write-Host "  Go: $(& go version)" -ForegroundColor Green
} else {
    Write-Host "  ERROR: Go not found. Run: winget install GoLang.Go" -ForegroundColor Red
    exit 1
}

# Node
$NodeExe = (Get-Command node -ErrorAction SilentlyContinue).Source
if ($NodeExe) {
    Write-Host "  Node: $(& node --version)" -ForegroundColor Green
} else {
    Write-Host "  ERROR: Node.js not found." -ForegroundColor Red
    exit 1
}

# ── 1. Frontend dependencies ────────────────────────────────────────
Write-Host ""
Write-Host "====== Frontend deps ======" -ForegroundColor Cyan
$WebDir      = Join-Path $ConsoleDir "web"
$NodeModules = Join-Path $WebDir "node_modules"

if (-not (Test-Path $NodeModules)) {
    Write-Host "  Installing frontend dependencies..." -ForegroundColor Yellow
    Push-Location $WebDir
    try {
        $pnpm = Get-Command pnpm -ErrorAction SilentlyContinue
        if ($pnpm) { & pnpm install }
        else       { & corepack pnpm install }
    } finally { Pop-Location }
} else {
    Write-Host "  node_modules exists, skip." -ForegroundColor Green
}

# ── 2. Build Go backend ─────────────────────────────────────────────
Write-Host ""
Write-Host "====== Build Go backend ======" -ForegroundColor Cyan
$GoBackendDir = Join-Path $ConsoleDir "bff-go"
$GoBinDir     = Join-Path $GoBackendDir "bin"
if (-not (Test-Path $GoBinDir)) { New-Item -ItemType Directory -Path $GoBinDir -Force | Out-Null }

Push-Location $GoBackendDir
try {
    & go build -o bin/backend-go.exe ./cmd/server
    Write-Host "  Go backend built OK" -ForegroundColor Green
} finally { Pop-Location }

# ── 3. Start Python Agent ───────────────────────────────────────────
Write-Host ""
Write-Host "====== Start services ======" -ForegroundColor Cyan

$LogsDir = Join-Path $ProjectRoot ".logs"
if (-not (Test-Path $LogsDir)) { New-Item -ItemType Directory -Path $LogsDir -Force | Out-Null }
$AgentLog = Join-Path $LogsDir "agent_go.log"

$PidsDir = Join-Path $ProjectRoot ".pids"
if (-not (Test-Path $PidsDir)) { New-Item -ItemType Directory -Path $PidsDir -Force | Out-Null }

Write-Host "  Starting PrivShield Engine (port 8079 / 50051)..." -ForegroundColor Yellow

$AgentProcess = Start-Process `
    -FilePath $PythonExe `
    -ArgumentList "-m", "engine.server" `
    -WorkingDirectory $ProjectRoot `
    -RedirectStandardOutput $AgentLog `
    -RedirectStandardError (Join-Path $LogsDir "agent_go.err.log") `
    -PassThru

# ── 4. Start Go backend ─────────────────────────────────────────────
Write-Host "  Starting Go gRPC Proxy (port 8081)..." -ForegroundColor Yellow

$GoExeFile = Join-Path $GoBinDir "backend-go.exe"
$GoProcess = Start-Process `
    -FilePath $GoExeFile `
    -WorkingDirectory $GoBackendDir `
    -RedirectStandardOutput (Join-Path $LogsDir "console_go.log") `
    -RedirectStandardError (Join-Path $LogsDir "console_go.err.log") `
    -PassThru

# ── 5. Start Vite Dev Server ────────────────────────────────────────
Write-Host "  Starting Vite Frontend (port 5173)..." -ForegroundColor Yellow

$PnpmCmd = Get-Command pnpm -ErrorAction SilentlyContinue
$ViteProcess = $null
if ($PnpmCmd) {
    $ViteProcess = Start-Process `
        -FilePath $PnpmCmd.Source `
        -ArgumentList "dev" `
        -WorkingDirectory $WebDir `
        -PassThru
} else {
    $ViteProcess = Start-Process `
        -FilePath "cmd.exe" `
        -ArgumentList "/c corepack pnpm dev" `
        -WorkingDirectory $WebDir `
        -PassThru
}

# ── 6. Ready ────────────────────────────────────────────────────────
Write-Host ""
Write-Host "======================================================================" -ForegroundColor Green
Write-Host "  PrivShield Dev Console is UP!" -ForegroundColor Green
Write-Host "  UI (Vite HMR) : $ViteUrl" -ForegroundColor Cyan
Write-Host "  Go Backend    : $ConsoleUrl" -ForegroundColor White
Write-Host "  Agent REST    : $AgentUrl" -ForegroundColor White
Write-Host "  Agent gRPC    : 127.0.0.1:50051" -ForegroundColor White
Write-Host "======================================================================" -ForegroundColor Green
Write-Host "  Press Ctrl+C to stop all services..." -ForegroundColor Yellow

# ── Cleanup on exit ─────────────────────────────────────────────────
try {
    while ($true) {
        Start-Sleep -Seconds 1
    }
} finally {
    Write-Host "`nStopping services..." -ForegroundColor Yellow
    foreach ($p in @($AgentProcess, $GoProcess, $ViteProcess)) {
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
    }
    Write-Host "All services stopped." -ForegroundColor Green
}
