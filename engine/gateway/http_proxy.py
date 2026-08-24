"""HTTP 反向代理网关模块 (HTTP Reverse-Proxy Gateway).

基于 FastAPI 实现通配路由代理，将 REST 请求透明转发至后端健康节点，
支持故障自适应重试、被动健康检测、熔断器保护、双向 TLS 回源与 Prometheus 指标采集。

主要职责与执行逻辑：
1. **连接池生命周期管理 (Lifespan)**：在应用级维护长连接单例 `httpx.AsyncClient`，并支持跨事件循环漂移感知自愈；
2. **Hop-by-Hop 与压缩头过滤**：按 RFC 7230 剔除逐段传输头，并在响应端剔除 `content-encoding` 杜绝客户端二次解包崩溃；
3. **真实客户端 IP 透传**：提取直连 IP 并规范化追加至 `X-Forwarded-For` 与 `X-Real-IP`；
4. **动态拓扑管理安全防护**：提供 `/v1/gateway/register` 与 `/v1/gateway/deregister` 端点，采用 Fail-Closed 鉴权与 SSRF 阻断；
5. **通配路由透明代理 (/{path:path})**：
   - 步骤 1：单次读取缓存 Request Body；
   - 步骤 2：负载均衡器动态选路；
   - 步骤 3：在途连接追踪 (`track_connection`) 与代理转发；
   - 步骤 4：状态码精细分流（5xx 熔断惩罚、2xx/3xx 熔断复位、4xx 业务透传）；
   - 步骤 5：毫秒级被动故障感知（5s 冷却）与非幂等重试安全中断；
   - 步骤 6：全链路 Prometheus 指标记录与内部错误脱敏。
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

from engine.observability.logging_config import get_logger
from engine.observability.metrics import (
    GATEWAY_LATENCY,
    GATEWAY_REQUESTS_TOTAL,
    GATEWAY_RETRIES_TOTAL,
)

from .balancer import LoadBalancer, backend_tls_verify

logger = get_logger(__name__)


# RFC 7230 规定的逐段传输头 (Hop-by-hop headers)，仅在直连链路有效，代理转发时不应向下传递
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
    """创建并初始化 HTTP 网关 FastAPI 核心应用。

    执行流程：
        1. 配置 FastAPI Lifespan 管理单例 HTTP 客户端连接池；
        2. 注册动态管理端点鉴权钩子 (require_management_auth)；
        3. 挂载动态注册 /v1/gateway/register 与注销 /v1/gateway/deregister 路由；
        4. 挂载通配反向代理路由 /{path:path} 并绑定重试与指标采集逻辑。

    Args:
        balancer: 关联的负载均衡调度器实例。

    Returns:
        FastAPI: 构建完成的网关 FastAPI 应用。
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        # 步骤 1：在应用启动时初始化全局单例 HTTP 客户端，配置超时、连接池上限与回源 CA
        app.state.http_client = httpx.AsyncClient(
            timeout=httpx.Timeout(30.0),
            limits=httpx.Limits(max_keepalive_connections=100, max_connections=500),
            trust_env=False,  # 禁用环境变量代理，防止本地转发流量被拦截
            verify=backend_tls_verify(),  # 回源 TLS 启用时按 CA 校验后端证书
        )
        yield
        # 步骤 2：在应用关闭时优雅释放连接池
        await app.state.http_client.aclose()

    app = FastAPI(title="SecretFlow Local Privacy Agent REST Gateway", lifespan=lifespan)
    gateway_api_key = os.environ.get("GATEWAY_API_KEY", "")

    def require_management_auth(authorization: str | None) -> None:
        """保护拓扑管理端点的鉴权函数（Fail-Closed 默认拒绝策略）。

        执行步骤：
            1. Fail-Closed 检查：若环境变量未配置 ``GATEWAY_API_KEY``，直接抛出 503 禁用接口；
            2. 常量时间比对：使用 ``hmac.compare_digest`` 严格比对 Authorization Bearer Token，
               比对失败时抛出 401 Unauthorized。
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
        """动态注册或热更新工作节点。

        执行步骤：
            1. 校验管理端点鉴权令牌；
            2. 校验 `http_url` 协议合法性（必须以 http:// 或 https:// 开头，杜绝 SSRF）；
            3. 调用 `balancer.add_node` 将节点注入调度池并刷新健康指标；
            4. 记录审计日志并返回 `{"status": "registered"}`。
        """
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
        """动态注销工作节点。

        执行步骤：
            1. 校验管理端点鉴权令牌；
            2. 调用 `balancer.remove_node` 从调度池中移除节点并异步释放底层通道；
            3. 记录审计日志并返回 `{"status": "deregistered"}`。
        """
        require_management_auth(authorization)
        balancer.remove_node(req.http_url, req.grpc_address)
        logger.info(
            "Node deregistered via API",
            extra={"http_url": req.http_url, "grpc_address": req.grpc_address},
        )
        return {"status": "deregistered"}

    @app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"])
    async def proxy_request(path: str, request: Request):
        """通配反向代理核心路由（支持故障重试、被动感知与指标采集）。

        执行步骤：
            1. **准备阶段**：记录请求开始时间戳，提取请求方法与 Query 参数；
            2. **Header 过滤与客户端 IP 注入**：
               - 剥离 Hop-by-Hop 逐段传输头；
               - 提取客户端直连 IP 并规范化追加至 `X-Forwarded-For` 与 `X-Real-IP`；
            3. **Body 单次缓冲**：通过 `await request.body()` 完成单次流式读取，确保重发安全；
            4. **重试与故障转移循环 (最多 3 次尝试)**：
               - 步骤 4.1：调用 `balancer.select_node()` 获取可用健康节点，若无节点可用立即返回 503；
               - 步骤 4.2：检查当前 AsyncIO Event Loop 是否漂移，按需重建单例 `httpx.AsyncClient`；
               - 步骤 4.3：在 `node.track_connection()` 上下文管理器内发起 HTTP 异步请求；
               - 步骤 4.4：请求成功：
                 * 记录 Prometheus QPS 与 Latency 直方图；
                 * 根据 HTTP 状态码反馈熔断器（>=500 记录失败，<400 记录成功，4xx 业务错误不惩罚）；
                 * 清洗响应头（剔除 Hop-by-Hop 与 `content-encoding`），封装 `fastapi.Response` 返回；
               - 步骤 4.5：请求异常（网络连接失败或超时）：
                 * 判定是否允许重试：幂等方法（GET/HEAD/OPTIONS）或 `httpx.ConnectError`（连接未建立，无副作用）；
                 * 记录熔断器失败与 Prometheus `privacy_gateway_retries_total` 计数；
                 * 触发毫秒级被动健康下线（`node.is_healthy = False`, 5 秒冷却退避）；
                 * 若为非幂等且已发送数据的超时异常，立即中断重试循环，防止产生重复扣减副作用；
            5. **重试耗尽兜底**：记录 502 监控指标，对客户端屏蔽内网真实错误详情并抛出 HTTP 502。
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

        # 防大包拒绝服务攻击 (Payload DDoS Protection): 限制最大 64 MiB 请求体
        content_length = request.headers.get("content-length")
        max_body_bytes = 64 * 1024 * 1024  # 64 MiB
        if content_length:
            try:
                if int(content_length) > max_body_bytes:
                    raise HTTPException(
                        status_code=413, detail=f"Payload too large: exceeds {max_body_bytes} bytes"
                    )
            except ValueError:
                pass

        # 仅读取一次请求 body，供重试使用
        body = await request.body()
        if len(body) > max_body_bytes:
            raise HTTPException(
                status_code=413, detail=f"Payload too large: exceeds {max_body_bytes} bytes"
            )
        last_exception: Exception | None = None

        for attempt in range(max_retries):
            # 步骤 4.1: 动态挑选健康后端节点
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

            # 步骤 4.2: 获取或延迟初始化应用级单例 HTTP 客户端（处理 Loop 漂移）
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
                # 步骤 4.3: 追踪连接数并转发请求
                async with node.track_connection():
                    resp = await client.request(
                        method=method,
                        url=url,
                        headers=headers,
                        params=query_params,
                        content=body,
                    )

                # 步骤 4.4: 记录监控指标与熔断反馈
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
                # 步骤 4.5: 异常处理、被动健康感知与重试判定
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
                # 被动健康检查更新：立即将该节点置为不健康并开启 5 秒冷却退避
                node.mark_unhealthy(cooldown_seconds=5.0)
                if not retry_allowed:
                    break

        # 步骤 5: 若重试全部耗尽：不向客户端回传后端异常原文（可能含内网 URL/拓扑信息），
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
