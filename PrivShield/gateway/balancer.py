"""负载均衡与健康检查引擎模块。

定义后端工作节点、负载均衡调度策略、异步健康检查循环及熔断器。

Load-balancing and health-check engine.

Defines backend worker nodes, scheduling strategies, async health-check loop,
and a per-node circuit breaker for fault isolation.

.. note::
    **回源 TLS（可选）**：网关到后端的回源链路默认使用明文
    （``grpc.aio.insecure_channel`` + ``http://`` 健康检查），适用于网关与
    后端同可信内网的部署。若后端启用了 TLS，设置以下环境变量开启 TLS 回源：

    - ``PRIVACY_GATEWAY_BACKEND_TLS_ENABLED=true``：启用 TLS 回源；
    - ``PRIVACY_GATEWAY_BACKEND_TLS_CA``：校验后端证书的 CA 文件路径（必填，
      缺失或未配置时启动/首次请求即报错，fail-fast）；
    - ``PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT`` / ``..._CLIENT_KEY``：
      可选，后端要求 mTLS 时的客户端证书与私钥。

    启用后健康检查与 HTTP 转发按该 CA 校验后端证书（节点注册时使用
    ``https://`` 前缀的 ``http_url``），gRPC 回源切换为 ``secure_channel``。
"""

from __future__ import annotations

import asyncio
import contextlib
import os
import random
import threading
import time

import grpc
import httpx

from PrivShield import privacy_pb2, privacy_pb2_grpc
from PrivShield.observability.logging_config import get_logger
from PrivShield.observability.metrics import (
    GATEWAY_HEALTHY_NODES,
)

logger = get_logger(__name__)

# gRPC 收发消息上限 64 MiB，与后端 grpc_server.serve() 的
# max_receive/max_send_message_length 配置对齐（默认 4 MiB 对大表/图片
# 分类场景极易超限，导致连接被重置）。
GRPC_MAX_MESSAGE_BYTES = 64 * 1024 * 1024

# 网关到后端的 gRPC 通道选项（回源方向）。
GRPC_CHANNEL_OPTIONS = [
    ("grpc.max_receive_message_length", GRPC_MAX_MESSAGE_BYTES),
    ("grpc.max_send_message_length", GRPC_MAX_MESSAGE_BYTES),
]


def backend_tls_enabled() -> bool:
    """是否启用网关→后端的 TLS 回源 / Whether backend-origin TLS is enabled."""
    return os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_ENABLED", "").strip().lower() in (
        "1",
        "true",
        "yes",
    )


def backend_tls_verify() -> "str | bool":
    """httpx ``verify`` 参数：启用回源 TLS 时返回 CA 路径，否则默认校验。

    Raises:
        RuntimeError: 启用了回源 TLS 但未配置 ``PRIVACY_GATEWAY_BACKEND_TLS_CA``。
    """
    if not backend_tls_enabled():
        return True
    ca_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CA", "").strip()
    if not ca_path:
        raise RuntimeError(
            "PRIVACY_GATEWAY_BACKEND_TLS_ENABLED=true 但未配置 "
            "PRIVACY_GATEWAY_BACKEND_TLS_CA（后端证书校验 CA 文件路径）"
        )
    if not os.path.isfile(ca_path):
        raise RuntimeError(f"回源 TLS CA 文件不存在: {ca_path}")
    return ca_path


def backend_channel_credentials() -> "grpc.ChannelCredentials | None":
    """构建回源 gRPC 通道凭据；未启用 TLS 时返回 None（调用方用 insecure_channel）。

    支持可选 mTLS（PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT / _CLIENT_KEY）。

    Raises:
        RuntimeError: TLS 配置缺失或证书文件不存在。
    """
    if not backend_tls_enabled():
        return None
    ca_path = backend_tls_verify()  # 复用校验逻辑（缺失/不存在时抛 RuntimeError）
    with open(ca_path, "rb") as f:
        root_certs = f.read()
    client_cert_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT", "").strip()
    client_key_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_KEY", "").strip()
    private_key = certificate_chain = None
    if client_cert_path or client_key_path:
        if not (client_cert_path and client_key_path):
            raise RuntimeError(
                "回源 mTLS 需同时配置 PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT 与 "
                "PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_KEY"
            )
        for p in (client_cert_path, client_key_path):
            if not os.path.isfile(p):
                raise RuntimeError(f"回源 mTLS 客户端证书/私钥文件不存在: {p}")
        with open(client_key_path, "rb") as f:
            private_key = f.read()
        with open(client_cert_path, "rb") as f:
            certificate_chain = f.read()
    return grpc.ssl_channel_credentials(
        root_certificates=root_certs,
        private_key=private_key,
        certificate_chain=certificate_chain,
    )


# ---------------------------------------------------------------------------
# Circuit Breaker (P2: 熔断器)
# ---------------------------------------------------------------------------


class CircuitBreaker:
    """Per-node circuit breaker with half-open recovery.

    每个后端节点配备独立的熔断器，连续失败达到阈值后打开（拒绝请求），
    经过恢复窗口后进入半开状态允许探测。

    States: closed (normal) → open (rejecting) → half_open (probing).
    """

    def __init__(self, failure_threshold: int = 5, recovery_timeout: float = 30.0):
        """Initialize circuit breaker.

        Args:
            failure_threshold: 连续失败次数阈值，达到后熔断器打开。
            recovery_timeout: 熔断后恢复窗口（秒），过后进入半开状态。
        """
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self._failure_count = 0
        self._state = "closed"
        self._opened_at: float = 0.0
        self._lock = threading.Lock()

    @property
    def state(self) -> str:
        """Return current state, transitioning open → half_open if timeout elapsed."""
        with self._lock:
            if self._state == "open" and (time.monotonic() - self._opened_at) >= self.recovery_timeout:
                self._state = "half_open"
            return self._state

    def record_success(self) -> None:
        """Record a successful call; reset breaker to closed."""
        with self._lock:
            self._failure_count = 0
            self._state = "closed"

    def record_failure(self) -> None:
        """Record a failed call; open breaker if threshold reached."""
        with self._lock:
            self._failure_count += 1
            if self._failure_count >= self.failure_threshold:
                self._state = "open"
                self._opened_at = time.monotonic()

    def allow_request(self) -> bool:
        """Return True if the breaker allows a request through."""
        state = self.state
        return state in ("closed", "half_open")


# ---------------------------------------------------------------------------
# Backend Node
# ---------------------------------------------------------------------------


class BackendNode:
    """后端工作节点。

    维护单个后端实例的地址信息、健康状态、长连接通道、活跃连接数及熔断器。

    Backend worker node maintaining address info, health status, gRPC channel,
    active connection count, and a per-node circuit breaker.
    """

    def __init__(self, http_url: str, grpc_address: str, weight: int = 1):
        """初始化工作节点 / Initialize worker node.

        Args:
            http_url: 后端 HTTP/REST 基准 URL (例如 "http://127.0.0.1:8079")。
            grpc_address: 后端 gRPC 地址 (例如 "127.0.0.1:50051")。
            weight: 权重 (预留用于加权轮询/随机)。
        """
        self.http_url = http_url.rstrip("/")
        self.grpc_address = grpc_address
        self.weight = max(1, weight)
        self.current_weight = 0  # 动态平滑加权轮询权重
        self.is_healthy = True
        self.passive_unhealthy_until = 0.0
        self.active_connections = 0
        self.circuit_breaker = CircuitBreaker()
        self._grpc_channel: grpc.aio.Channel | None = None
        self._grpc_stub: privacy_pb2_grpc.PrivacyServiceStub | None = None
        self._connection_lock = asyncio.Lock()
        # 保护 grpc_stub 懒初始化：并发首次访问时避免重复创建 channel
        # （多创建的 channel 无引用会被泄漏）。
        self._stub_lock = threading.Lock()

    @property
    def grpc_stub(self) -> privacy_pb2_grpc.PrivacyServiceStub:
        """延迟初始化并获取 gRPC Stub / Lazily initialize gRPC stub."""
        if self._grpc_stub is None:
            # Double-checked locking：通道创建幂等但非原子，加锁防止并发
            # 首次访问时创建出双份 channel 而泄漏其一。
            with self._stub_lock:
                if self._grpc_stub is None:
                    credentials = backend_channel_credentials()
                    if credentials is not None:
                        # TLS 回源：按 PRIVACY_GATEWAY_BACKEND_TLS_CA 校验后端证书
                        self._grpc_channel = grpc.aio.secure_channel(
                            self.grpc_address, credentials, options=GRPC_CHANNEL_OPTIONS
                        )
                    else:
                        self._grpc_channel = grpc.aio.insecure_channel(
                            self.grpc_address, options=GRPC_CHANNEL_OPTIONS
                        )
                    self._grpc_stub = privacy_pb2_grpc.PrivacyServiceStub(self._grpc_channel)
        return self._grpc_stub

    @contextlib.asynccontextmanager
    async def track_connection(self):
        """自动追踪活跃连接数上下文管理器（最小连接数调度使用）。"""
        async with self._connection_lock:
            self.active_connections += 1
        try:
            yield self
        finally:
            async with self._connection_lock:
                self.active_connections = max(0, self.active_connections - 1)

    async def close(self) -> None:
        """关闭 gRPC 连接通道 / Close gRPC channel."""
        if self._grpc_channel is not None:
            await self._grpc_channel.close()
            self._grpc_channel = None
            self._grpc_stub = None


# ---------------------------------------------------------------------------
# Load Balancer
# ---------------------------------------------------------------------------


class LoadBalancer:
    """负载均衡调度器。

    支持对健康后端的平滑加权轮询、加权随机、最小连接数等分发策略。

    Load-balancer scheduler supporting smooth weighted round-robin,
    weighted random, and least-connections strategies over healthy backend nodes.
    """

    def __init__(self, strategy: str = "round_robin"):
        """初始化负载均衡器 / Initialize load balancer.

        Args:
            strategy: 负载均衡策略 ("round_robin", "weighted_round_robin", "random", "weighted_random", "least_connections")。
        """
        self.strategy = strategy.lower()
        self.nodes: list[BackendNode] = []
        self.rr_index = 0
        self.modify_lock = threading.Lock()
        self._selection_lock = asyncio.Lock()

    def add_node(self, http_url: str, grpc_address: str, weight: int = 1) -> None:
        """添加工作节点到地址池 / Add worker node to pool (thread-safe, dedup)."""
        with self.modify_lock:
            clean_url = http_url.rstrip("/")
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    node.is_healthy = True
                    node.passive_unhealthy_until = 0.0
                    node.weight = max(1, weight)
                    node.active_connections = 0
                    node.circuit_breaker.record_success()
                    logger.info(
                        "Updated existing backend node",
                        extra={"http_url": http_url, "grpc_address": grpc_address},
                    )
                    # 节点被重新标记为健康，同步刷新健康节点数指标
                    self._update_healthy_gauge()
                    return

            node = BackendNode(http_url, grpc_address, weight)
            self.nodes.append(node)
            logger.info(
                "Added backend node",
                extra={"http_url": http_url, "grpc_address": grpc_address},
            )
            self._update_healthy_gauge()

    def remove_node(self, http_url: str, grpc_address: str) -> None:
        """安全地从节点池中移除工作节点 / Safely remove a node from pool."""
        with self.modify_lock:
            clean_url = http_url.rstrip("/")
            new_nodes = []
            removed = False
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    # 尽力关闭该节点的 gRPC 通道：
                    # - 存在运行中的事件循环（如 REST 注销路由）时调度异步关闭；
                    # - 同步上下文（脚本/测试）中没有运行中的循环，
                    #   create_task 会抛 RuntimeError，此时用独立循环完成关闭。
                    try:
                        loop = asyncio.get_running_loop()
                    except RuntimeError:
                        loop = None
                    if loop is not None:
                        loop.create_task(node.close())  # noqa: RUF006
                    else:
                        with contextlib.suppress(Exception):
                            asyncio.run(node.close())
                    removed = True
                else:
                    new_nodes.append(node)
            self.nodes = new_nodes
            if removed:
                logger.info(
                    "Removed backend node",
                    extra={"http_url": http_url, "grpc_address": grpc_address},
                )
                self._update_healthy_gauge()

    def get_healthy_nodes(self) -> list[BackendNode]:
        """获取当前健康且熔断器允许的节点列表 / Get healthy + circuit-closed nodes."""
        return [
            node
            for node in self.nodes
            if node.is_healthy
            and time.monotonic() >= node.passive_unhealthy_until
            and node.circuit_breaker.allow_request()
        ]

    async def select_node(self) -> BackendNode | None:
        """按策略选择一个可用后端节点 / Select a backend node by strategy.

        Returns:
            若有可用节点返回 BackendNode，否则返回 None。
        """
        async with self._selection_lock:
            healthy = self.get_healthy_nodes()
            if not healthy:
                return None

            if self.strategy in ("random", "weighted_random"):
                weights = [n.weight for n in healthy]
                return random.choices(healthy, weights=weights, k=1)[0]

            elif self.strategy == "least_connections":
                return min(healthy, key=lambda n: n.active_connections)

            elif self.strategy == "weighted_round_robin":
                # Smooth Weighted Round-Robin (Nginx Algorithm)
                total_weight = sum(n.weight for n in healthy)
                best_node: BackendNode | None = None
                for node in healthy:
                    node.current_weight += node.weight
                    if best_node is None or node.current_weight > best_node.current_weight:
                        best_node = node
                if best_node is not None:
                    best_node.current_weight -= total_weight
                return best_node

            else:  # round_robin
                node = healthy[self.rr_index % len(healthy)]
                self.rr_index = (self.rr_index + 1) % len(healthy)
                return node

    async def close_all(self) -> None:
        """关闭所有后端的 gRPC 通道 / Close all backend gRPC channels."""
        for node in self.nodes:
            await node.close()

    def _update_healthy_gauge(self) -> None:
        """Update Prometheus healthy-nodes gauge."""
        count = len(self.get_healthy_nodes())
        GATEWAY_HEALTHY_NODES.set(count)


# ---------------------------------------------------------------------------
# Health Check Loop
# ---------------------------------------------------------------------------


async def health_check_loop(balancer: LoadBalancer, interval: float = 5.0) -> None:
    """异步健康检查后台任务 / Async background health-check loop.

    定时向所有后端节点发送 HTTP 与 gRPC 健康请求，更新节点在线状态与熔断器。

    Periodically probes all backend nodes via HTTP and gRPC health endpoints,
    updating node health status and circuit breaker state.

    Args:
        balancer: 关联的负载均衡实例。
        interval: 检测间隔时间（秒）。
    """
    logger.info("Starting background health check loop", extra={"interval_seconds": interval})
    # 回源 TLS 启用时按配置的 CA 校验后端证书（backend_tls_verify 在配置缺失时抛错）
    async with httpx.AsyncClient(verify=backend_tls_verify()) as client:
        while True:
            for node in balancer.nodes:
                # 1. 检查 REST (HTTP) 服务
                http_ok = False
                try:
                    res = await client.get(f"{node.http_url}/health", timeout=2.0)
                    if res.status_code == 200:
                        data = res.json()
                        if data.get("status") == "ok":
                            http_ok = True
                except Exception as e:
                    logger.debug(
                        "HTTP health check failed",
                        extra={"node": node.http_url, "error": str(e)},
                    )

                # 2. 检查 gRPC 服务
                grpc_ok = False
                try:
                    req = privacy_pb2.HealthRequest()
                    res = await node.grpc_stub.Health(req, timeout=2.0)
                    if res.status == "ok":
                        grpc_ok = True
                except Exception as e:
                    logger.debug(
                        "gRPC health check failed",
                        extra={"node": node.grpc_address, "error": str(e)},
                    )

                # 3. 状态决策与更替
                was_healthy = node.is_healthy
                passive_cooldown = time.monotonic() < node.passive_unhealthy_until
                node.is_healthy = http_ok and grpc_ok and not passive_cooldown

                # Update circuit breaker based on health result
                if node.is_healthy:
                    node.circuit_breaker.record_success()
                else:
                    node.circuit_breaker.record_failure()

                if was_healthy != node.is_healthy:
                    status_str = "healthy" if node.is_healthy else "unhealthy"
                    log_func = logger.info if node.is_healthy else logger.warning
                    log_func(
                        "Node status changed",
                        extra={
                            "node": node.grpc_address,
                            "status": status_str,
                            "http": "UP" if http_ok else "DOWN",
                            "grpc": "UP" if grpc_ok else "DOWN",
                            "circuit_breaker": node.circuit_breaker.state,
                        },
                    )

            # Update gauge after each sweep
            balancer._update_healthy_gauge()
            await asyncio.sleep(interval)

