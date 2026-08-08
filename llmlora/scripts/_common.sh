#!/usr/bin/env bash
# 公共前置：定位仓库根目录、校验独立虚拟环境、切换到仓库根。
# Shared preamble: locate repo root, verify the isolated venv, cd to repo root.
# 用法（被其他脚本 source）/ Usage (sourced by other scripts):
#   source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LLMLORA_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$LLMLORA_DIR")"
VENV_PY="$LLMLORA_DIR/.venv/bin/python"

if [[ ! -x "$VENV_PY" ]]; then
    echo "[错误] 未找到独立训练环境: $VENV_PY" >&2
    echo "       请先运行: ./llmlora/scripts/setup_env.sh" >&2
    exit 1
fi

# 所有 python -m llmlora.* 命令必须从仓库根目录执行
# All python -m llmlora.* commands must run from the repository root
cd "$REPO_ROOT"
