#!/usr/bin/env bash
# ============================================================================
# 【开发模式】一键停止控制台全部开发服务
# Stop all dev mode console services (Agent, Backends, Vite Dev Server)
#
# ⚠️ 注意 / WARNING:
#   本脚本按 .pids/ 中的 PID 文件精确停止，并对开发常用端口
#   (5173/8081/8082/8083/8084/8079/50051) 上残留的任何进程执行清理。
#   清理策略为先 SIGTERM 优雅退出、1 秒后仍存活再 SIGKILL 强杀。
# ============================================================================

set -euo pipefail

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -h, --help    显示帮助信息并退出"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONSOLE_DIR="$PROJECT_ROOT/console"
PIDS_DIR="$PROJECT_ROOT/.pids"
LEGACY_PIDS_DIR="$CONSOLE_DIR/.pids"

# ── kill_by_pid_file: 通过 PID 文件精确停止指定服务 ────────────────────
# 策略：SIGTERM → 0.5s → 仍存活则 SIGKILL
kill_by_pid_file() {
    local file="$1"
    local name="$2"
    if [[ -f "$file" ]]; then
        local pid
        pid=$(cat "$file")
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            echo "停止 $name (PID $pid)..."
            kill "$pid" 2>/dev/null || true
            sleep 0.5
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$file"
    fi
}

# ── kill_by_port: 清理指定端口上的残余进程与 Docker 容器 ──────────────
# 作为 PID 文件的补充安全网，确保端口完全释放
# 支持三种工具：lsof（macOS/Linux）、ss（Linux）、fuser（备选），以及 Docker 容器检测
kill_by_port() {
    local port="$1"
    local name="$2"

    # 1. 检查并停止占用该端口的 Docker 容器
    if command -v docker >/dev/null 2>&1; then
        local cids=""
        cids=$(docker ps -q --filter "publish=$port" 2>/dev/null || true)
        if [[ -n "$cids" ]]; then
            echo "停止占用端口 $port 的 Docker 容器 ($name)..."
            for cid in $cids; do
                docker stop "$cid" >/dev/null 2>&1 || docker kill "$cid" >/dev/null 2>&1 || true
            done
        fi
    fi

    # 2. 清理宿主机残余进程
    local pids=""
    if command -v lsof >/dev/null 2>&1; then
        pids=$( (lsof -t -i :"$port" 2>/dev/null || true) | sort -u | tr '\n' ' ')
    elif command -v ss >/dev/null 2>&1; then
        pids=$( (ss -tlnp 2>/dev/null || true) | (grep -E "LISTEN.*:$port\\s" || true) | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | sort -u | tr '\n' ' ')
    elif command -v fuser >/dev/null 2>&1; then
        pids=$(fuser "$port"/tcp 2>/dev/null | tr -s ' ' || true)
    fi

    if [[ -n "$pids" ]]; then
        echo "清理端口 $port 上的残余进程 ($name: $pids)..."
        for pid in $pids; do
            kill -15 "$pid" 2>/dev/null || true
        done
        sleep 0.5
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null || true
            fi
        done
    fi
}

echo "正在停止【开发模式】控制台所有服务..."

# ── 第一步：通过 PID 文件停止已知服务（按依赖反序） ──────────────────
# 遍历新版 .pids/ 和旧版 console/.pids/ 两个目录
for dir in "$PIDS_DIR" "$LEGACY_PIDS_DIR"; do
    if [[ -d "$dir" ]]; then
        kill_by_pid_file "$dir/vite-dev.pid" "Vite 开发服务器"
        kill_by_pid_file "$dir/console-go-mtls.pid" "Go gRPC 代理后端 (mTLS)"
        kill_by_pid_file "$dir/console-go-all.pid" "Go gRPC 代理后端 (all)"
        kill_by_pid_file "$dir/console-go.pid" "Go BFF 代理后端"
        kill_by_pid_file "$dir/service-hub.pid" "service-hub 调度中枢"
        kill_by_pid_file "$dir/datasource-mgr.pid" "datasource-mgr 数据源"
        kill_by_pid_file "$dir/audit-log.pid" "audit-log 审计日志"
        kill_by_pid_file "$dir/privshield-gateway.pid" "PrivShield Gateway (Go)"
        kill_by_pid_file "$dir/privshield-agent.pid" "PrivShield Agent (Go)"
        kill_by_pid_file "$dir/agent-go-mtls.pid" "PrivShield (mTLS)"
        kill_by_pid_file "$dir/agent-all.pid" "PrivShield (all)"
        kill_by_pid_file "$dir/agent-go.pid" "PrivShield (gRPC)"
        kill_by_pid_file "$dir/agent.pid" "PrivShield (REST)"
    fi
done

# ── 第二步：端口级安全网清理 ────────────────────────────────────────
# 即使 PID 文件缺失或过期，也确保所有开发端口完全释放
kill_by_port 5173 "Vite 前端开发服务器"
kill_by_port 8085 "App-LZ Go BFF"
kill_by_port 8084 "audit-log 审计日志"
kill_by_port 8083 "datasource-mgr 数据源管理"
kill_by_port 8082 "service-hub 调度中枢"
kill_by_port 8081 "Go BFF 代理后端"
kill_by_port 50000 "PrivShield Gateway gRPC"
kill_by_port 8000 "PrivShield Gateway REST"
kill_by_port 50051 "PrivShield gRPC"
kill_by_port 8079 "PrivShield REST"

echo "✅ 开发模式所有服务已安全停止。"
