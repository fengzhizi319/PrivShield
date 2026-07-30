#!/usr/bin/env bash
# ============================================================================
# Python Console Backend Development Startup Script
# Python 控制台后端开发模式启动脚本
#
# Starts the FastAPI-based console backend with hot-reload enabled (uvicorn --reload).
# 启动基于 FastAPI 的控制台后端，启用热重载（uvicorn --reload）。
# Default listen address: 127.0.0.1:8080, configurable via env vars.
# 默认监听地址：127.0.0.1:8080，可通过环境变量配置。
#
# Environment variables / 环境变量：
#   PRIVACY_CONSOLE_HOST - Listen host / 监听地址 (default: 127.0.0.1)
#   PRIVACY_CONSOLE_PORT - Listen port / 监听端口 (default: 8080)
# ============================================================================

# Enable strict mode: exit on error, undefined var, pipe failure
# 启用严格模式：遇错退出、未定义变量报错、管道失败报错
set -euo pipefail

# Change to script directory (console/backend/)
# 切换到脚本所在目录（console/backend/）
cd "$(dirname "$0")"

# Read listen host from env, default to 127.0.0.1
# 从环境变量读取监听地址，默认 127.0.0.1
HOST="${PRIVACY_CONSOLE_HOST:-127.0.0.1}"
# Read listen port from env, default to 8080
# 从环境变量读取监听端口，默认 8080
PORT="${PRIVACY_CONSOLE_PORT:-8080}"

# Replace current process with uvicorn (exec saves one process layer)
# 用 uvicorn 替换当前进程（exec 节省一层进程）
# --reload enables auto-restart on code changes (dev only)
# --reload 启用代码变更自动重启（仅开发用）
exec uvicorn app.main:app --host "$HOST" --port "$PORT" --reload
