#!/usr/bin/env bash
# ==============================================================================
# PrivShield - 一键启动 Prometheus + Grafana 监控大屏
# Start Prometheus + Grafana monitoring stack via Docker Compose
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

echo "========================================================"
echo " 📈 正在启动 PrivShield 监控栈 (Prometheus + Grafana)..."
echo "========================================================"

if ! command -v docker &> /dev/null; then
  echo "❌ 错误: 未检测到 Docker，请先安装 Docker。"
  exit 1
fi

docker compose -f "$COMPOSE_FILE" --profile monitoring up -d prometheus grafana

echo ""
echo "✅ 监控栈已在后台启动！"
echo "--------------------------------------------------------"
echo "  • Prometheus Web UI : http://localhost:9090"
echo "  • Grafana 监控大屏  : http://localhost:3000"
echo "    - 默认管理员账号 : admin"
echo "    - 默认管理员密码 : admin123 (或见 .env 配置)"
echo "    - 预置大屏 1     : PrivShield Overview (全景总览)"
echo "    - 预置大屏 2     : PrivShield Service Hub (调度中枢专属大屏)"
echo "--------------------------------------------------------"
echo "💡 提示: 运行 bash scripts/dev/check_metrics_endpoints.sh 检查各微服务指标抓取状态"
echo "========================================================"
