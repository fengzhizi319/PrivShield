#!/usr/bin/env bash
# ============================================================================
# 【Docker 模式】一键停止、清理控制台及全栈相关容器
# Stop & Cleanup Console & Full Stack Docker Containers
#
# 用法 / Usage: ./scripts/dev/docker-stop.sh [--volumes]
# ============================================================================

set -euo pipefail

# ── 解析脚本所在目录，定位项目根目录 ────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
VOLUMES_FLAG=""

# ── 解析命令行参数：仅支持 --volumes / -v / --help ────────────────────────
for arg in "$@"; do
    case "$arg" in
        --volumes|-v)
            # 传入 --volumes 时，docker compose down 会同时删除关联的命名卷
            # 注意：卷内持久化数据（如隐私预算 DB）将被永久清空
            VOLUMES_FLAG="--volumes"
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [--volumes]"
            echo ""
            echo "选项 / Options:"
            echo "  --volumes, -v   同时删除关联的命名卷 (持久化数据将被清空)"
            echo "  -h, --help      显示帮助信息"
            exit 0
            ;;
    esac
done

echo "============================================================================"
echo "🛑 [Docker Mode] 正在停止并清理 PrivShield 容器服务..."
echo "============================================================================"

# ── 进入 docker-compose 目录，执行 down 命令 ──────────────────────────────
# 同时激活 llm 和 monitoring profile，确保所有可选容器也被停止
cd "$PROJECT_ROOT/deploy/docker-compose"

# shellcheck disable=SC2086
docker compose --profile llm --profile monitoring down $VOLUMES_FLAG

echo ""
echo "✅ 所有相关容器与网络已成功停止与清理！"
echo "============================================================================"
