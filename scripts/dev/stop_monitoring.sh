#!/usr/bin/env bash
# ==============================================================================
# PrivShield - 一键停止 Prometheus + Grafana 监控大屏
# Stop Prometheus + Grafana monitoring stack
# ==============================================================================
set -e

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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/deploy/docker-compose/docker-compose.prod.yml"

echo "🛑 正在停止监控栈 (Prometheus + Grafana)..."

docker compose -f "$COMPOSE_FILE" --profile monitoring stop prometheus grafana || true
docker compose -f "$COMPOSE_FILE" --profile monitoring rm -f prometheus grafana || true

echo "✅ 监控栈已停止。"
