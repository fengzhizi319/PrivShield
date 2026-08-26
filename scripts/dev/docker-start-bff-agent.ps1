# ============================================================================
# 【Docker 模式】启动控制台三件套（Agent + Go BFF + Web UI）(PowerShell)
# Launch Console Trio (Agent + Go BFF + Web UI) in Docker Compose
#
# 用法 / Usage:
#   .\scripts\dev\docker-start-bff-agent.ps1 [-Build] [-NoBuild] [-Force] [-MTLS] [-NoMTLS]
# ============================================================================

[CmdletBinding()]
param (
    [switch]$NoBuild,
    [switch]$Build,
    [switch]$MTLS,
    [switch]$NoMTLS,
    [switch]$Force,
    [switch]$Help
)

if ($Help) {
    Write-Host "用法 / Usage: .\scripts\dev\docker-start-bff-agent.ps1 [选项 / Options]"
    Write-Host ""
    Write-Host "选项 / Options:"
    Write-Host "  -MTLS      以 mTLS 双向证书认证模式启动 (REST/HTTPS + gRPC/mTLS)"
    Write-Host "  -NoMTLS    以标准明文模式启动 (REST/HTTP + gRPC/Plain) (默认)"
    Write-Host "  -NoBuild   跳过镜像构建，使用本地已有镜像"
    Write-Host "  -Build     启动前重新构建本地镜像 (默认)"
    Write-Host "  -Force     自动释放占用端口"
    Write-Host "  -Help      显示帮助信息"
    exit 0
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$CertDir = Join-Path $ProjectRoot "console\bff-go\certs"

$BuildFlag = "--build"
if ($NoBuild) {
    $BuildFlag = ""
}

Write-Host "============================================================================" -ForegroundColor Cyan
if ($MTLS) {
    Write-Host "🌟 [Docker Mode] 正在启动 PrivShield 控制台套件 (mTLS 双向认证版本)..." -ForegroundColor Cyan
} else {
    Write-Host "🌟 [Docker Mode] 正在启动 PrivShield 控制台套件 (标准非 mTLS 版本)..." -ForegroundColor Cyan
}
Write-Host "============================================================================" -ForegroundColor Cyan

# ── 前置准备：确保前端已构建 ─────────────────────────────────────────────
$DistDir = Join-Path $ProjectRoot "console\web\dist"
if (!(Test-Path $DistDir) -or $Build) {
    Write-Host "📦 准备前端静态资源 (Vite build)..." -ForegroundColor Yellow
    Push-Location (Join-Path $ProjectRoot "console\web")
    try {
        if (Get-Command corepack -ErrorAction SilentlyContinue) {
            corepack pnpm build
        } elseif (Get-Command pnpm -ErrorAction SilentlyContinue) {
            pnpm build
        } else {
            npm run build
        }
    } catch {
        Write-Warning "前端构建失败，将由 Docker 容器内构建"
    } finally {
        Pop-Location
    }
}

# ── 清理可能残留的同名独立单容器 ─────────────────────────────────────────
docker rm -f PrivShield privacy-console-backend-go privacy-console-web 2>$null | Out-Null

# ── 进入 docker-compose 目录，按模式启动容器 ───────────────────────────
Push-Location (Join-Path $ProjectRoot "deploy\docker-compose")

$ComposeFiles = @("-f", "docker-compose.yml")
if ($MTLS) {
    $ComposeFiles += @("-f", "docker-compose.mtls.yml")
}

$AllArgs = $ComposeFiles + @("up", "-d")
if ($BuildFlag) {
    $AllArgs += "--build"
}
$AllArgs += @("PrivShield", "console-backend-go", "console-web")

docker compose @AllArgs

Pop-Location

Write-Host ""
Write-Host "============================================================================" -ForegroundColor Cyan
if ($MTLS) {
    Write-Host " ✨ PrivShield 控制台套件 [mTLS 双向认证版本] 已成功启动！" -ForegroundColor Green
    Write-Host " 🌐 React 控制台 Web UI   : http://localhost:5173" -ForegroundColor Green
    Write-Host " 🔌 Go BFF 代理网关 REST   : http://localhost:8081" -ForegroundColor Green
    Write-Host " 🛡️ Privacy Agent REST    : https://localhost:8079 (TLS 加密)" -ForegroundColor Green
    Write-Host " ⚡ Privacy Agent gRPC    : localhost:50051 (mTLS 双向证书鉴权)" -ForegroundColor Green
    Write-Host " 🔒 受信任根 CA 证书路径   : $CertDir\ca.crt" -ForegroundColor Green
} else {
    Write-Host " ✨ PrivShield 控制台套件 [标准非 mTLS 版本] 已成功启动！" -ForegroundColor Green
    Write-Host " 🌐 React 控制台 Web UI   : http://localhost:5173" -ForegroundColor Green
    Write-Host " 🔌 Go BFF 代理网关 REST   : http://localhost:8081" -ForegroundColor Green
    Write-Host " 🛡️ Privacy Agent REST    : http://localhost:8079 (明文 HTTP)" -ForegroundColor Green
    Write-Host " ⚡ Privacy Agent gRPC    : localhost:50051 (明文 gRPC)" -ForegroundColor Green
}
Write-Host " 停止服务命令            : .\scripts\dev\docker-stop.ps1 或 .\scripts\dev\docker-stop.sh" -ForegroundColor Yellow
Write-Host "============================================================================" -ForegroundColor Cyan
