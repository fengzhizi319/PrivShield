"""HTTP 反向代理网关模块。

基于 FastAPI 实现通配路由代理，将 REST 请求透明转发至后端健康节点，
支持故障重试、被动健康检测与 Prometheus 指标采集。

HTTP reverse-proxy gateway module.

Implements a wildcard-route proxy on FastAPI that transparently forwards REST
requests to healthy backend nodes with retry, passive health detection, and
Prometheus metrics instrumentation.
"""

from __future__ import annotations

import asyncio
import hmac
import os
import time
from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI, Header, HTTPException, Request, Response
from pydantic import BaseModel

from PrivShield.observability.logging_config import get_logger
from PrivShield.observability.metrics import (
    GATEWAY_LATENCY,
    GATEWAY_REQUESTS_TOTAL,
    GATEWAY_RETRIES_TOTAL,
)

from .balancer import LoadBalancer, backend_tls_verify

logger = get_logger(__name__)


# RFC 7230 规定的逐段传输头 (Hop-by-hop headers)，在代理转发时不应向下传递
EXCLUDE_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "content-length",
    "host",
}

# 响应方向额外需要剔除的头。
# httpx 在读取 ``resp.content`` 时会自动解压 gzip/deflate/br 等编码，
# 若把 ``content-encoding`` 原样透传给客户端，客户端会对已解压的
# 明文再次尝试解压，导致响应解析失败。
RESPONSE_EXCLUDE_HEADERS = EXCLUDE_HEADERS | {"content-encoding"}


class RegisterRequest(BaseModel):
    """节点动态注册请求模型 / Node registration request model."""

    http_url: str
    grpc_address: str
    weight: int = 1


class DeregisterRequest(BaseModel):
    """节点动态注销请求模型 / Node deregistration request model."""

    http_url: str
    grpc_address: str


def create_http_gateway_app(balancer: LoadBalancer) -> FastAPI:
    """创建并初始化 HTTP 网关 FastAPI 应用 / Create HTTP gateway FastAPI app.

    Args:
        balancer: 关联的负载均衡实例。

    Returns:
        初始化后的 FastAPI 应用实例。
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        # 初始化应用级单例 HTTP 客户端，并优化连接池配置
        app.state.http_client = httpx.AsyncClient(
            timeout=httpx.Timeout(30.0),
            limits=httpx.Limits(max_keepalive_connections=100, max_connections=500),
            trust_env=False,  # 禁用环境变量代理，防止本地转发流量被拦截
            verify=backend_tls_verify(),  # 回源 TLS 启用时按 CA 校验后端证书
        )
        yield
        # 优雅释放连接池
        await app.state.http_client.aclose()

    app = FastAPI(title="SecretFlow Local Privacy Agent REST Gateway", lifespan=lifespan)
    gateway_api_key = os.environ.get("GATEWAY_API_KEY", "")

    def require_management_auth(authorization: str | None) -> None:
        """Protect topology mutation endpoints with an operator key (fail-closed).

        未配置 ``GATEWAY_API_KEY`` 时管理端点一律返回 503（fail-closed）：
        拓扑注册/注销可借 SSRF 把流量引向任意内网地址，绝不能默认放行。
        本地开发如需使用，必须显式设置一个已知值（不接受空值）。
        """
        if not gateway_api_key:
            logger.warning(
                "Gateway management endpoint rejected: GATEWAY_API_KEY is not configured; "
                "set it explicitly to enable /v1/gateway/register|deregister"
            )
            raise HTTPException(
                status_code=503,
                detail="Gateway management API is disabled: GATEWAY_API_KEY is not configured",
            )
        expected = f"Bearer {gateway_api_key}"
        if not authorization or not hmac.compare_digest(authorization.encode("utf-8"), expected.encode("utf-8")):
            raise HTTPException(status_code=401, detail="Unauthorized gateway management request")

    @app.post("/v1/gateway/register")
    async def register_node(req: RegisterRequest, authorization: str | None = Header(default=None)):
        """动态注册工作节点 / Register a worker node to the pool."""
        require_management_auth(authorization)
        if not (req.http_url.startswith("http://") or req.http_url.startswith("https://")):
            raise HTTPException(status_code=400, detail="Invalid http_url scheme, must be http:// or https://")
        balancer.add_node(req.http_url, req.grpc_address, req.weight)
        logger.info(
            "Node registered via API",
            extra={"http_url": req.http_url, "grpc_address": req.grpc_address},
        )
        return {"status": "registered"}

    @app.post("/v1/gateway/deregister")
    async def deregister_node(req: DeregisterRequest, authorization: str | None = Header(default=None)):
        """注销工作节点 / Deregister a worker node from the pool."""
        require_management_auth(authorization)
        balancer.remove_node(req.http_url, req.grpc_address)
        logger.info(
            "Node deregistered via API",
            extra={"http_url": req.http_url, "grpc_address": req.grpc_address},
        )
        return {"status": "deregistered"}

    @app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"])
    async def proxy_request(path: str, request: Request):
        """通配路由代理 / Wildcard proxy route with retry and metrics.

        代理并转发所有 HTTP 方法的请求，支持故障重试、被动健康检测、
        熔断器保护与 Prometheus 延迟/计数指标采集。
        """
        method = request.method
        # 非幂等请求仅在 ConnectError（TCP 连接未建立，请求尚未到达后端）时
        # 允许故障转移；超时或响应读取失败可能已经产生副作用，不重复发送。
        max_retries = 3
        query_params = request.query_params
        start_time = time.perf_counter()

        # 提取原请求 headers，排除 Hop-by-hop 头
        headers = {}
        for k, v in request.headers.items():
            if k.lower() not in EXCLUDE_HEADERS:
                headers[k] = v

        # 补充客户端真实 IP 代理请求头
        client_ip = request.client.host if request.client else "127.0.0.1"
        if "x-forwarded-for" in headers:
            headers["x-forwarded-for"] = f"{headers['x-forwarded-for']}, {client_ip}"
        else:
            headers["x-forwarded-for"] = client_ip
        if "x-real-ip" not in headers:
            headers["x-real-ip"] = client_ip

        # 仅读取一次请求 body，供重试使用
        body = await request.body()
        last_exception: Exception | None = None

        for attempt in range(max_retries):
            node = await balancer.select_node()
            if not node:
                duration = time.perf_counter() - start_time
                GATEWAY_REQUESTS_TOTAL.labels(protocol="http", method=method, status="503").inc()
                GATEWAY_LATENCY.labels(protocol="http").observe(duration)
                logger.error(
                    "No healthy backend nodes available",
                    extra={"path": path, "method": method},
                )
                raise HTTPException(status_code=503, detail="No healthy backend nodes available")

            # 获取或延迟初始化应用级单例 HTTP 客户端
            current_loop = asyncio.get_running_loop()
            client = getattr(request.app.state, "http_client", None)
            cached_loop = getattr(request.app.state, "http_client_loop", None)

            if client is None or cached_loop is not current_loop:
                if client is not None:
                    # 已知限制：跨事件循环重建客户端时，旧 client 的关闭是
                    # fire-and-forget（aclose 调度到当前 loop 异步执行）。
                    # 若旧 loop 上仍有在途请求复用该 client，可能被淘汰中的
                    # 连接掐断；该场景仅在同一 app 实例被多个 loop 交替服务时
                    # 出现（如混合测试/多 loop 部署），生产单 loop 部署不受影响。
                    asyncio.create_task(client.aclose())  # noqa: RUF006

                client = httpx.AsyncClient(
                    timeout=httpx.Timeout(30.0),
                    limits=httpx.Limits(max_keepalive_connections=100, max_connections=500),
                    trust_env=False,
                    verify=backend_tls_verify(),
                )
                request.app.state.http_client = client
                request.app.state.http_client_loop = current_loop

            url = f"{node.http_url}/{path}"
            try:
                async with node.track_connection():
                    resp = await client.request(
                        method=method,
                        url=url,
                        headers=headers,
                        params=query_params,
                        content=body,
                    )

                # 记录成功指标
                duration = time.perf_counter() - start_time
                GATEWAY_REQUESTS_TOTAL.labels(
                    protocol="http", method=method, status=str(resp.status_code)
                ).inc()
                GATEWAY_LATENCY.labels(protocol="http").observe(duration)
                # 后端 5xx 表明节点服务能力异常，计入熔断器失败统计；
                # 4xx 属于客户端请求问题：既不算节点故障，也不算节点恢复，
                # 因此不惩罚节点，也不重置已有的失败统计。
                if resp.status_code >= 500:
                    node.circuit_breaker.record_failure()
                elif resp.status_code < 400:
                    node.circuit_breaker.record_success()

                # 构建并清洗响应 headers
                resp_headers = {}
                for k, v in resp.headers.items():
                    if k.lower() not in RESPONSE_EXCLUDE_HEADERS:
                        resp_headers[k] = v

                return Response(
                    content=resp.content,
                    status_code=resp.status_code,
                    headers=resp_headers,
                )
            except Exception as exc:
                last_exception = exc
                retry_allowed = method in {"GET", "HEAD", "OPTIONS"} or isinstance(exc, httpx.ConnectError)
                node.circuit_breaker.record_failure()
                GATEWAY_RETRIES_TOTAL.labels(protocol="http", reason="connection_error").inc()
                logger.warning(
                    "HTTP proxy attempt failed, retrying",
                    extra={
                        "attempt": attempt + 1,
                        "max_retries": max_retries,
                        "url": url,
                        "error": str(exc),
                        "circuit_breaker": node.circuit_breaker.state,
                    },
                )
                # 被动健康检查更新：立即将该节点置为不健康
                node.is_healthy = False
                node.passive_unhealthy_until = time.monotonic() + 5.0
                if not retry_allowed:
                    break


        # 若重试全部耗尽：不向客户端回传后端异常原文（可能含内网 URL/拓扑信息），
        # 详细原因仅记录在网关内部日志中。
        duration = time.perf_counter() - start_time
        GATEWAY_REQUESTS_TOTAL.labels(protocol="http", method=method, status="502").inc()
        GATEWAY_LATENCY.labels(protocol="http").observe(duration)
        logger.error(
            "HTTP proxy request failed after all retries",
            extra={"path": path, "method": method, "max_retries": max_retries, "last_error": str(last_exception)},
        )
        raise HTTPException(
            status_code=502,
            detail=f"Bad Gateway: all {max_retries} backend retry attempts failed",
        )

    return app

