#!/usr/bin/env python3
"""将本地 PyTorch / Hugging Face 模型转换为 Apple MLX 格式，用于 macOS Metal 推理。

Convert local PyTorch/Hugging Face models to Apple MLX format for macOS Metal inference.

依赖 / Dependencies (macOS only):
    pip install mlx torch transformers safetensors

用法 / Usage:
    python scripts/models/convert_models_to_mlx.py \
        --model .models/Qwen2-VL-2B-Instruct \
        --output .models/Qwen2-VL-2B-Instruct-mlx

    python scripts/models/convert_models_to_mlx.py \
        --model .models/raner_cmeee \
        --output .models/raner_cmeee-mlx
"""

from __future__ import annotations

import argparse
import json
import platform
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any


# 需要复制到输出目录的常用辅助文件 / Auxiliary files to copy alongside weights
_TOKENIZER_FILES = (
    "config.json",
    "tokenizer.json",
    "tokenizer_config.json",
    "vocab.txt",
    "chat_template.json",
    "preprocessor_config.json",
    "generation_config.json",
    "merges.txt",
    "special_tokens_map.json",
)


def _require_macos() -> None:
    """MLX 只在 Apple Silicon 上可用 / MLX is only available on Apple Silicon."""
    if platform.system() != "Darwin" or platform.machine() not in ("arm64", "x86_64"):
        print(
            "[ERROR] 本脚本仅支持 macOS（Apple Silicon 优先）。"
            "MLX conversion only runs on macOS.",
            file=sys.stderr,
        )
        sys.exit(1)


def _load_torch_weights(model_path: Path) -> dict[str, Any]:
    """加载 PyTorch 权重字典，支持 safetensors 与 pytorch_model.bin。"""
    import torch

    safetensors_files = sorted(model_path.glob("model*.safetensors"))
    if safetensors_files:
        from safetensors.torch import load_file

        state: dict[str, Any] = {}
        for file in safetensors_files:
            state.update(load_file(str(file)))
        return state

    pt_file = model_path / "pytorch_model.bin"
    if pt_file.exists():
        return torch.load(pt_file, map_location="cpu", weights_only=True)

    raise FileNotFoundError(
        f"未在 {model_path} 找到 safetensors 或 pytorch_model.bin 权重文件"
    )


def _convert_state_dict_to_mlx(
    state_dict: dict[str, Any],
    dtype: str | None,
) -> dict[str, Any]:
    """将 PyTorch 权重字典转换为 MLX 数组字典。"""
    import mlx.core as mx
    import torch

    dtype_map = {
        "float32": None,
        "float16": mx.float16,
        "bfloat16": mx.bfloat16,
    }
    target_dtype = dtype_map.get(dtype) if dtype else None

    mlx_weights: dict[str, Any] = {}
    for key, tensor in state_dict.items():
        import numpy as np

        # BFloat16 张量需要先转为 float32 再转 numpy（numpy 不支持 bf16）
        t = tensor.detach().cpu()
        if t.dtype == torch.bfloat16:
            t = t.float()
        arr = t.numpy().astype(np.float32)
        mlx_weights[key] = mx.array(arr, dtype=target_dtype)
    return mlx_weights


def _generic_convert(
    model_path: Path,
    output_path: Path,
    dtype: str | None,
) -> None:
    """通用转换：直接将 PyTorch 权重保存为 MLX safetensors。"""
    import mlx.core as mx

    output_path.mkdir(parents=True, exist_ok=True)
    print(f"[*] 加载 PyTorch 权重: {model_path}")
    state_dict = _load_torch_weights(model_path)
    print(f"[*] 共 {len(state_dict)} 个张量")

    print(f"[*] 转换为 MLX 数组 (dtype={dtype})")
    mlx_weights = _convert_state_dict_to_mlx(state_dict, dtype)

    weights_file = output_path / "weights.safetensors"
    print(f"[*] 保存 MLX weights -> {weights_file}")
    mx.save_safetensors(str(weights_file), mlx_weights)


def _copy_auxiliary_files(model_path: Path, output_path: Path) -> None:
    """复制 config / tokenizer 等辅助文件。"""
    output_path.mkdir(parents=True, exist_ok=True)
    for name in _TOKENIZER_FILES:
        src = model_path / name
        if src.exists():
            dst = output_path / name
            print(f"[*] 复制 {name} -> {dst}")
            shutil.copy2(src, dst)


def _try_mlx_lm_convert(
    model_path: Path,
    output_path: Path,
    dtype: str,
    quantize: bool,
) -> bool:
    """尝试使用 mlx_lm.convert 进行转换（仅支持 decoder LLM）。"""
    try:
        subprocess.run(
            [
                sys.executable,
                "-m",
                "mlx_lm.convert",
                "--hf-path",
                str(model_path),
                "--mlx-path",
                str(output_path),
                "--dtype",
                dtype,
            ]
            + (["--quantize", "--q-bits", "4"] if quantize else []),
            check=True,
        )
        return True
    except (subprocess.CalledProcessError, FileNotFoundError) as e:
        print(f"[!] mlx_lm.convert 不可用或失败: {e}")
        return False


def convert_model(
    model_path: Path,
    output_path: Path,
    dtype: str = "bfloat16",
    quantize: bool = False,
) -> None:
    """转换单个模型目录。"""
    _require_macos()

    if not model_path.exists() or not model_path.is_dir():
        raise FileNotFoundError(f"模型目录不存在: {model_path}")

    config_path = model_path / "config.json"
    if not config_path.exists():
        raise FileNotFoundError(f"未找到 config.json: {config_path}")

    config = json.loads(config_path.read_text(encoding="utf-8"))
    model_type = config.get("model_type", "")
    print(f"[*] 模型类型: {model_type}")

    # 对常见 decoder LLM 先尝试 mlx_lm.convert（可生成量化模型）
    if model_type in {"qwen2", "llama", "mistral", "gemma", "qwen2_moe"}:
        print("[*] 尝试使用 mlx_lm.convert 转换...")
        if _try_mlx_lm_convert(model_path, output_path, dtype, quantize):
            _copy_auxiliary_files(model_path, output_path)
            return
        print("[!] 回退到通用转换")

    # 通用转换（适用于 Qwen2-VL、BERT NER 等 encoder / VLM）
    _generic_convert(model_path, output_path, dtype=dtype)
    _copy_auxiliary_files(model_path, output_path)

    print(f"[+] 转换完成: {output_path}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="将本地 PyTorch 模型转换为 Apple MLX 格式 (macOS Metal)"
    )
    parser.add_argument(
        "--model",
        required=True,
        type=Path,
        help="源模型目录，例如 .models/Qwen2-VL-2B-Instruct",
    )
    parser.add_argument(
        "--output",
        required=True,
        type=Path,
        help="输出 MLX 模型目录，例如 .models/Qwen2-VL-2B-Instruct-mlx",
    )
    parser.add_argument(
        "--dtype",
        default="bfloat16",
        choices=["float16", "bfloat16", "float32"],
        help="目标权重精度",
    )
    parser.add_argument(
        "--quantize",
        action="store_true",
        help="尝试使用 mlx_lm.convert 进行 4-bit 量化（仅部分 LLM 支持）",
    )
    args = parser.parse_args()
    convert_model(args.model, args.output, dtype=args.dtype, quantize=args.quantize)


if __name__ == "__main__":
    main()
