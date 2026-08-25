#!/usr/bin/env python3
"""vLLM Server Launcher in Python for PrivShield.

Loads environment variables from .env and executes vLLM entrypoint.
Usage:
    python run_vllm_server.py
"""

from __future__ import annotations

import os
import sys
import subprocess
from pathlib import Path

from engine.env_loader import load_env_file


def main() -> None:
    # 自动装载项目根目录下的 .env
    root_dir = Path(__file__).resolve().parents[2]
    load_env_file(root_dir / ".env")

    host = os.environ.get("PRIVACY_LLM_API_HOST", "127.0.0.1")
    port = os.environ.get("PRIVACY_LLM_API_PORT", "8000")
    model_path = os.environ.get(
        "PRIVACY_LLM_MODEL_PATH",
        ".models/Qwen3.5-0.8B-Privacy-Classifier-Smoother",
    )
    served_name = os.environ.get(
        "PRIVACY_LLM_MODEL_NAME",
        "Qwen3.5-0.8B-Privacy-Classifier-Smoother",
    )
    gpu_util = os.environ.get("PRIVACY_VLLM_GPU_MEMORY_UTILIZATION", "0.90")

    if not Path(model_path).exists():
        print(f"⚠️  未在 {model_path} 找到本地模型目录")
        if served_name and served_name != "Qwen3.5-0.8B-Privacy-Classifier-Smoother":
            model_path = served_name
        else:
            model_path = "Qwen/Qwen3.5-0.8B"
        print(f"ℹ️  改用 HuggingFace 开源权重: {model_path}")

    print("🚀 启动 vLLM OpenAI 兼容 HTTP 服务...")
    print(f"  地址: http://{host}:{port}/v1/chat/completions")
    print(f"  模型: {model_path} (对外标识: {served_name})")

    cmd = [
        sys.executable,
        "-m",
        "vllm.entrypoints.openai.api_server",
        "--model",
        model_path,
        "--host",
        host,
        "--port",
        port,
        "--served-model-name",
        served_name,
        "--trust-remote-code",
        "--gpu-memory-utilization",
        gpu_util,
    ]

    try:
        subprocess.run(cmd, check=True)
    except FileNotFoundError:
        print("❌ 错误: 未找到 python/vllm 可执行包。请先安装: pip install vllm")
        sys.exit(1)
    except KeyboardInterrupt:
        print("\n👋 vLLM 服务正常停止")


if __name__ == "__main__":
    main()
