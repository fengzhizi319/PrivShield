# ============================================================================
# [DEV MODE] Launch Go gRPC Console with Vite HMR - Windows PowerShell
#
# Usage: powershell -ExecutionPolicy Bypass -File console\scripts\dev-start-go.ps1
#
# Components:
#   1. PrivShield Agent  (REST: 8079, gRPC: 50051)
#   2. Go gRPC Proxy     (API: 8081)
#   3. Vite Frontend     (UI: 5173, HMR hot-reload)
# ============================================================================

$ErrorActionPreference = "Stop"

$ScriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ConsoleDir  = Split-Path -Parent $ScriptDir
$ProjectRoot = Split-Path -Parent $ConsoleDir

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
$GoBackendDir = Join-Path $ConsoleDir "backend-go"
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

Write-Host "  Starting PrivShield (REST: $AgentUrl, gRPC: 127.0.0.1:50051)..." -ForegroundColor Yellow
$env:PYTHONPATH = $ProjectRoot
$AgentProcess = Start-Process -FilePath $PythonExe `
    -ArgumentList "-m", "PrivShield.server" `
    -WorkingDirectory $ProjectRoot `
    -RedirectStandardOutput $AgentLog `
    -RedirectStandardError (Join-Path $LogsDir "agent_go_err.log") `
    -PassThru -NoNewWindow
Write-Host "  Agent PID: $($AgentProcess.Id)" -ForegroundColor Green

# ── 4. Wait for Agent ───────────────────────────────────────────────
Write-Host "  Waiting for PrivShield REST..." -ForegroundColor Yellow -NoNewline
$ready = $false
for ($i = 1; $i -le 30; $i++) {
    try {
        $r = Invoke-WebRequest -Uri "$AgentUrl/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq 200) { Write-Host " OK" -ForegroundColor Green; $ready = $true; break }
    } catch { }
    Write-Host "." -NoNewline
    Start-Sleep -Seconds 1
}
if (-not $ready) {
    Write-Host " TIMEOUT! Check: $AgentLog" -ForegroundColor Red
    Stop-Process -Id $AgentProcess.Id -Force -ErrorAction SilentlyContinue
    exit 1
}

# ── 5. Start Go backend ─────────────────────────────────────────────
Write-Host "  Starting Go proxy (API: $ConsoleUrl)..." -ForegroundColor Yellow
$GoExePath = Join-Path (Join-Path $GoBackendDir "bin") "backend-go.exe"
$ConsoleProcess = Start-Process -FilePath $GoExePath `
    -WorkingDirectory $GoBackendDir `
    -RedirectStandardOutput (Join-Path $LogsDir "console_go.log") `
    -RedirectStandardError (Join-Path $LogsDir "console_go_err.log") `
    -PassThru -NoNewWindow
Write-Host "  Go backend PID: $($ConsoleProcess.Id)" -ForegroundColor Green

Write-Host "  Waiting for Go backend..." -ForegroundColor Yellow -NoNewline
$ready = $false
for ($i = 1; $i -le 30; $i++) {
    try {
        $r = Invoke-WebRequest -Uri "$ConsoleUrl/api/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq 200) { Write-Host " OK" -ForegroundColor Green; $ready = $true; break }
    } catch { }
    Write-Host "." -NoNewline
    Start-Sleep -Seconds 1
}
if (-not $ready) { Write-Host " TIMEOUT!" -ForegroundColor Red }

# ── 6. Start Vite dev server ────────────────────────────────────────
Write-Host "  Starting Vite dev server (HMR)..." -ForegroundColor Yellow
Push-Location $WebDir
$ViteProcess = Start-Process -FilePath "node" `
    -ArgumentList "node_modules/vite/bin/vite.js" `
    -WorkingDirectory $WebDir `
    -RedirectStandardOutput (Join-Path $LogsDir "vite_dev.log") `
    -RedirectStandardError (Join-Path $LogsDir "vite_dev_err.log") `
    -PassThru -NoNewWindow
Pop-Location
Write-Host "  Vite PID: $($ViteProcess.Id)" -ForegroundColor Green

Write-Host "  Waiting for Vite..." -ForegroundColor Yellow -NoNewline
for ($i = 1; $i -le 30; $i++) {
    try {
        $r = Invoke-WebRequest -Uri $ViteUrl -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq 200) { Write-Host " OK" -ForegroundColor Green; break }
    } catch { }
    Write-Host "." -NoNewline
    Start-Sleep -Seconds 1
}

# ── Summary ─────────────────────────────────────────────────────────
Write-Host ""
Write-Host "======================================================================" -ForegroundColor Cyan
Write-Host "  [DEV MODE] Go gRPC console is UP!" -ForegroundColor Green
Write-Host "======================================================================" -ForegroundColor Cyan
Write-Host "  Frontend UI (Vite HMR): $ViteUrl"
Write-Host "  Go proxy (API):         $ConsoleUrl"
Write-Host "  Agent REST:             $AgentUrl"
Write-Host "  Agent gRPC:             127.0.0.1:50051"
Write-Host "----------------------------------------------------------------------"
Write-Host "  Agent PID: $($AgentProcess.Id) | Go PID: $($ConsoleProcess.Id) | Vite PID: $($ViteProcess.Id)"
Write-Host "  Press Ctrl+C to stop all services"
Write-Host "======================================================================" -ForegroundColor Cyan

# ── 7. Wait & cleanup ───────────────────────────────────────────────
try { Wait-Process -Id $AgentProcess.Id -ErrorAction SilentlyContinue } catch { }

Write-Host ""
Write-Host "Stopping all services..." -ForegroundColor Yellow
Stop-Process -Id $AgentProcess.Id   -Force -ErrorAction SilentlyContinue
Stop-Process -Id $ConsoleProcess.Id -Force -ErrorAction SilentlyContinue
Stop-Process -Id $ViteProcess.Id    -Force -ErrorAction SilentlyContinue
Write-Host "All services stopped." -ForegroundColor Green
