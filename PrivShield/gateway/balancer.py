"""负载均衡与健康检查引擎模块 (Load Balancer & Health Check Engine).

本模块定义后端工作节点 (BackendNode)、独立熔断器 (CircuitBreaker)、负载均衡调度策略
(LoadBalancer) 以及异步主动健康检查循环 (health_check_loop)。

主要职责与执行逻辑：
1. **节点模型封装 (BackendNode)**：维护单节点的 REST 与 gRPC 地址、权重、动态平滑权重、
   主动/被动健康状态、在途连接数及独立熔断器，并提供延迟线程安全 Channel 懒加载；
2. **熔断器保护 (CircuitBreaker)**：提供基于连续失败阈值与时间窗口的三态 (Closed/Open/Half-Open) 状态机；
3. **负载均衡调度 (LoadBalancer)**：
   - 轮询 (Round-Robin)：简单游标递增取模调度；
   - 平滑加权轮询 (Smooth Weighted Round-Robin)：Nginx 算法动态累加与削减权重，避免大权重瞬时集中；
   - 最小连接数 (Least Connections)：实时统计在途活跃请求，将高耗时任务导向最空闲实例；
   - 随机与加权随机 (Random / Weighted Random)；
4. **高可用主动探针 (health_check_loop)**：周期性并发发起 HTTP /health 与 gRPC Health 探针，强一致判定在线状态；
5. **回源 TLS 体系 (Backend Origin TLS)**：按需构建基于 CA 校验的 Secure Channel 与 HTTPS 客户端，保障东西向流量安全。

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
    """检查是否启用网关→后端的东西向 TLS 回源。

    执行逻辑：
        读取环境变量 ``PRIVACY_GATEWAY_BACKEND_TLS_ENABLED``，若值为 "1"、"true"、"yes"（忽略大小写）
        则判定为启用回源 TLS，否则默认为明文回源。

    Returns:
        bool: 是否启用 TLS 回源。
    """
    return os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_ENABLED", "").strip().lower() in (
        "1",
        "true",
        "yes",
    )


def backend_tls_verify() -> "str | bool":
    """获取 HTTP 客户端 (httpx) 的 TLS 校验参数 (CA 路径或布尔值)。

    执行逻辑：
        1. 步骤 1：若未启用回源 TLS，直接返回 True（标准系统默认证书校验）；
        2. 步骤 2：若已启用回源 TLS，提取 ``PRIVACY_GATEWAY_BACKEND_TLS_CA`` 环境变量；
        3. 步骤 3 (Fail-Fast 校验)：若未配置 CA 路径或指定的文件不存在，立即抛出 RuntimeError，
           杜绝配置错误时静默降级为明文。

    Returns:
        str | bool: CA 证书绝对路径或布尔校验开关。

    Raises:
        RuntimeError: 启用了回源 TLS 但未配置 CA 路径或文件不存在。
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
    """构建用于连接后端的 gRPC 通道安全凭据 (ChannelCredentials)。

    执行逻辑：
        1. 步骤 1：若未启用回源 TLS，返回 None（上层调用方据此使用 ``insecure_channel``）；
        2. 步骤 2：复用 ``backend_tls_verify()`` 读取并校验根 CA 证书二进制内容；
        3. 步骤 3：检测是否配置了回源客户端证书与私钥 (mTLS)：
           - 若配置，必须证书与私钥成对提供，并校验本地文件真实存在；
           - 读取客户端证书链与私钥二进制字节流；
        4. 步骤 4：调用 ``grpc.ssl_channel_credentials`` 组装 SSL 通道凭据并返回。

    Returns:
        grpc.ChannelCredentials | None: 组装好的 gRPC 通道凭据，未开启 TLS 时为 None。

    Raises:
        RuntimeError: TLS 配置缺失、证书/私钥不成对或文件不存在。
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
# Circuit Breaker (节点独立熔断器)
# ---------------------------------------------------------------------------


class CircuitBreaker:
    """单节点独立熔断器（支持半开试探与自愈恢复）。

    设计理念：
        每个后端节点配备独立的熔断器实例，隔离单节点的连续异常崩溃。
        当某节点连续失败达到阈值后打开熔断器（拒绝分配新请求），
        经过冷却窗口期后自动进入半开状态，允许单个探测请求验证其健康度。

    状态流转模型：
        - Closed (闭合/正常)：请求正常通行，失败计数累计；
        - Open (开启/熔断)：达到连续失败上限，彻底阻断请求（持续 recovery_timeout 秒）；
        - Half-Open (半开/探测)：冷却期过后放行少量请求，若成功则重置为 Closed，若失败则重回 Open。
    """

    def __init__(self, failure_threshold: int = 5, recovery_timeout: float = 30.0):
        """初始化熔断器参数。

        Args:
            failure_threshold: 触发熔断的连续失败次数阈值（默认 5 次）。
            recovery_timeout: 熔断冷却持续时间窗口（秒，默认 30.0 秒）。
        """
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self._failure_count = 0
        self._state = "closed"
        self._opened_at: float = 0.0
        self._lock = threading.Lock()

    @property
    def state(self) -> str:
        """获取当前熔断状态（在读取时执行惰性时间窗口校验与状态迁移）。

        执行步骤：
            1. 加锁保护内部状态；
            2. 若当前为 "open" 且单调时钟差值已超过 ``recovery_timeout``，自动迁移为 "half_open"；
            3. 返回最新的状态字符串 ("closed" | "open" | "half_open")。
        """
        with self._lock:
            if self._state == "open" and (time.monotonic() - self._opened_at) >= self.recovery_timeout:
                self._state = "half_open"
            return self._state

    def record_success(self) -> None:
        """记录一次成功的请求或通过的主动探针。

        执行步骤：
            加锁并将失败计数清零 (`_failure_count = 0`)，强置状态为 "closed"（完全复位）。
        """
        with self._lock:
            self._failure_count = 0
            self._state = "closed"

    def record_failure(self) -> None:
        """记录一次失败的请求（5xx 或网络连接中断）。

        执行步骤：
            1. 加锁并将连续失败计数递增 1；
            2. 若失败次数达到或超过 ``failure_threshold``，将状态置为 "open"，并记录当前单调时间戳。
        """
        with self._lock:
            self._failure_count += 1
            if self._failure_count >= self.failure_threshold:
                self._state = "open"
                self._opened_at = time.monotonic()

    def allow_request(self) -> bool:
        """判断熔断器当前是否允许请求通过。

        Returns:
            bool: 处于 "closed" 或 "half_open" 状态时返回 True，处于 "open" 熔断时返回 False。
        """
        state = self.state
        return state in ("closed", "half_open")


# ---------------------------------------------------------------------------
# Backend Node (后端工作节点模型)
# ---------------------------------------------------------------------------


class BackendNode:
    """后端工作节点数据模型与链路封装。

    维护单个后端计算实例的网络坐标、健康标记、在途活跃连接数、
    平滑加权轮询动态权重、绑定熔断器以及延迟初始化的 gRPC 通道。
    """

    def __init__(self, http_url: str, grpc_address: str, weight: int = 1):
        """初始化后端工作节点。

        Args:
            http_url: 后端 HTTP/REST 基准 URL (例如 "http://127.0.0.1:8079" 或 "https://agent:8079")。
            grpc_address: 后端 gRPC 地址 (例如 "127.0.0.1:50051")。
            weight: 静态配置权重（下限保底为 1）。
        """
        self.http_url = http_url.rstrip("/")
        self.grpc_address = grpc_address
        self.weight = max(1, weight)
        self.current_weight = 0  # 动态平滑加权轮询权重（SWRR 算法运行时更新）
        self.is_healthy = True
        self.passive_unhealthy_until = 0.0  # 被动故障感知冷却截止时间 (time.monotonic)
        self.active_connections = 0  # 当前在途正在处理的并发请求计数
        self.circuit_breaker = CircuitBreaker()  # 专属独立熔断器
        self._grpc_channel: grpc.aio.Channel | None = None
        self._grpc_stub: privacy_pb2_grpc.PrivacyServiceStub | None = None
        self._connection_lock = asyncio.Lock()  # 保护 active_connections 原子操作
        self._stub_lock = threading.Lock()  # 保护 grpc_stub 延迟初始化的双重检查锁

    @property
    def grpc_stub(self) -> privacy_pb2_grpc.PrivacyServiceStub:
        """延迟初始化并获取长连接 gRPC Stub 存根。

        执行步骤：
            1. 快速路径：若 `_grpc_stub` 已创建，直接无锁返回；
            2. 慢速路径 (Double-Checked Locking)：
               - 加锁 `_stub_lock`，再次检查 `_grpc_stub` 是否为空；
               - 调用 `backend_channel_credentials()` 获取回源凭据；
               - 若凭据存在，通过 `grpc.aio.secure_channel` 构建加密通道；
               - 若无凭据，通过 `grpc.aio.insecure_channel` 构建明文通道；
               - 注入 64 MiB 缓冲区选项 `GRPC_CHANNEL_OPTIONS`；
               - 实例化 `PrivacyServiceStub` 并赋值给 `_grpc_stub`。

        Returns:
            privacy_pb2_grpc.PrivacyServiceStub: 可复用的 gRPC 客户端存根。
        """
        if self._grpc_stub is None:
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
        """自动追踪在途活跃连接数的异步上下文管理器（供最小连接数算法精确计量）。

        执行步骤：
            1. 进入上下文：加锁递增 `self.active_connections += 1`；
            2. yield 出让执行权给业务请求；
            3. 退出上下文：在 finally 块中加锁递减 `self.active_connections`（下限保底为 0）。
        """
        async with self._connection_lock:
            self.active_connections += 1
        try:
            yield self
        finally:
            async with self._connection_lock:
                self.active_connections = max(0, self.active_connections - 1)

    async def close(self) -> None:
        """优雅关闭并释放该节点的底层 gRPC 通道。

        执行步骤：
            若已创建 `_grpc_channel`，调用 `await self._grpc_channel.close()` 释放底层 TCP 套接字，
            并将引用置为 None 以便垃圾回收。
        """
        if self._grpc_channel is not None:
            await self._grpc_channel.close()
            self._grpc_channel = None
            self._grpc_stub = None


# ---------------------------------------------------------------------------
# Load Balancer (负载均衡调度引擎)
# ---------------------------------------------------------------------------


class LoadBalancer:
    """负载均衡调度器。

    支持对健康后端节点池执行平滑加权轮询 (SWRR)、加权随机、最小连接数 (Least Connections)
    及普通轮询 (Round-Robin) 等多策略分发。
    """

    def __init__(self, strategy: str = "round_robin"):
        """初始化负载均衡调度器。

        Args:
            strategy: 调度策略名称 ("round_robin", "weighted_round_robin", "random", "weighted_random", "least_connections")。
        """
        self.strategy = strategy.lower()
        self.nodes: list[BackendNode] = []
        self.rr_index = 0
        self.modify_lock = threading.Lock()  # 保护节点池增删操作的线程锁
        self._selection_lock = asyncio.Lock()  # 保护节点选择调度过程的协程锁

    def add_node(self, http_url: str, grpc_address: str, weight: int = 1) -> None:
        """向节点池添加新节点或就地更新既有节点 (线程安全、自动去重与状态复位)。

        执行步骤：
            1. 格式正规化：去除 `http_url` 末尾斜杠；
            2. 幂等去重与原地更新：
               - 遍历节点池，若找到相同 `(http_url, grpc_address)` 的节点；
               - 立即将其标记为健康（`is_healthy = True`），清零被动冷却与活跃连接；
               - 调用熔断器 `record_success()` 重置熔断状态；
               - 更新权重配置并记录 INFO 日志，随后刷新 Prometheus 指标并直接返回；
            3. 新增节点：若不存在，实例化 `BackendNode` 并追加至列表；
            4. 触发 `_update_healthy_gauge()` 刷新监控指标。
        """
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
        """从节点池安全注销工作节点并异步关闭其 gRPC 通道。

        执行步骤：
            1. 格式正规化并过滤出目标节点；
            2. 双环境优雅销毁底层连接：
               - 检测是否存在运行中的 AsyncIO 事件循环（如在 REST 注销端点中）；
               - 若存在，通过 `loop.create_task(node.close())` 异步排空关闭通道；
               - 若处于同步环境（如测试销毁），降级使用 `asyncio.run(node.close())`；
            3. 更新节点池列表，记录日志并刷新 Prometheus 健康节点指标。
        """
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
        """获取当前处于健康可用状态且未被熔断器阻断的节点列表。

        判定准则（必须同时满足 3 项）：
            1. `node.is_healthy is True`（主动探针检查通过）；
            2. `time.monotonic() >= node.passive_unhealthy_until`（被动故障 5 秒冷却已过）；
            3. `node.circuit_breaker.allow_request() is True`（熔断器允许请求通过）。

        Returns:
            list[BackendNode]: 可路由的健康节点列表。
        """
        return [
            node
            for node in self.nodes
            if node.is_healthy
            and time.monotonic() >= node.passive_unhealthy_until
            and node.circuit_breaker.allow_request()
        ]

    async def select_node(self) -> BackendNode | None:
        """根据配置的负载均衡算法从健康节点池中挑选一个目标节点。

        执行步骤：
            1. 加锁 `_selection_lock`，防止多协程并发调度产生竞争；
            2. 调用 `get_healthy_nodes()` 过滤可用节点，若列表为空立即返回 None；
            3. 执行算法分支：
               - **Random / Weighted Random**：调用 `random.choices` 按静态权重带权抽样；
               - **Least Connections**：寻找 `active_connections` 最小的节点；
               - **Weighted Round-Robin (SWRR 平滑加权轮询)**：
                 a. 遍历每个健康节点，累加其静态权重：`node.current_weight += node.weight`，并计算 `total_weight`；
                 b. 选取 `current_weight` 最大的节点作为 `best_node`；
                 c. 将该节点的动态权重扣除总权重：`best_node.current_weight -= total_weight`；
                 d. 返回 `best_node`；
               - **Round-Robin (普通轮询)**：
                 取 `healthy[self.rr_index % len(healthy)]`，并将游标递增 1；
            4. 返回选中的节点对象。

        Returns:
            BackendNode | None: 选中的后端节点，若无可路由节点返回 None。
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
        """优雅关闭节点池中所有后端实例的 gRPC 通道。"""
        for node in self.nodes:
            await node.close()

    def _update_healthy_gauge(self) -> None:
        """同步更新 Prometheus 健康可用节点数 Gauge 指标。"""
        count = len(self.get_healthy_nodes())
        GATEWAY_HEALTHY_NODES.set(count)


# ---------------------------------------------------------------------------
# Active Health Check Loop (主动健康探针后台协程)
# ---------------------------------------------------------------------------


async def health_check_loop(balancer: LoadBalancer, interval: float = 5.0) -> None:
    """双协议主动健康检查后台守护任务。

    执行流程（周期性无限循环）：
        1. 步骤 1：初始化专用 `httpx.AsyncClient`（配置回源 CA 校验）；
        2. 步骤 2：遍历负载均衡器中注册的所有节点：
           - **HTTP 探针**：异步请求 `GET {http_url}/health`（2.0s 超时），预期状态码 200 且 JSON `status == 'ok'`；
           - **gRPC 探针**：调用 `await node.grpc_stub.Health(HealthRequest(), timeout=2.0)`，预期响应 `status == 'ok'`；
        3. 步骤 3：综合状态决策：
           - 只有当 HTTP 和 gRPC 均通过，且当前单调时间已超出被动冷却期时，节点才判定为健康（`is_healthy = True`）；
           - 否则判定为不健康（`is_healthy = False`）；
        4. 步骤 4：联动更新节点熔断器（健康则调用 `record_success()`，不健康则调用 `record_failure()`）；
        5. 步骤 5：状态变迁日志（当节点在线状态发生改变时输出告警或通知日志）；
        6. 步骤 6：刷新 Prometheus `privacy_gateway_healthy_nodes` 指标；
        7. 步骤 7：等待指定的检测间隔时间（`await asyncio.sleep(interval)`）后进入下一轮巡检。

    Args:
        balancer: 关联的负载均衡调度器实例。
        interval: 巡检周期时间间隔（秒，默认 5.0 秒）。
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
