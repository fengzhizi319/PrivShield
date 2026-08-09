# -*- coding: utf-8 -*-
"""
llmlora 训练一键启动脚本 / One-command training launcher.

运行环境要求 / Environment requirement:
    llmlora/.venv (transformers>=5.2 + peft + accelerate)，构建方式见
    docs/design_and_workflow.md。
    Use llmlora/.venv (transformers>=5.2 + peft + accelerate); see
    docs/design_and_workflow.md for setup instructions.

用法示例 / Usage:
    python -m llmlora.scripts.train --epochs 3 --batch-size 4
    python -m llmlora.scripts.train --max-steps 10 --no-merge   # 冒烟测试
"""
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

# 支持从任意工作目录启动 / Allow launching from any cwd
_REPO_ROOT = Path(__file__).resolve().parents[2]
if str(_REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(_REPO_ROOT))

from llmlora.src.models.trainer import run_lora_training  # noqa: E402
from llmlora.src.utils.config import Config  # noqa: E402
from llmlora.src.utils.logger import setup_logger  # noqa: E402

logger = setup_logger("train")


def _warn_if_wrong_env() -> None:
    """检测当前解释器 transformers 版本，过低时给出明确提示。

    Check the interpreter's transformers version and fail fast with a clear
    message, because the Qwen3.5 base requires transformers>=5.2.
    """
    try:
        import transformers
    except ImportError:
        logger.error("未安装 transformers，请先激活 llmlora/.venv")
        sys.exit(1)
    major = int(transformers.__version__.split(".")[0])
    if major < 5:
        logger.error(
            f"当前 transformers=={transformers.__version__} 无法加载 Qwen3.5 基座，"
            "请使用 llmlora/.venv（transformers>=5.2）运行本脚本"
        )
        sys.exit(1)


def main() -> None:
    """训练入口 / Training entry point."""
    _warn_if_wrong_env()

    parser = argparse.ArgumentParser(description="运行 llmlora LoRA 微调训练流程")
    # 路径 / Paths
    parser.add_argument("--base-model-path", type=str, default=None, help="基座模型路径")
    parser.add_argument("--data-dir", type=str, default=None, help="数据集目录")
    parser.add_argument("--output-dir", type=str, default=None, help="LoRA 保存目录")
    parser.add_argument("--merged-output-dir", type=str, default=None, help="合并模型保存目录")
    parser.add_argument("--resume-from-checkpoint", type=str, default=None, help="断点续训 checkpoint 目录")
    # 训练超参 / Training hyper-params
    parser.add_argument("--epochs", type=int, default=3, help="训练 Epoch 数")
    parser.add_argument("--max-steps", type=int, default=-1, help="最大训练步数（-1=跑满 epoch）")
    parser.add_argument("--batch-size", type=int, default=4, help="每卡 Batch Size")
    parser.add_argument("--grad-accum-steps", type=int, default=4, help="梯度累积步数")
    parser.add_argument("--lr", type=float, default=2e-4, help="学习率")
    parser.add_argument("--max-length", type=int, default=512, help="单样本最大 token 长度")
    parser.add_argument("--seed", type=int, default=42, help="随机种子")
    # LoRA / LoRA
    parser.add_argument("--lora-r", type=int, default=16, help="LoRA 秩 r")
    parser.add_argument("--lora-alpha", type=int, default=32, help="LoRA alpha")
    parser.add_argument("--lora-dropout", type=float, default=0.05, help="LoRA dropout")
    # 硬件与导出 / Hardware & export
    parser.add_argument(
        "--dtype", type=str, default="auto", choices=["auto", "bf16", "fp16", "fp32"],
        help="强制计算精度",
    )
    parser.add_argument("--agent-model-dir", type=str, default=None, help="Agent .models 部署目标目录")
    parser.add_argument("--no-gradient-checkpointing", action="store_true", help="关闭梯度检查点")
    parser.add_argument("--no-merge", action="store_true", help="训练后不自动合并 LoRA 权重")
    parser.add_argument("--no-copy-to-agent", action="store_true", help="训练合并后不自动同步到 Agent .models 部署目录")
    args = parser.parse_args()

    cfg = Config()
    # 仅覆盖用户显式传入的字段 / Override only explicitly passed fields
    if args.base_model_path:
        cfg.base_model_path = args.base_model_path
    if args.data_dir:
        cfg.data_dir = args.data_dir
    if args.output_dir:
        cfg.output_dir = args.output_dir
    if args.merged_output_dir:
        cfg.merged_output_dir = args.merged_output_dir
    if args.agent_model_dir:
        cfg.agent_model_dir = args.agent_model_dir
    if args.no_copy_to_agent:
        cfg.auto_copy_to_agent_dir = False
    cfg.resume_from_checkpoint = args.resume_from_checkpoint
    cfg.num_epochs = args.epochs
    cfg.max_steps = args.max_steps
    cfg.batch_size = args.batch_size
    cfg.grad_accum_steps = args.grad_accum_steps
    cfg.learning_rate = args.lr
    cfg.max_length = args.max_length
    cfg.seed = args.seed
    cfg.lora_r = args.lora_r
    cfg.lora_alpha = args.lora_alpha
    cfg.lora_dropout = args.lora_dropout
    cfg.dtype = args.dtype
    if args.no_gradient_checkpointing:
        cfg.gradient_checkpointing = False
    cfg.merge_on_completion = not args.no_merge

    logger.info(
        f"训练配置 | base={os.path.basename(cfg.base_model_path)} "
        f"| epochs={cfg.num_epochs} | bs={cfg.batch_size}x{cfg.grad_accum_steps} "
        f"| lr={cfg.learning_rate} | lora r={cfg.lora_r}/alpha={cfg.lora_alpha} "
        f"| dtype={cfg.dtype}"
    )
    run_lora_training(cfg)


if __name__ == "__main__":
    main()
