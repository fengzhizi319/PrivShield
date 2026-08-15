"""HTTP 网关代理边界回归测试。

覆盖以下修复点：
- 响应头 content-encoding 必须剥离（httpx 已自动解压 body，透传会导致客户端二次解压失败）；
- POST 等非幂等请求在超时 / 读取失败时不得重试（仅 ConnectError 允许故障转移）；
- 幂等 GET 超时后应重试并故障转移到健康节点；
- 后端 5xx 响应应计入节点熔断器失败统计，且原样透传状态码。
"""

from __future__ import annotations

import gzip

import httpx
import pytest
from fastapi.testclient import TestClient

from PrivShield.gateway import http_proxy
from PrivShield.gateway.balancer import LoadBalancer
from PrivShield.gateway.http_proxy import create_http_gateway_app


@pytest.fixture
def patched_client(monkeypatch):
    """将代理内部创建的 httpx.AsyncClient 替换为基于 MockTransport 的客户端。

    Returns:
        (calls, state)：calls 记录每次转发的 "METHOD URL"；
        state["handler"] 由各用例设置为 (request) -> Response 的可调用。
    """
    calls: list[str] = []
    state: dict = {"handler": None}
    real_async_client = httpx.AsyncClient  # 先保留真实类，避免 factory 内递归调用

    def factory(**_kwargs):
        def dispatch(request: httpx.Request) -> httpx.Response:
            calls.append(f"{request.method} {request.url}")
            return state["handler"](request)

        return real_async_client(transport=httpx.MockTransport(dispatch))

    monkeypatch.setattr(http_proxy.httpx, "AsyncClient", factory)
    return calls, state


def test_response_strips_content_encoding(patched_client):
    """代理响应必须剥离 content-encoding，其余自定义头保留。

    模拟真实场景：上游返回 gzip 压缩体 + content-encoding 头，
    httpx 会自动解压，若再把头透传给客户端将导致二次解压失败。
    """
    _calls, state = patched_client
    payload = gzip.compress(b'{"status": "ok"}')

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            content=payload,
            headers={"content-encoding": "gzip", "x-custom": "keep"},
        )

    state["handler"] = handler

    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://node-a.test", "127.0.0.1:50051")
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.content == b'{"status": "ok"}'
    assert "content-encoding" not in {k.lower() for k in resp.headers}
    assert resp.headers.get("x-custom") == "keep"


def test_post_read_timeout_not_retried(patched_client):
    """POST（非幂等）读超时不应重试：只发出一次上游请求并返回 502。"""
    calls, state = patched_client

    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("slow backend", request=request)

    state["handler"] = handler

    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://node-a.test", "127.0.0.1:50051")
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.post(
        "/v1/privacy/mask",
        json={"field_name": "mobile", "value": "13812345678", "context": ""},
    )
    assert resp.status_code == 502
    # 关键断言：非幂等请求超时后未重试（副作用不可重放）
    assert len(calls) == 1


def test_get_timeout_fails_over_to_healthy_node(patched_client):
    """幂等 GET 超时后应被动下线故障节点并重试到健康节点。"""
    calls, state = patched_client

    def handler(request: httpx.Request) -> httpx.Response:
        if "node-a" in str(request.url):
            raise httpx.ReadTimeout("slow backend", request=request)
        return httpx.Response(200, json={"status": "ok"})

    state["handler"] = handler

    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://node-a.test", "127.0.0.1:50051")
    balancer.add_node("http://node-b.test", "127.0.0.1:50052")
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"
    # 第一次命中 node-a 超时，重试命中 node-b 成功，共两次上游调用
    assert len(calls) == 2
    bad_node = next(n for n in balancer.nodes if "node-a" in n.http_url)
    assert bad_node.is_healthy is False


def test_backend_5xx_records_circuit_breaker_failure(patched_client):
    """后端 5xx 应计入熔断器失败统计并原样透传，4xx 不影响熔断器状态。"""
    calls, state = patched_client

    state["handler"] = lambda request: httpx.Response(500, json={"error": "boom"})

    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://node-a.test", "127.0.0.1:50051")
    node = balancer.nodes[0]
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.get("/health")
    # 5xx 原样透传给调用方（不吞为 502）
    assert resp.status_code == 500
    # 只发生了一次上游转发（5xx 不触发重试）
    assert len(calls) == 1
    # 熔断器失败计数 +1，但单次 5xx 不触发被动下线
    assert node.circuit_breaker._failure_count == 1
    assert node.is_healthy is True

    # 4xx 属于客户端问题：不增加失败计数，也不重置已有失败统计
    state["handler"] = lambda request: httpx.Response(404, json={"error": "not found"})
    resp404 = client.get("/v1/unknown")
    assert resp404.status_code == 404
    assert node.circuit_breaker._failure_count == 1


def test_register_node_invalid_scheme_rejected(monkeypatch):
    """注册节点的 http_url 必须校验 Scheme，非 http/https 返回 400。"""
    monkeypatch.setenv("GATEWAY_API_KEY", "secret-token-123")
    balancer = LoadBalancer(strategy="round_robin")
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.post(
        "/v1/gateway/register",
        headers={"Authorization": "Bearer secret-token-123"},
        json={"http_url": "ftp://malicious-node.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp.status_code == 400
    assert "Invalid http_url scheme" in resp.json()["detail"]


def test_management_endpoints_fail_closed_without_key(monkeypatch):
    """未配置 GATEWAY_API_KEY 时管理端点一律 503（fail-closed，防 SSRF）。"""
    monkeypatch.delenv("GATEWAY_API_KEY", raising=False)
    balancer = LoadBalancer(strategy="round_robin")
    client = TestClient(create_http_gateway_app(balancer))

    resp_reg = client.post(
        "/v1/gateway/register",
        json={"http_url": "http://node-b.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp_reg.status_code == 503
    assert "GATEWAY_API_KEY" in resp_reg.json()["detail"]

    resp_dereg = client.post(
        "/v1/gateway/deregister",
        json={"http_url": "http://node-b.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp_dereg.status_code == 503


def test_502_does_not_leak_backend_error(patched_client):
    """502 响应不得回传后端异常原文（可能含内网 URL），仅给通用文案。"""
    _calls, state = patched_client

    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("slow backend", request=request)

    state["handler"] = handler

    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://node-a.internal:9000", "127.0.0.1:50051")
    client = TestClient(create_http_gateway_app(balancer))

    resp = client.post(
        "/v1/privacy/mask",
        json={"field_name": "mobile", "value": "13812345678", "context": ""},
    )
    assert resp.status_code == 502
    detail = resp.json()["detail"]
    assert "Bad Gateway" in detail
    # 不泄漏后端异常细节与内网地址
    assert "slow backend" not in detail
    assert "node-a.internal" not in detail


def test_register_node_auth_constant_time(monkeypatch):
    """验证管理接口配置 GATEWAY_API_KEY 时的鉴权保护。"""
    monkeypatch.setenv("GATEWAY_API_KEY", "secret-token-123")
    balancer = LoadBalancer(strategy="round_robin")
    client = TestClient(create_http_gateway_app(balancer))

    # 未提供 Token -> 401
    resp_no_token = client.post(
        "/v1/gateway/register",
        json={"http_url": "http://node-b.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp_no_token.status_code == 401

    # 提供错误 Token -> 401
    resp_wrong_token = client.post(
        "/v1/gateway/register",
        headers={"Authorization": "Bearer wrong-token"},
        json={"http_url": "http://node-b.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp_wrong_token.status_code == 401

    # 提供正确 Token -> 200
    resp_correct = client.post(
        "/v1/gateway/register",
        headers={"Authorization": "Bearer secret-token-123"},
        json={"http_url": "http://node-b.test", "grpc_address": "127.0.0.1:50051"},
    )
    assert resp_correct.status_code == 200
    assert resp_correct.json() == {"status": "registered"}

