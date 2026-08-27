#!/usr/bin/env bash
# scripts/dev/start-postgres.sh — 开发/测试环境 PostgreSQL 快速启动。
# 使用 Docker 在本地启动 PostgreSQL 实例，供 Phase B LeasedTaskStore 开发调试。
#
# 用法：
#   bash scripts/dev/start-postgres.sh          # 启动 PostgreSQL
#   bash scripts/dev/start-postgres.sh --stop    # 停止并移除容器
#
# 环境变量：
#   PG_CONTAINER_NAME  容器名称（默认 privshield-pg-dev）
#   PG_PORT            宿主机映射端口（默认 5432）
#   PG_PASSWORD        数据库密码（默认 privshield_dev）
set -euo pipefail

CONTAINER_NAME="${PG_CONTAINER_NAME:-privshield-pg-dev}"
PG_PORT="${PG_PORT:-5432}"
PG_PASSWORD="${PG_PASSWORD:-privshield_dev}"
PG_DB="privshield_hub"
PG_USER="privshield"

stop_postgres() {
    echo "[INFO] Stopping PostgreSQL container '${CONTAINER_NAME}'..."
    docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true
    echo "[INFO] PostgreSQL container stopped."
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "用法 / Usage: $0 [选项]"
    echo ""
    echo "选项 / Options:"
    echo "  --stop      停止并移除 PostgreSQL 容器"
    echo "  -h, --help  显示帮助信息并退出"
    exit 0
fi

if [[ "${1:-}" == "--stop" ]]; then
    stop_postgres
    exit 0
fi

# Check if already running / 检查是否已在运行
if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "[INFO] PostgreSQL container '${CONTAINER_NAME}' is already running."
    echo "[INFO] Connection: postgres://${PG_USER}:${PG_PASSWORD}@localhost:${PG_PORT}/${PG_DB}"
    exit 0
fi

# Remove stale container if exists / 移除残留容器
docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true

echo "[INFO] Starting PostgreSQL ${PG_PORT} -> ${CONTAINER_NAME}..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    -e POSTGRES_USER="${PG_USER}" \
    -e POSTGRES_PASSWORD="${PG_PASSWORD}" \
    -e POSTGRES_DB="${PG_DB}" \
    -p "${PG_PORT}:5432" \
    postgres:16-alpine

echo "[INFO] Waiting for PostgreSQL to be ready..."
for i in $(seq 1 30); do
    if docker exec "${CONTAINER_NAME}" pg_isready -U "${PG_USER}" -d "${PG_DB}" >/dev/null 2>&1; then
        echo "[INFO] PostgreSQL is ready!"
        echo ""
        echo "  Connection string (for SERVICE_HUB_PG_DSN):"
        echo "    postgres://${PG_USER}:${PG_PASSWORD}@localhost:${PG_PORT}/${PG_DB}?sslmode=disable"
        echo ""
        echo "  To stop: bash scripts/dev/start-postgres.sh --stop"
        exit 0
    fi
    sleep 1
done

echo "[ERROR] PostgreSQL did not become ready within 30 seconds."
exit 1
