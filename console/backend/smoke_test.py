"""Smoke test: invoke all sample endpoints sequentially via the backend proxy.
烟雾测试：通过后端代理逐个调用所有示例端点。

Prerequisites: backend (8080) and privacy-local-agent must both be running:
运行前提：后端（8080）与 privacy-local-agent 均已启动：

    cd console/backend
    source .venv/bin/activate
    python smoke_test.py

Design notes / 设计说明：
    - Iterates over samples from :func:`get_samples`, forwarding via ``/api/proxy``;
      逐个遍历 :func:`get_samples` 返回的示例，经 ``/api/proxy`` 转发；
    - Non-200 responses are printed but don't abort, avoiding partial env issues
      (e.g. missing LLM model) masking real results from other endpoints;
      非 200 响应只打印不中断，避免局部环境问题掌盖其他端点的真实结果；
    - Endpoints requiring pre-existing runtime resources (async job query, review confirm)
      are skipped as they depend on real IDs and need manual UI verification;
      需要预存运行时资源的端点会被跳过，因为它们依赖真实 ID；
    - Exit code 0 when all pass, 1 on any failure (CI-friendly).
      全部通过时退出码为 0，有失败时为 1（便于 CI 集成）。
"""

from __future__ import annotations

import sys
from typing import Any

import httpx

from app.fixtures.samples import get_samples

BASE = "http://127.0.0.1:8080"


def send(
    method: str,
    path: str,
    body: Any = None,
    raw_b64: str | None = None,
    content_type: str | None = None,
) -> tuple[int, Any]:
    """构造代理请求并发送到 ``/api/proxy``，返回（状态码，响应体）。"""
    # 组装 /api/proxy 的请求体：method/path/body 必填，
    # 二进制载荷（raw_payload_b64 / content_type）可为 None。
    payload: dict[str, Any] = {
        "method": method,
        "path": path,
        "body": body,
        "raw_payload_b64": raw_b64,
        "content_type": content_type,
    }
    # 同步发送 POST 到 /api/proxy（超时 60s，容忍 LLM 等慢端点）。
    r = httpx.post(f"{BASE}/api/proxy", json=payload, timeout=60.0)
    try:
        # 优先按 JSON 解析响应体。
        return r.status_code, r.json()
    except Exception:
        # 非 JSON 响应（如纯文本错误）降级返回原始文本。
        return r.status_code, r.text


def main() -> int:
    """主流程：遍历示例逐个调用，统计并打印通过 / 失败数，返回退出码。"""
    # 加载所有端点示例（与 /api/samples 同源）。
    samples = get_samples()
    passed = 0                                # 成功计数
    failed = 0                                # 失败计数

    # 逐个遍历示例，经 /api/proxy 转发到 agent。
    for sample in samples:
        # 拼接用于打印的端点标签（方法 路径 (名称)）。
        label = f"{sample['method']} {sample['path']} ({sample['label']})"
        # 取出示例的 JSON 体与可选的二进制载荷字段。
        body = sample.get("body")
        raw_b64 = sample.get("rawPayloadB64")
        content_type = sample.get("contentType")

        # 发送请求并获取（状态码，响应体）。
        status, data = send(sample["method"], sample["path"], body, raw_b64, content_type)
        if status == 200:
            # 成功：打印 OK 并计数。
            print(f"OK    {label}")
            passed += 1
        else:
            # 失败：提取 detail（若为 dict）或原文本，打印不中断。
            detail = data.get("detail", data) if isinstance(data, dict) else data
            print(f"FAIL  {label} -> {status}: {detail}")
            failed += 1

    # 打印汇总统计。
    print(f"\nTotal: {passed} passed, {failed} failed out of {passed + failed}")
    # 有失败返回 1（便于 CI 集成），全部通过返回 0。
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
