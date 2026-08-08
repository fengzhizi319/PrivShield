# -*- coding: utf-8 -*-
"""
vLLM 高性能推理性能测试 (Fast Sub-20s Mode).
"""
from __future__ import annotations

import os
os.environ["VLLM_USE_V1"] = "0"
os.environ["VLLM_ENABLE_V1_MULTIPROCESSING"] = "0"

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

from vllm import LLM, SamplingParams
from transformers import AutoTokenizer
from llmlora.src.dataset.loader import render_prompt_text, load_jsonl


def run_vllm_benchmark(
    model_path: str,
    test_data_path: str,
    batch_sizes: List[int] = [1, 4, 16],
    gpu_utilization: float = 0.5,
) -> Dict[str, Any]:
    """运行 vLLM 快速基准测试"""
    print("=" * 64)
    print("🚀 启动 vLLM 高性能推理性能 Benchmark (快速模式)")
    print(f"  模型路径: {model_path}")
    print("=" * 64)

    samples = load_jsonl(test_data_path)
    if not samples:
        print("❌ 测试集数据为空")
        return {}

    tokenizer = AutoTokenizer.from_pretrained(model_path, trust_remote_code=True)
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    # 临时将 model_type 设为 qwen2，绕过 vLLM 多模态 Qwen3_5Config 的 preprocessor 校验
    cfg_file = Path(model_path) / "config.json"
    cfg_backup = cfg_file.read_text(encoding="utf-8")
    try:
        cfg_data = json.loads(cfg_backup)
        cfg_data["model_type"] = "qwen2"
        cfg_data["architectures"] = ["Qwen2ForCausalLM"]
        cfg_data.pop("sliding_window", None)
        cfg_data.pop("use_sliding_window", None)
        cfg_data.pop("rope_parameters", None)
        cfg_file.write_text(json.dumps(cfg_data, indent=2), encoding="utf-8")

        start_init = time.perf_counter()
        llm = LLM(
            model=model_path,
            trust_remote_code=True,
            tensor_parallel_size=1,
            gpu_memory_utilization=gpu_utilization,
            enforce_eager=True,
            disable_log_stats=True,
        )
        init_time = time.perf_counter() - start_init
    finally:
        cfg_file.write_text(cfg_backup, encoding="utf-8")
    print(f"✅ vLLM 引擎初始化完成，耗时: {init_time:.2f}s\n")

    sampling_params = SamplingParams(
        temperature=0.0,
        max_tokens=64,
        stop_token_ids=[tokenizer.eos_token_id] if tokenizer.eos_token_id else None,
    )

    results = {}

    print("-" * 64)
    print(f"{'Batch Size':<12} | {'总耗时(ms)':<12} | {'单条延迟(ms)':<14} | {'吞吐(tokens/s)':<14}")
    print("-" * 64)

    for bsize in batch_sizes:
        batch_items = (samples * ((bsize // len(samples)) + 2))[:bsize]
        prompts = [render_prompt_text(tokenizer, s["input"]) for s in batch_items]

        start_gen = time.perf_counter()
        outputs = llm.generate(prompts, sampling_params)
        elapsed_sec = time.perf_counter() - start_gen
        elapsed_ms = elapsed_sec * 1000.0

        total_gen_tokens = sum(len(o.outputs[0].token_ids) for o in outputs)
        tokens_per_sec = total_gen_tokens / elapsed_sec if elapsed_sec > 0 else 0.0
        avg_latency = elapsed_ms / bsize

        results[f"batch_{bsize}"] = {
            "batch_size": bsize,
            "elapsed_ms": elapsed_ms,
            "avg_latency_ms": avg_latency,
            "tokens_per_sec": tokens_per_sec,
        }

        print(f"{bsize:<12} | {elapsed_ms:<12.1f} | {avg_latency:<14.1f} | {tokens_per_sec:<14.2f}")

    print("-" * 64)
    return results


def main():
    parser = argparse.ArgumentParser(description="vLLM 快速推理性能测试")
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

    run_vllm_benchmark(args.model_path, args.test_data)


if __name__ == "__main__":
    main()
