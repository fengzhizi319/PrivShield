#!/usr/bin/env bash
# Benchmark 评估（JSON 合法率 / 密级准确率 / 实体 F1 / 零泄漏率 / 延迟统计）
# Benchmark evaluation (JSON validity / level accuracy / entity F1 / leakage / latency).
#
# 用法 / Usage:
#   ./llmlora/scripts/evaluate.sh                                             # 默认评估合并模型（全部测试样本）
#   ./llmlora/scripts/evaluate.sh --max-samples 20                            # 限制评估条数
#   ./llmlora/scripts/evaluate.sh --model-path llmlora/basemodels/cmeee_merged \
#       --adapter-path llmlora/output/saves/qwen35-cmeee-privacy-lora         # 基座 + LoRA 模式
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

if [[ $# -eq 0 ]]; then
    set -- --model-path llmlora/output/models/Qwen3.5-0.8B-Privacy-Classifier-Smoother
fi

exec "$VENV_PY" -m llmlora.scripts.evaluate "$@"
