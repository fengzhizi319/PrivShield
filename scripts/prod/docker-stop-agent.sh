#!/usr/bin/env bash
# ============================================================================
# 【生产模式】停止并清理 PrivShield 生产级 Agent 容器
# Stop and remove Standalone Production PrivShield Agent container
#
# 执行步骤总览：
#   1. 初始化容器名称（PrivShield-prod-agent）
#   2. 查询 Docker 守护进程中是否存在该容器
#   3. 发送 SIGTERM 优雅停止容器并执行 rm 清理
#
# 用法 / Usage: ./scripts/prod/docker-stop-agent.sh
# ============================================================================

set -euo pipefail

# ── 步骤 1：指定容器名称与参数解析 ──────────────────────────────────────────
CONTAINER_NAME="PrivShield-prod-agent"

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

echo "============================================================================"
echo "🛑 【生产模式】正在安全停止 PrivShield 生产级 Agent 容器 ($CONTAINER_NAME)..."
echo "============================================================================"

# ── 步骤 2：检查并停止清理容器 ────────────────────────────────────────────
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
    echo "✅ 容器 $CONTAINER_NAME 已成功停止并清理。"
else
    echo "ℹ️  容器 $CONTAINER_NAME 未在运行。"
fi

echo "============================================================================"
