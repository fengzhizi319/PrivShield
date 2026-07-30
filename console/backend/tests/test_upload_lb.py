"""文件上传（/api/upload）与负载均衡测试（/api/lb_test）端点的单元测试。
Unit tests for file upload (/api/upload) and load balancing test (/api/lb_test) endpoints.

测试策略 / Testing Strategy：
    - ``/api/upload``：对 ``agent_client.request_multipart`` 打桩，验证后端
      正确以 multipart 转发到 agent 并包装为 ProxyResponse，无需真实 agent；
      Stubs ``agent_client.request_multipart``, verifies backend correctly forwards
      as multipart to agent and wraps as ProxyResponse, no real agent needed;
    - ``/api/lb_test``：用 ``httpx.MockTransport`` 注入两个假后端，直接调用
      ``_run_lb_test`` 验证 round_robin 均匀分发与统计字段，并测试端点接线。
      Uses ``httpx.MockTransport`` to inject two fake backends, directly calls
      ``_run_lb_test`` to verify round_robin even distribution and stats fields.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, patch

import httpx
import pytest
from fastapi.testclient import TestClient

from app.main import (
    LbBackend,
    LbTestRequest,
    _lb_pick_backends,
    _run_lb_test,
    _validate_lb_url,
    app,
)


@pytest.fixture
def client() -> TestClient:
    """提供包裹 FastAPI 应用的测试客户端。
    Provide a test client wrapping the FastAPI application.
    """
    return TestClient(app)


@pytest.fixture
def mock_multipart():
    """对模块级 ``agent_client.request_multipart`` 异步方法打桩。
    Stub the module-level ``agent_client.request_multipart`` async method.
    """
    with patch(
        "app.main.agent_client.request_multipart", new_callable=AsyncMock
    ) as mocked:
        yield mocked


# --------------------------------------------------------------------------- #
# /api/upload
# --------------------------------------------------------------------------- #
def test_upload_forwards_multipart(client: TestClient, mock_multipart: AsyncMock) -> None:
    """上传 CSV 应经 request_multipart 转发到 agent 并包装为 ProxyResponse。
    Uploading CSV should forward via request_multipart to agent and wrap as ProxyResponse.
    """
    mock_multipart.return_value = {
        "operation": "mask_dataframe",
        "rows_in": 2,
        "rows_out": 2,
        "result": [{"email": "a***@example.com"}],
    }

    csv_bytes = b"email,phone\nalice@example.com,13800138000\nbob@example.com,13900139000\n"
    response = client.post(
        "/api/upload",
        files={"file": ("data.csv", csv_bytes, "text/csv")},
        data={"operation": "mask_dataframe", "params": json.dumps({"columns": ["email"]})},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == 200
    assert body["data"]["operation"] == "mask_dataframe"
    assert body["data"]["rows_out"] == 2
    assert "duration_ms" in body
    # 后端身份标识随上传响应一同下发，供前端验证切换生效。
    # Backend identity fields included in upload response, for frontend to verify switch.
    assert body["via"] == "python-rest"
    assert body["protocol"] == "REST"

    # 验证转发参数：目标路径与表单字段
    # Verify forwarding parameters: target path and form fields
    args, kwargs = mock_multipart.call_args
    assert args[0] == "/v1/privacy/process_file"
    assert kwargs["data"]["operation"] == "mask_dataframe"
    # files 中携带了文件名与内容
    # files carry the filename and content
    forwarded = kwargs["files"]["file"]
    assert forwarded[0] == "data.csv"
    assert forwarded[1] == csv_bytes


def test_upload_upstream_error(client: TestClient, mock_multipart: AsyncMock) -> None:
    """agent 返回错误时，/api/upload 应透传状态码与 detail。
    When agent returns an error, /api/upload should passthrough status code and detail.
    """
    from fastapi import HTTPException

    mock_multipart.side_effect = HTTPException(status_code=400, detail="仅支持 .csv 与 .json 文件")

    response = client.post(
        "/api/upload",
        files={"file": ("data.txt", b"hello", "text/plain")},
        data={"operation": "mask_dataframe"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "仅支持 .csv 与 .json 文件"


# --------------------------------------------------------------------------- #
# /api/lb_test
# --------------------------------------------------------------------------- #
def _mock_transport() -> httpx.MockTransport:
    """构造一个对所有探测请求返回 200 的假后端 transport。
    Construct a fake backend transport that returns 200 for all probe requests.
    """

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"status": "ok"})

    return httpx.MockTransport(handler)


@pytest.mark.anyio
async def test_run_lb_test_round_robin_distribution() -> None:
    """round_robin 策略下 6 个请求应均匀分发到 2 个节点，统计字段完整。
    Under round_robin strategy, 6 requests should distribute evenly to 2 nodes with complete stats.
    """
    req = LbTestRequest(
        backends=[
            LbBackend(name="a", url="http://backend-a"),
            LbBackend(name="b", url="http://backend-b"),
        ],
        num_requests=6,
        strategy="round_robin",
    )

    # SSRF 加固会真实解析 DNS；单测用 MockTransport 假后端，需 mock DNS 返回公网 IP 隔离网络 I/O。
    # SSRF hardening resolves DNS for real; unit tests use MockTransport fake backends, need to mock DNS
    # returning public IPs to isolate network I/O.
    with patch("app.main._resolve_host_ips", return_value={"93.184.216.34"}):
        resp = await _run_lb_test(req, transport=_mock_transport())

    assert resp.strategy == "round_robin"
    assert resp.total == 6
    assert resp.success == 6
    assert resp.failed == 0
    assert len(resp.distribution) == 2
    # 均匀分发：每个节点各命中 3 次
    # Even distribution: each node hit exactly 3 times
    counts = {d.name: d.count for d in resp.distribution}
    assert counts == {"a": 3, "b": 3}
    for item in resp.distribution:
        assert item.success == item.count
        assert item.failed == 0
        assert item.avg_latency_ms >= 0
        assert item.min_latency_ms <= item.avg_latency_ms <= item.max_latency_ms


@pytest.mark.anyio
async def test_run_lb_test_failed_probe() -> None:
    """探测返回 500 时应计入 failed。
    When probe returns 500, it should count as failed.
    """

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"error": "boom"})

    req = LbTestRequest(
        backends=[LbBackend(name="a", url="http://backend-a")],
        num_requests=3,
        strategy="round_robin",
    )
    # mock DNS 解析（假后端域名无法真实解析），隔离网络 I/O。
    # Mock DNS resolution (fake backend domains can't resolve for real), isolate network I/O.
    with patch("app.main._resolve_host_ips", return_value={"93.184.216.34"}):
        resp = await _run_lb_test(req, transport=httpx.MockTransport(handler))

    assert resp.total == 3
    assert resp.success == 0
    assert resp.failed == 3
    assert resp.distribution[0].failed == 3


@pytest.mark.anyio
async def test_run_lb_test_empty_backends() -> None:
    """backends 为空时应抛出 400。
    Should raise 400 when backends is empty.
    """
    from fastapi import HTTPException

    req = LbTestRequest(backends=[], num_requests=3, strategy="round_robin")
    with pytest.raises(HTTPException) as excinfo:
        await _run_lb_test(req, transport=_mock_transport())
    assert excinfo.value.status_code == 400


def test_lb_pick_backends_strategies() -> None:
    """三种策略生成的下标序列均合法且长度正确。
    All three strategies should produce valid index sequences with correct length.
    """
    for strategy in ("round_robin", "random", "least_connections"):
        seq = _lb_pick_backends(strategy, 10, 3)
        assert len(seq) == 10
        assert all(0 <= i < 3 for i in seq)


def test_lb_pick_backends_invalid_strategy() -> None:
    """未知策略应抛出 400。
    Unknown strategy should raise 400.
    """
    from fastapi import HTTPException

    with pytest.raises(HTTPException) as excinfo:
        _lb_pick_backends("foobar", 5, 2)
    assert excinfo.value.status_code == 400


def test_lb_test_endpoint_empty_backends(client: TestClient) -> None:
    """端点层：backends 为空时返回 400。
    Endpoint layer: returns 400 when backends is empty.
    """
    response = client.post(
        "/api/lb_test",
        json={"backends": [], "num_requests": 3, "strategy": "round_robin"},
    )
    assert response.status_code == 400


# --------------------------------------------------------------------------- #
# 安全加固：SSRF 防护 + 上传大小限制
# --------------------------------------------------------------------------- #
@pytest.mark.anyio
async def test_run_lb_test_invalid_scheme() -> None:
    """探测地址 scheme 非 http/https 时应抛出 400（SSRF 防护）。
    Should raise 400 when probe URL scheme is not http/https (SSRF protection).
    """
    from fastapi import HTTPException

    req = LbTestRequest(
        backends=[LbBackend(name="a", url="file:///etc/passwd")],
        num_requests=1,
        strategy="round_robin",
    )
    with pytest.raises(HTTPException) as excinfo:
        await _run_lb_test(req, transport=_mock_transport())
    assert excinfo.value.status_code == 400


def test_lb_test_endpoint_invalid_scheme(client: TestClient) -> None:
    """端点层：非法 scheme（如 gopher://）返回 400。
    Endpoint layer: invalid scheme (e.g. gopher://) returns 400.
    """
    response = client.post(
        "/api/lb_test",
        json={
            "backends": [{"name": "a", "url": "gopher://127.0.0.1:6379"}],
            "num_requests": 1,
            "strategy": "round_robin",
        },
    )
    assert response.status_code == 400


def test_validate_lb_url_allowlist() -> None:
    """配置 host 白名单时，未命中的 host 应抛出 400。
    When host allowlist is configured, non-matching hosts should raise 400.
    """
    from fastapi import HTTPException

    from app.main import settings

    # mock DNS 解析返回公网 IP，隔离真实网络 I/O（本用例仅验证白名单逻辑）。
    # Mock DNS resolution returning public IP, isolate real network I/O (this case only verifies allowlist logic).
    with patch("app.main._resolve_host_ips", return_value={"93.184.216.34"}):
        with patch.object(settings, "lb_allowed_hosts", "trusted.local"):
            # 命中白名单：正常通过（不抛异常）。
            # Matches allowlist: passes normally (no exception).
            _validate_lb_url("http://trusted.local:8079")
            # 未命中白名单：400。
            # Not in allowlist: 400.
            with pytest.raises(HTTPException) as excinfo:
                _validate_lb_url("http://evil.local")
            assert excinfo.value.status_code == 400


def test_upload_oversized_file_returns_413(client: TestClient, mock_multipart: AsyncMock) -> None:
    """上传超过大小上限的文件应返回 413，且不转发到 agent。
    Uploading a file exceeding size limit should return 413 without forwarding to agent.
    """
    from app.main import settings

    big = b"x" * 100
    with patch.object(settings, "max_upload_bytes", 10):
        response = client.post(
            "/api/upload",
            files={"file": ("big.csv", big, "text/csv")},
            data={"operation": "mask_dataframe"},
        )
    assert response.status_code == 413
    # 超限请求不应转发到 agent。
    # Oversized requests should not be forwarded to agent.
    mock_multipart.assert_not_called()
