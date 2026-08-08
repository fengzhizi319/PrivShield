# -*- coding: utf-8 -*-
"""
PyTorch 原生推理性能测试 (Fast Sub-20s Mode).
"""
from __future__ import annotations

import argparse
import gc
import json
import time
import sys
from pathlib import Path
from typing import Dict, List, Any

_REPO_ROOT = Path(__file__).resolve().parents[2]
if str(_REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(_REPO_ROOT))

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer
from llmlora.src.dataset.loader import render_prompt_text, load_jsonl


def run_pytorch_benchmark(
    model_path: str,
    test_data_path: str,
    batch_sizes: List[int] = [1, 4],
    max_new_tokens: int = 64,
) -> Dict[str, Any]:
    """运行 PyTorch 快速基准测试"""
    print("=" * 64)
    print("🔥 启动 PyTorch 原生推理性能 Benchmark (快速模式)")
    print(f"  模型路径: {model_path}")
    print("=" * 64)

    samples = load_jsonl(test_data_path)
    if not samples:
        print("❌ 测试集数据为空")
        return {}

    device = "cuda" if torch.cuda.is_available() else "cpu"
    start_load = time.perf_counter()
    tokenizer = AutoTokenizer.from_pretrained(model_path, trust_remote_code=True, padding_side="left")
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    model = AutoModelForCausalLM.from_pretrained(
        model_path,
        trust_remote_code=True,
        torch_dtype=torch.bfloat16 if torch.cuda.is_available() else torch.float32,
        device_map="auto" if torch.cuda.is_available() else None,
    )
    model.eval()
    load_time = time.perf_counter() - start_load
    print(f"✅ PyTorch 模型加载完成，耗时: {load_time:.2f}s\n")

    results = {}

    print("-" * 64)
    print(f"{'Batch Size':<12} | {'总耗时(ms)':<12} | {'单条延迟(ms)':<14} | {'吞吐(tokens/s)':<14}")
    print("-" * 64)

    for bsize in batch_sizes:
        batch_items = (samples * ((bsize // len(samples)) + 2))[:bsize]
        prompts = [render_prompt_text(tokenizer, s["input"]) for s in batch_items]

        inputs = tokenizer(prompts, return_tensors="pt", padding=True).to(device)

        start_gen = time.perf_counter()
        with torch.no_grad():
            outputs = model.generate(
                **inputs,
                max_new_tokens=max_new_tokens,
                do_sample=False,
                pad_token_id=tokenizer.pad_token_id,
            )
        elapsed_sec = time.perf_counter() - start_gen
        elapsed_ms = elapsed_sec * 1000.0

        gen_tokens = outputs.shape[1] - inputs["input_ids"].shape[1]
        total_tokens = gen_tokens * bsize
        tokens_per_sec = total_tokens / elapsed_sec if elapsed_sec > 0 else 0.0
        avg_latency = elapsed_ms / bsize

        results[f"batch_{bsize}"] = {
            "batch_size": bsize,
            "elapsed_ms": elapsed_ms,
            "avg_latency_ms": avg_latency,
            "tokens_per_sec": tokens_per_sec,
        }

        print(f"{bsize:<12} | {elapsed_ms:<12.1f} | {avg_latency:<14.1f} | {tokens_per_sec:<14.2f}")

    print("-" * 64)

    # 释放 GPU 显存
    del model
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    return results


def main():
    parser = argparse.ArgumentParser(description="PyTorch 快速推理性能测试")
    parser.add_argument(
        "--model-path",
        type=str,
        default=str(_REPO_ROOT / "llmlora" / "output" / "models" / "Qwen3.5-0.8B-Privacy-Classifier-Smoother"),
    )
    parser.add_argument(
        "--test-data",
        type=str,
        default=str(_REPO_ROOT / "llmlora" / "data" / "test.jsonl"),
    )
    args = parser.parse_args()

    run_pytorch_benchmark(args.model_path, args.test_data)


if __name__ == "__main__":
    main()
