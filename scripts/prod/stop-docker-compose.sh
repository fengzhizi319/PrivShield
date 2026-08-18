#!/usr/bin/env bash
# ============================================================================
# 【生产模式】PrivShield 生产级 Docker Compose 优雅停服脚本
# Gracefully stop PrivShield Production Docker Compose Stack
#
# 执行步骤总览：
#   1. 解析命令行参数（指定 compose 文件、是否清理卷、是否清理孤儿容器）
#   2. 切换到 compose 编排目录（docker compose 按 CWD 查找配置文件）
#   3. 检查 compose 文件是否存在，不存在则回退到默认 docker-compose.yml
#   4. 执行 docker compose down，停止所有 Profile 的服务（含 llm + monitoring）
#   5. 输出停服完成提示
#
# 用法 / Usage:
#   ./scripts/prod/stop-docker-compose.sh [选项]
#
# 选项 / Options:
#   -f, --file FILE      指定 Compose 配置文件 (默认: docker-compose.prod.yml)
#   --volumes, -v        同时清理挂载的匿名数据卷 (注意：慎用，避免持久化预算丢失)
#   --remove-orphans     清理孤儿容器
#   -h, --help           显示帮助信息
# ============================================================================

# set -e: 任何命令返回非零状态码立即退出（防止错误级联）
# set -u: 引用未定义变量时报错（防止拼写错误导致静默失败）
# set -o pipefail: 管道中任一命令失败则整体返回非零（防止 | 后掩盖错误）
set -euo pipefail

# ── 步骤 0：定位路径 ──────────────────────────────────────────────────────
# 通过 $0（脚本自身路径）反推项目根目录，确保无论从哪里调用都能正确定位 compose 文件
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"          # 脚本所在目录：scripts/prod/
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"       # 上溯两级：项目根目录
COMPOSE_DIR="$PROJECT_ROOT/deploy/docker-compose"      # Docker Compose 编排文件目录

# ── 步骤 1：设置参数默认值 ────────────────────────────────────────────────
# 默认使用生产 compose 文件；EXTRA_FLAGS 收集可选的 down 参数（如 --volumes）
COMPOSE_FILE="docker-compose.prod.yml"
EXTRA_FLAGS=()

# ── 步骤 2：解析命令行参数 ────────────────────────────────────────────────
# 遍历所有位置参数，按 --key value 配对消费（shift 跳过已处理的参数）
while [[ $# -gt 0 ]]; do
    case "$1" in
        -f|--file)
            # 指定自定义 compose 文件（如 docker-compose.dev.yml / docker-compose.test.yml）
            COMPOSE_FILE="$2"
            shift 2
            ;;
        --volumes|-v)
            # 追加 --volumes 标志：down 时同时删除匿名数据卷
            # 警告：会丢失预算数据库（budget-db）和审计日志（audit-logs）等持久化数据！
            EXTRA_FLAGS+=("--volumes")
            shift 1
            ;;
        --remove-orphans)
            # 追加 --remove-orphans 标志：删除 compose 文件中未声明但仍在运行的容器
            # 场景：旧版 compose 文件启动了额外服务，更新文件后需要清理残留
            EXTRA_FLAGS+=("--remove-orphans")
            shift 1
            ;;
        -h|--help)
            echo "用法 / Usage: $0 [选项]"
            echo ""
            echo "选项 / Options:"
            echo "  -f, --file FILE      指定 Compose 配置文件 (默认: docker-compose.prod.yml)"
            echo "  -v, --volumes        同时移除持久化数据卷 (默认保留数据卷)"
            echo "  --remove-orphans     移除未在 compose 文件中定义的孤儿容器"
            echo "  -h, --help           显示帮助信息并退出"
            exit 0
            ;;
        *)
            echo "❌ [错误] 未知参数: $1" >&2
            echo "   请运行 $0 --help 查看帮助" >&2
            exit 1
            ;;
    esac
done

# ── 步骤 3：打印停服摘要 ──────────────────────────────────────────────────
echo "============================================================================"
echo "🛑 【生产模式】正在优雅停止 PrivShield 生产级容器集群..."
echo "============================================================================"

# ── 步骤 4：切换到编排目录 ────────────────────────────────────────────────
# docker compose 默认在当前工作目录查找 compose 文件；
# 必须 cd 到编排目录，否则 -f 相对路径会解析错误
cd "$COMPOSE_DIR"

# ── 步骤 5：检查 compose 文件存在性（带回退逻辑）────────────────────────
# 若指定的文件不存在，回退到通用默认文件 docker-compose.yml
# 场景：用户指定 docker-compose.prod.yml 但该文件被删除/重命名时，不至于报错退出
if [[ ! -f "$COMPOSE_FILE" ]]; then
    COMPOSE_FILE="docker-compose.yml"
fi

echo "   • 编排配置文件: $COMPOSE_FILE"

# ── 步骤 6：执行 docker compose down ─────────────────────────────────────
# docker compose down 的完整动作：
#   1. 停止所有运行中的容器（发送 SIGTERM，等待优雅退出，超时后 SIGKILL）
#   2. 删除容器、网络（compose 自动创建的自定义网络）
#   3. 若带 --volumes：同时删除匿名数据卷（预算 DB、审计日志等持久化数据会丢失！）
#   4. 若带 --remove-orphans：删除 compose 文件中未声明的残留容器
#
# --profile llm --profile monitoring 的作用：
#   compose 文件中 vllm 服务标记了 profiles: ["llm"]，prometheus/grafana 标记了 profiles: ["monitoring"]
#   默认 docker compose down 仅停止无 profile 的服务；显式指定 profile 确保全部服务都被停止
#   否则 llm/monitoring 容器会成为孤儿继续运行占用资源
#
# "${EXTRA_FLAGS[@]}" 展开为数组元素：
#   空数组时不传任何额外参数；非空时展开为 --volumes 和/或 --remove-orphans
docker compose -f "$COMPOSE_FILE" --profile llm --profile monitoring down "${EXTRA_FLAGS[@]}"

# ── 步骤 7：输出停服完成提示 ──────────────────────────────────────────────
echo ""
echo "✅ PrivShield 生产容器集群已安全停止。"
echo "============================================================================"
