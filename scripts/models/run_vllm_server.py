#!/usr/bin/env python3
"""vLLM Server Launcher in Python for PrivShield.

Loads environment variables from .env and executes vLLM entrypoint.
Usage:
    python run_vllm_server.py [--host HOST] [--port PORT] [--model PATH]
"""

from __future__ import annotations

import argparse
import os
import sys
import subprocess
from pathlib import Path

def load_env_file(path: Path) -> None:
    if not path.exists():
        return
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k, v = k.strip(), v.strip().strip("'\"")
            if k and k not in os.environ:
                os.environ[k] = v


def main() -> None:
    parser = argparse.ArgumentParser(description="启动 vLLM OpenAI 兼容 HTTP 推理服务")
    parser.add_argument("--host", default=None, help="监听地址 (默认 127.0.0.1)")
    parser.add_argument("--port", default=None, help="监听端口 (默认 8000)")
    parser.add_argument("--model", default=None, help="模型权重路径或 HuggingFace ID")
    parser.add_argument("--gpu-util", default=None, help="GPU 显存占用比例 (默认 0.90)")
    args = parser.parse_args()

    # 自动装载项目根目录下的 .env
    root_dir = Path(__file__).resolve().parents[2]
    load_env_file(root_dir / ".env")

    host = args.host or os.environ.get("PRIVACY_LLM_API_HOST", "127.0.0.1")
    port = args.port or os.environ.get("PRIVACY_LLM_API_PORT", "8000")
    model_path = args.model or os.environ.get(
        "PRIVACY_LLM_MODEL_PATH",
        ".models/Qwen3.5-0.8B-Privacy-Classifier-Smoother",
    )
    served_name = os.environ.get(
        "PRIVACY_LLM_MODEL_NAME",
        "Qwen3.5-0.8B-Privacy-Classifier-Smoother",
    )
    gpu_util = args.gpu_util or os.environ.get("PRIVACY_VLLM_GPU_MEMORY_UTILIZATION", "0.90")

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
        str(port),
        "--served-model-name",
        served_name,
        "--trust-remote-code",
        "--gpu-memory-utilization",
        str(gpu_util),
    ]

    try:
        subprocess.run(cmd, check=True)
    except KeyboardInterrupt:
        print("\n🛑 vLLM 服务已安全停止。")
        sys.exit(0)


if __name__ == "__main__":
    main()
