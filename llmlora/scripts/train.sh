#!/usr/bin/env bash
# LoRA 训练一键启动（默认训练完成后自动合并导出）
# One-command LoRA training (auto merge & export by default).
#
# 用法 / Usage:
#   ./llmlora/scripts/train.sh                                # 默认 3 epoch, bs=4
#   ./llmlora/scripts/train.sh --epochs 5 --lr 1e-4           # 自定义参数透传
#   ./llmlora/scripts/train.sh --max-steps 10 --no-merge      # 冒烟快跑
#   ./llmlora/scripts/train.sh --resume-from-checkpoint <dir> # 断点续训
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

exec "$VENV_PY" -m llmlora.scripts.train "$@"
