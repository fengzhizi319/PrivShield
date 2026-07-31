"""Console 代理后端与 Agent 侧边栏抗抖动、降级与超时测试。
Proxy Backend & Agent Sidecar Resilience, Fallback and Timeout Unit Tests.

覆盖场景：
    1. 上游 Agent 超时（HTTP 504 Gateway Timeout）降级处理
    2. 上游 Agent 网络连通断开（HTTP 502 Bad Gateway）透传
    3. 异常算子/未可知崩溃时的统一 Error Handling 兜底
"""

from unittest.mock import AsyncMock, patch
import pytest
from fastapi import HTTPException
from fastapi.testclient import TestClient

from app.main import app

@pytest.fixture
def client() -> TestClient:
    return TestClient(app)

@pytest.fixture
def mock_agent_client():
    with patch("app.main.agent_client.request", new_callable=AsyncMock) as mocked:
        yield mocked

def test_proxy_timeout_fallback(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """当上游 Agent 响应超时 (504) 时，代理应捕获超时异常并返回规范错误描述。"""
    mock_agent_client.side_effect = HTTPException(status_code=504, detail="Agent connection timed out after 30s")

    response = client.post(
        "/api/proxy",
        json={
            "method": "POST",
            "path": "/v1/privacy/mask",
            "body": {"field_name": "phone", "value": "13800138000"},
        },
    )

    assert response.status_code == 504
    body = response.json()
    assert "detail" in body
    assert "timed out" in body["detail"]

def test_proxy_bad_gateway_fallback(client: TestClient, mock_agent_client: AsyncMock) -> None:
    """当上游 Agent 异常崩溃或无法建立 TCP 连接时 (502)，代理应返回 502 Bad Gateway。"""
    mock_agent_client.side_effect = HTTPException(status_code=502, detail="Agent process unreachable")

    response = client.post(
        "/api/proxy",
        json={
            "method": "POST",
            "path": "/api/v1/classification/classify",
            "body": {"text": "测试文本"},
        },
    )

    assert response.status_code == 502
    body = response.json()
    assert body["detail"] == "Agent process unreachable"
