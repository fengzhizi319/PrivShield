"""Python FastAPI 代理后端的单元测试。
Unit tests for the Python FastAPI proxy backend.

测试策略 / Testing Strategy：
    - 使用 ``fastapi.testclient.TestClient`` 直接调用应用路由，无需真实启动服务；
      Uses ``fastapi.testclient.TestClient`` to invoke routes directly, no real server needed;
    - 通过 ``unittest.mock`` 对上游 ``agent_client.request`` 打桩，
      因此**不需要**运行中的 privacy-local-agent；
      Stubs upstream ``agent_client.request`` via ``unittest.mock``,
      so a running privacy-local-agent is **not required**;
    - 覆盖公开 API 面：``/api/health``、``/api/samples``、``/api/proxy`` 的
      正常、上游不可达、参数校验与上游错误透传等场景。
      Covers public API surface: normal, upstream-unreachable, validation,
      and upstream-error-passthrough scenarios for ``/api/health``, ``/api/samples``, ``/api/proxy``.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from app.fixtures.samples import get_samples
from app.main import app


@pytest.fixture
def client() -> TestClient:
    """提供包裹 FastAPI 应用的测试客户端。
    Provide a test client wrapping the FastAPI application.
    """
    return TestClient(app)


@pytest.fixture
def mock_agent_client():
    """对模块级 ``agent_client.request`` 异步方法打桩。
    Stub the module-level ``agent_client.request`` async method.

    使用 ``AsyncMock`` 以便能用 ``return_value`` / ``side_effect`` 控制
    异步调用的返回与异常，隔离对真实 agent 的依赖。
    Uses ``AsyncMock`` so ``return_value`` / ``side_effect`` can control
    async call returns and exceptions, isolating from the real agent.
    """
    with patch("app.main.agent_client.request", new_callable=AsyncMock) as mocked:
        yield mocked


def test_health_ok(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """agent 可达时，/api/health 应返回 backend/agent 双正常与延迟字段。
    When agent is reachable, /api/health should return backend/agent both ok with latency field.
    """
    mock_agent_client.return_value = {"status": "ok", "namespace": "default"}

    response = client.get("/api/health")

    assert response.status_code == 200
    body = response.json()
    assert body["backend"] == "ok"
    assert body["agent"]["status"] == "ok"
    assert body["agent_url"] == "http://127.0.0.1:8079"
    assert "latency_ms" in body
    # 后端身份标识：Python 后端恒为 python-rest / REST，供前端验证切换生效。
    # Backend identity: Python backend is always python-rest / REST, for frontend to verify switch took effect.
    assert body["via"] == "python-rest"
    assert body["protocol"] == "REST"


def test_health_agent_unreachable(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """agent 不可达时，/api/health 仍返回 200，但 agent 字段为 unreachable。
    When agent is unreachable, /api/health still returns 200 but agent field is unreachable.
    """
    from fastapi import HTTPException

    mock_agent_client.side_effect = HTTPException(status_code=502, detail="connection refused")

    response = client.get("/api/health")

    assert response.status_code == 200
    body = response.json()
    assert body["backend"] == "ok"
    assert body["agent"] == "unreachable"
    assert "error" in body
    # 即使 agent 不可达，身份标识仍应下发。
    # Even when agent is unreachable, identity fields should still be present.
    assert body["via"] == "python-rest"
    assert body["protocol"] == "REST"


def test_samples(client: TestClient) -> None:
    """/api/samples 应返回与 get_samples() 数量一致的示例列表。
    /api/samples should return a sample list matching get_samples() count.
    """
    response = client.get("/api/samples")

    assert response.status_code == 200
    body = response.json()
    assert "samples" in body
    assert len(body["samples"]) == len(get_samples())
    assert body["samples"][0]["path"]
    # 跨后端一致性：profile/recommend 与 Go 后端 samples 对齐，
    # agent 支持且 Go 已提供，Python 端不应缺失。
    paths = {s["path"] for s in body["samples"]}
    assert "/v1/privacy/profile/recommend" in paths


def test_proxy_json(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """/api/proxy 转发 JSON 请求，应包装为 status/duration_ms/data 结构。
    /api/proxy forwarding JSON requests should wrap in status/duration_ms/data structure.
    """
    mock_agent_client.return_value = {"result": "a***@example.com"}

    response = client.post(
        "/api/proxy",
        json={
            "method": "POST",
            "path": "/v1/privacy/mask",
            "body": {"field_name": "email", "value": "alice@example.com"},
        },
    )

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == 200
    assert body["data"]["result"] == "a***@example.com"
    assert "duration_ms" in body
    # 后端身份标识随代理响应一同下发。
    # Backend identity fields are included in proxy response.
    assert body["via"] == "python-rest"
    assert body["protocol"] == "REST"


def test_proxy_invalid_body(client: TestClient) -> None:
    """缺少必填字段（path）时，Pydantic v2 应返回 422 校验错误。
    When required field (path) is missing, Pydantic v2 should return 422 validation error.
    """
    response = client.post("/api/proxy", json={"method": "POST"})

    # Pydantic v2 默认对缺失必填字段返回 422。
    # Pydantic v2 returns 422 by default for missing required fields.
    assert response.status_code == 422
    body = response.json()
    assert "detail" in body


def test_proxy_upstream_error(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """上游 agent 返回错误时，/api/proxy 应透传状态码与 detail。
    When upstream agent returns an error, /api/proxy should passthrough status code and detail.
    """
    from fastapi import HTTPException

    mock_agent_client.side_effect = HTTPException(status_code=422, detail="invalid field")

    response = client.post(
        "/api/proxy",
        json={
            "method": "POST",
            "path": "/v1/privacy/mask",
            "body": {"field_name": "unknown", "value": "x"},
        },
    )

    assert response.status_code == 422
    body = response.json()
    assert body["detail"] == "invalid field"


def test_proxy_dynclassification(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """测试 /api/proxy 代理转发 /v1/dynclassification/eval 动态分类请求。
    Test /api/proxy forwarding /v1/dynclassification/eval dynamic classification request.
    """
    mock_agent_client.return_value = {
        "fieldResult": {"fieldName": "mobile_phone", "finalLevel": "L3"},
        "auditInfo": {"domain": "general-pii"},
    }

    response = client.post(
        "/api/proxy",
        json={
            "method": "POST",
            "path": "/v1/dynclassification/eval",
            "body": {"fieldName": "mobile_phone", "value": "13800138000", "domain": "general-pii"},
        },
    )

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == 200
    assert body["data"]["fieldResult"]["finalLevel"] == "L3"
    assert body["via"] == "python-rest"
    assert body["protocol"] == "REST"

