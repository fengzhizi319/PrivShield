#!/usr/bin/env bash
# ============================================================================
# Service Hub (数据服务调度中枢) Development Startup Script
# 数据服务调度中枢本地开发模式快速启动脚本
#
# 功能说明：
#   1. 定位脚本所在目录作为工作根目录；
#   2. 从环境变量读取绑定主机与端口（默认 127.0.0.1:8082）；
#   3. 自动创建 bin 编译输出目录并使用 go build 编译 cmd/server；
#   4. 以 exec 方式替换当前 Shell 进程拉起 service-hub 二进制服务。
#
# 环境变量配置：
#   SERVICE_HUB_HOST: HTTP 监听地址（默认 127.0.0.1）
#   SERVICE_HUB_PORT: HTTP 监听端口（默认 8082）
#
# 启动方式：
#   bash run.sh
# ============================================================================

set -euo pipefail

# 1. 切换到脚本所在目录（即 services/service-hub）
cd "$(dirname "$0")"

# 2. 读取网络监听配置，若未设置则使用默认开发参数
HOST="${SERVICE_HUB_HOST:-127.0.0.1}"
PORT="${SERVICE_HUB_PORT:-8082}"

export SERVICE_HUB_HOST="$HOST"
export SERVICE_HUB_PORT="$PORT"

# 3. 创建二进制输出目录并编译服务端程序
mkdir -p bin
echo ">> 正在编译 service-hub 调度中枢..."
go build -o bin/service-hub ./cmd/server

# 4. 执行二进制程序并移交进程信号控制权
echo ">> 启动 service-hub 在 http://${SERVICE_HUB_HOST}:${SERVICE_HUB_PORT} ..."
exec ./bin/service-hub

