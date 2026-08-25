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
   - 两选择随机算法 (Power of Two Choices, P2C)：随机选取两个健康节点并路由至负载更优者，防止羊群效应；
   - 随机与加权随机 (Random / Weighted Random)；
4. **高可用主动探针 (health_check_loop)**：周期性顺序发起 HTTP /health 与 gRPC Health 探针，强一致判定在线状态；
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

# 允许在类型注解中使用尚未定义的类名，并延迟解析注解。
from __future__ import annotations

# 提供异步锁、线程转移和异步上下文管理所需的基础能力。
import asyncio
# 提供 `asynccontextmanager` 装饰器，用于追踪请求生命周期。
import contextlib
# 读取网关回源 TLS 和其他运行时环境变量。
import os
# 为随机负载均衡和 P2C 算法提供随机函数。
import random
# 保护跨线程访问的节点状态和节点池。
import threading
# 提供单调时钟，避免系统时间回拨影响超时判断。
import time

# 使用 gRPC 异步客户端连接后端服务。
import grpc
# 使用 HTTP 异步客户端执行 REST 健康探针。
import httpx

# 导入健康检查请求消息定义。
from engine import privacy_pb2, privacy_pb2_grpc
# 使用项目统一日志配置，保持结构化日志格式一致。
from engine.observability.logging_config import get_logger
# 导入网关健康、熔断和管理状态指标。
from engine.observability.metrics import (
    GATEWAY_CIRCUIT_BREAKER_STATE,
    GATEWAY_HEALTHY_NODES,
    GATEWAY_NODE_ADMIN_STATE,
)

# 为当前模块创建带模块名的日志记录器。
logger = get_logger(__name__)

# gRPC 收发消息上限 64 MiB，与后端 grpc_server.serve() 的
# max_receive/max_send_message_length 配置对齐（默认 4 MiB 对大表/图片
# 分类场景极易超限，导致连接被重置）。
# 将 64 MiB 转换为字节，供 gRPC 收发大小选项使用。
GRPC_MAX_MESSAGE_BYTES = 64 * 1024 * 1024

# 网关到后端的 gRPC 通道选项（回源方向）。
# 以 gRPC 要求的键值对列表保存客户端通道配置。
GRPC_CHANNEL_OPTIONS = [
    # 限制后端返回消息大小，支持大表和图片类响应。
    ("grpc.max_receive_message_length", GRPC_MAX_MESSAGE_BYTES),
    # 限制发送给后端的请求大小，与接收限制保持一致。
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
    # 读取配置并统一去除空白、转换小写后再比较允许的真值。
    return os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_ENABLED", "").strip().lower() in (
        # 支持数字形式的启用值。
        "1",
        # 支持常见文本形式的启用值。
        "true",
        # 兼容 yes 形式的启用值。
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
    # 未启用回源 TLS 时，让 httpx 使用其默认的系统证书校验行为。
    if not backend_tls_enabled():
        return True
    # TLS 模式必须明确提供 CA 文件，不能静默退回明文连接。
    ca_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CA", "").strip()
    # 缺失 CA 路径表示配置不完整，立即失败以避免错误部署。
    if not ca_path:
        raise RuntimeError(
            "PRIVACY_GATEWAY_BACKEND_TLS_ENABLED=true 但未配置 "
            "PRIVACY_GATEWAY_BACKEND_TLS_CA（后端证书校验 CA 文件路径）"
        )
    # 校验 CA 路径确实指向一个文件，而不是目录或不存在的路径。
    if not os.path.isfile(ca_path):
        raise RuntimeError(f"回源 TLS CA 文件不存在: {ca_path}")
    # 将已验证的路径交给 httpx 作为自定义信任根。
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
    # 明文回源无需创建 TLS 凭据，由调用方选择 insecure_channel。
    if not backend_tls_enabled():
        return None
    # 复用 HTTP TLS 校验逻辑，同时保证 CA 文件存在。
    ca_path = backend_tls_verify()  # 复用校验逻辑（缺失/不存在时抛 RuntimeError）
    # 以二进制读取根 CA，满足 gRPC SSL API 的参数要求。
    with open(ca_path, "rb") as f:
        root_certs = f.read()
    # 读取可选的 mTLS 客户端证书和私钥路径。
    client_cert_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT", "").strip()
    client_key_path = os.environ.get("PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_KEY", "").strip()
    # 默认不携带客户端证书；只有同时配置两项时才启用 mTLS。
    private_key = certificate_chain = None
    # 任意一项存在都要求另一项同时存在，避免半配置状态。
    if client_cert_path or client_key_path:
        # 检查证书和私钥是否成对提供。
        if not (client_cert_path and client_key_path):
            raise RuntimeError(
                "回源 mTLS 需同时配置 PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_CERT 与 "
                "PRIVACY_GATEWAY_BACKEND_TLS_CLIENT_KEY"
            )
        # 分别验证客户端证书和私钥文件存在。
        for p in (client_cert_path, client_key_path):
            if not os.path.isfile(p):
                raise RuntimeError(f"回源 mTLS 客户端证书/私钥文件不存在: {p}")
        # 读取客户端私钥字节内容。
        with open(client_key_path, "rb") as f:
            private_key = f.read()
        # 读取客户端证书链字节内容。
        with open(client_cert_path, "rb") as f:
            certificate_chain = f.read()
    # 根据根 CA 及可选客户端身份材料构造 gRPC TLS 凭据。
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
        # 保存连续失败达到多少次后打开熔断器。
        self.failure_threshold = failure_threshold
        # 保存打开状态持续多久后允许恢复探测。
        self.recovery_timeout = recovery_timeout
        # 初始化连续失败计数。
        self._failure_count = 0
        # 新建熔断器默认处于正常闭合状态。
        self._state = "closed"
        # 尚未打开过熔断器，因此没有打开时间。
        self._opened_at: float = 0.0
        # 半开状态的唯一恢复探测当前未被占用。
        self._half_open_probe_in_flight = False
        # 使用线程锁保护熔断器的全部可变字段。
        self._lock = threading.Lock()

    @property
    def state(self) -> str:
        """获取当前熔断状态（在读取时执行惰性时间窗口校验与状态迁移）。

        执行步骤：
            1. 加锁保护内部状态；
            2. 若当前为 "open" 且单调时钟差值已超过 ``recovery_timeout``，自动迁移为 "half_open"；
            3. 返回最新的状态字符串 ("closed" | "open" | "half_open")。
        """
        # 读取和可能的状态迁移必须原子完成。
        with self._lock:
            # 冷却时间到达后，将 open 惰性迁移为 half_open。
            if self._state == "open" and (time.monotonic() - self._opened_at) >= self.recovery_timeout:
                self._state = "half_open"
            # 返回迁移后的最新熔断状态。
            return self._state

    def record_success(self) -> None:
        """记录一次成功的请求或通过的主动探针。

        执行步骤：
            加锁并将失败计数清零 (`_failure_count = 0`)，强置状态为 "closed"（完全复位）。
        """
        # 串行化成功反馈，防止与失败反馈同时修改状态。
        with self._lock:
            # 成功意味着连续失败链路被打断。
            self._failure_count = 0
            # 无论此前是否半开，都恢复为正常闭合状态。
            self._state = "closed"
            # 释放半开探测占用，让后续请求正常通过。
            self._half_open_probe_in_flight = False

    def record_failure(self) -> None:
        """记录一次失败的请求（5xx 或网络连接中断）。

        执行步骤：
            1. 加锁并将连续失败计数递增 1；
            2. 若失败次数达到或超过 ``failure_threshold``，将状态置为 "open"，并记录当前单调时间戳。
        """
        # 串行化失败反馈和熔断阈值判断。
        with self._lock:
            # 将本次失败加入连续失败计数。
            self._failure_count += 1
            # 达到阈值后打开熔断器并记录单调时间。
            if self._failure_count >= self.failure_threshold:
                self._state = "open"
                self._opened_at = time.monotonic()
                self._half_open_probe_in_flight = False

    def is_available(self) -> bool:
        """Return whether the breaker can be considered during node selection.

        A half-open breaker remains available only while its single recovery probe
        slot is unclaimed. This method does not reserve the slot.
        """
        # 读取可用性时锁住状态，避免看到部分更新。
        with self._lock:
            # 到达恢复窗口后先完成 open 到 half_open 的惰性迁移。
            if self._state == "open" and (time.monotonic() - self._opened_at) >= self.recovery_timeout:
                self._state = "half_open"
            # 闭合状态可用；半开状态仅在探测槽尚未占用时可被候选过滤。
            return self._state == "closed" or (
                self._state == "half_open" and not self._half_open_probe_in_flight
            )

    def allow_request(self) -> bool:
        """Atomically reserve permission for a request to pass the breaker.

        Returns:
            bool: Closed breakers allow requests. Half-open breakers allow exactly
            one in-flight recovery probe; concurrent requests are rejected until
            that probe records success or failure.
        """
        # 锁住检查和占用动作，保证半开状态只放行一个探测请求。
        with self._lock:
            # 冷却结束时允许本次调用把状态推进到半开。
            if self._state == "open" and (time.monotonic() - self._opened_at) >= self.recovery_timeout:
                self._state = "half_open"
            # 闭合状态不需要额外占用探测槽。
            if self._state == "closed":
                return True
            # 半开状态且没有其他探测时，原子占用唯一探测槽。
            if self._state == "half_open" and not self._half_open_probe_in_flight:
                self._half_open_probe_in_flight = True
                return True
            # 打开状态冷却未结束，或半开探测已被占用，因此拒绝请求。
            return False


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
        # 统一去掉 REST 基地址末尾斜杠，避免拼接健康路径时出现双斜杠。
        self.http_url = http_url.rstrip("/")
        # 保存后端 gRPC 地址，供 Stub 建立异步通道。
        self.grpc_address = grpc_address
        # 将权重限制为至少 1，防止调度算法出现零权重除法或永不选中节点。
        self.weight = max(1, weight)
        # Smooth Weighted Round-Robin 使用的运行时累计权重。
        self.current_weight = 0  # 动态平滑加权轮询权重（SWRR 算法运行时更新）
        # 健康状态与在途连接数由统一的线程锁保护，避免并发调度与被动下线之间的
        # 竞态读取（如 select_node 读取 active_connections 时 track_connection 正在
        # 修改同一计数）。
        # 用线程锁保护健康状态、被动冷却时间和活跃连接数。
        self._state_lock = threading.Lock()
        # 新注册节点初始视为健康，等待首次主动探针校验。
        self._is_healthy = True
        self._passive_unhealthy_until = 0.0  # 被动故障感知冷却截止时间 (time.monotonic)
        self._active_connections = 0  # 当前在途正在处理的并发请求计数
        # 每个节点拥有独立熔断器，避免单节点故障影响整个节点池。
        self.circuit_breaker = CircuitBreaker()  # 专属独立熔断器
        # 延迟创建 gRPC 通道，避免仅使用 HTTP 时立即建立连接。
        self._grpc_channel: grpc.aio.Channel | None = None
        # 延迟创建并缓存 gRPC Stub，供健康检查和代理请求复用。
        self._grpc_stub: privacy_pb2_grpc.PrivacyServiceStub | None = None
        self._stub_lock = threading.Lock()  # 保护 grpc_stub 延迟初始化的双重检查锁
        # 管理状态：支持手动隔离/排空运维操作（#13）
        # "active" = 正常参与调度；"isolated" = 强制排除（运维手动隔离）；"drained" = 不再接受新请求但在途请求可完成
        self._admin_state = "active"

    # ------------------------------------------------------------------
    # 线程安全的健康状态与连接数属性
    # ------------------------------------------------------------------
    @property
    def is_healthy(self) -> bool:
        # 通过状态锁读取健康标志，避免与健康检查写入产生竞态。
        with self._state_lock:
            return self._is_healthy

    @is_healthy.setter
    def is_healthy(self, value: bool) -> None:
        # 通过 bool 转换统一调用方传入的真值类型。
        with self._state_lock:
            self._is_healthy = bool(value)

    @property
    def passive_unhealthy_until(self) -> float:
        # 返回被动故障冷却截止时间戳。
        with self._state_lock:
            return self._passive_unhealthy_until

    @passive_unhealthy_until.setter
    def passive_unhealthy_until(self, value: float) -> None:
        # 将截止时间转换为浮点数，便于与 monotonic 时间比较。
        with self._state_lock:
            self._passive_unhealthy_until = float(value)

    @property
    def active_connections(self) -> int:
        # 读取当前在途请求数，供最小连接数和 P2C 策略使用。
        with self._state_lock:
            return self._active_connections

    @active_connections.setter
    def active_connections(self, value: int) -> None:
        # 转为整数且不允许计数出现负值。
        with self._state_lock:
            self._active_connections = max(0, int(value))

    @property
    def admin_state(self) -> str:
        # 返回运维管理状态：active、isolated 或 drained。
        with self._state_lock:
            return self._admin_state

    @admin_state.setter
    def admin_state(self, value: str) -> None:
        # 写入管理状态，调度器会据此决定是否接收新请求。
        with self._state_lock:
            self._admin_state = value

    def mark_unhealthy(self, cooldown_seconds: float = 5.0) -> None:
        """原子地将节点标记为不健康并设置被动冷却截止时间。"""
        # 在同一临界区更新健康标志和冷却截止时间。
        with self._state_lock:
            self._is_healthy = False
            self._passive_unhealthy_until = time.monotonic() + cooldown_seconds

    def mark_healthy(self) -> None:
        """原子地将节点标记为健康并清除被动冷却截止时间。"""
        # 原子恢复健康并清除此前的被动故障冷却。
        with self._state_lock:
            self._is_healthy = True
            self._passive_unhealthy_until = 0.0

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
        # 快速路径：已初始化时不获取锁，直接复用现有 Stub。
        if self._grpc_stub is None:
            # 慢速路径：只有首次初始化才进入线程锁。
            with self._stub_lock:
                # 二次检查，防止多个线程同时通过快速路径而重复创建通道。
                if self._grpc_stub is None:
                    # 根据环境配置构造明文或 TLS gRPC 凭据。
                    credentials = backend_channel_credentials()
                    # 有凭据时创建经过 CA 校验的安全通道。
                    if credentials is not None:
                        # TLS 回源：按 PRIVACY_GATEWAY_BACKEND_TLS_CA 校验后端证书
                        self._grpc_channel = grpc.aio.secure_channel(
                            self.grpc_address, credentials, options=GRPC_CHANNEL_OPTIONS
                        )
                    else:
                        # 无凭据时使用明文通道，适用于可信内网回源。
                        self._grpc_channel = grpc.aio.insecure_channel(
                            self.grpc_address, options=GRPC_CHANNEL_OPTIONS
                        )
                    # 使用已创建的通道构造可复用 RPC Stub。
                    self._grpc_stub = privacy_pb2_grpc.PrivacyServiceStub(self._grpc_channel)
        # 返回缓存的 Stub，后续调用无需重复建立连接。
        return self._grpc_stub

    @contextlib.asynccontextmanager
    async def track_connection(self):
        """自动追踪在途活跃连接数的异步上下文管理器（供最小连接数算法精确计量）。

        执行步骤：
            1. 进入上下文：在线程锁保护下递增 `_active_connections`；
            2. yield 出让执行权给业务请求；
            3. 退出上下文：在 finally 块中在线程锁保护下递减 `_active_connections`（下限保底为 0）。

        使用 `threading.Lock` 而非 `asyncio.Lock`，因为 `active_connections` 会被同步的
        `select_node` 读取；通过 `asyncio.to_thread` 跨协程安全地获取锁，避免在途计数
        读取与修改之间的竞态条件。
        """
        # 在线程池中获取同步锁，避免在事件循环线程中阻塞其他协程。
        await asyncio.to_thread(self._state_lock.acquire)
        # 进入请求上下文时增加在途连接计数。
        self._active_connections += 1
        # 释放状态锁，让节点选择和其他请求继续读取状态。
        self._state_lock.release()
        try:
            # 将控制权交给实际 HTTP/gRPC 请求处理代码。
            yield self
        finally:
            # 无论请求成功还是抛异常，都必须执行计数回收。
            await asyncio.to_thread(self._state_lock.acquire)
            # 递减计数并将异常情况下的负值钳制为零。
            self._active_connections = max(0, self._active_connections - 1)
            # 释放计数更新所持有的同步锁。
            self._state_lock.release()

    async def close(self) -> None:
        """优雅关闭并释放该节点的底层 gRPC 通道。

        执行步骤：
            若已创建 `_grpc_channel`，调用 `await self._grpc_channel.close()` 释放底层 TCP 套接字，
            并将引用置为 None 以便垃圾回收。
        """
        # 只有实际创建过通道的节点才需要异步关闭底层连接。
        if self._grpc_channel is not None:
            # 等待 gRPC 通道完成关闭，释放底层连接资源。
            await self._grpc_channel.close()
            # 清空通道引用，允许后续生命周期重新懒加载。
            self._grpc_channel = None
            # 同步清空 Stub，避免继续使用已关闭通道。
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
        # 统一策略名称大小写，便于后续使用固定字符串分支。
        self.strategy = strategy.lower()
        # 节点池保存所有已注册后端，包含暂时不健康节点。
        self.nodes: list[BackendNode] = []
        # 普通轮询策略使用的当前游标。
        self.rr_index = 0
        # 统一节点池锁：保护 nodes 列表、rr_index 以及 current_weight 的读写。
        # 同步 API（add_node / remove_node / get_healthy_nodes）直接加锁；
        # 异步调度（select_node / health_check_loop）通过 asyncio.to_thread 安全获取，
        # 避免在事件循环线程中阻塞其他协程。
        # 节点池锁保护列表、轮询游标和动态权重。
        self._nodes_lock = threading.Lock()
        # 保留旧别名以便现有代码兼容。
        # 保留历史属性名，确保旧调用方仍与同一把锁同步。
        self.modify_lock = self._nodes_lock
        # 协程级调度锁：保证 select_node 内部状态更新（rr_index / current_weight）原子可见。
        # 异步选择锁保证一次选择中的状态更新不会互相覆盖。
        self._selection_lock = asyncio.Lock()

    def _get_healthy_nodes_locked(self, nodes: list[BackendNode]) -> list[BackendNode]:
        """在已持有 `_nodes_lock` 的前提下计算健康节点列表。

        判定准则（必须同时满足 4 项）：
            1. `node.is_healthy is True`（主动探针检查通过）；
            2. `time.monotonic() >= node.passive_unhealthy_until`（被动故障 5 秒冷却已过）；
            3. `node.circuit_breaker.is_available() is True`（熔断器允许节点进入候选集）；
            4. `node.admin_state == "active"`（未被运维手动隔离或排空）。
        """
        # 使用单调时间判断被动冷却，避免系统时钟调整影响结果。
        now = time.monotonic()
        # 只返回同时满足健康、冷却、熔断和管理状态条件的节点。
        return [
            node
            for node in nodes
            if node.is_healthy
            and now >= node.passive_unhealthy_until
            and node.circuit_breaker.is_available()
            and node.admin_state == "active"
        ]

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
        # 串行化节点注册与已有节点更新。
        with self._nodes_lock:
            # 规范化 URL 后作为节点唯一键的一部分。
            clean_url = http_url.rstrip("/")
            # 遍历节点池，处理重复注册或找到新增位置。
            for node in self.nodes:
                # HTTP URL 与 gRPC 地址同时相同才视为同一节点。
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    # 重新注册表示节点可用，清除被动故障状态。
                    node.mark_healthy()
                    # 更新静态权重并保障其不小于 1。
                    node.weight = max(1, weight)
                    # 重置运行时连接和 SWRR 状态，使注册配置立即生效。
                    node.active_connections = 0
                    node.current_weight = 0
                    node.circuit_breaker.record_success()
                    logger.info(
                        "Updated existing backend node",
                        extra={"http_url": http_url, "grpc_address": grpc_address},
                    )
                    # 节点被重新标记为健康，同步刷新健康节点数指标
                    self._update_healthy_gauge_locked()
                    # 已完成幂等更新，不再创建重复节点。
                    return

            # 唯一键不存在时创建全新的后端节点对象。
            node = BackendNode(http_url, grpc_address, weight)
            # 将新节点加入节点池。
            self.nodes.append(node)
            logger.info(
                "Added backend node",
                extra={"http_url": http_url, "grpc_address": grpc_address},
            )
            # 在仍持有节点池锁时刷新健康节点指标。
            self._update_healthy_gauge_locked()

    def remove_node(self, http_url: str, grpc_address: str) -> None:
        """从节点池安全注销工作节点并在后台线程优雅关闭其 gRPC 通道。

        执行步骤：
            1. 格式正规化并过滤出目标节点；
            2. 在独立守护线程中关闭底层 gRPC 通道，避免在调用线程中直接
               `asyncio.run(node.close())` 造成的事件循环嵌套或 fire-and-forget 泄漏；
            3. 更新节点池列表，记录日志并刷新 Prometheus 健康节点指标。
        """
        # 锁住节点列表，保证删除期间不会与注册操作交叉修改。
        with self._nodes_lock:
            # 规范化目标 URL，与注册时的键规则保持一致。
            clean_url = http_url.rstrip("/")
            # 准备保存未被删除的节点。
            new_nodes = []
            # 记录本次是否至少删除了一个节点。
            removed = False
            # 逐个检查节点唯一键。
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    # 在后台线程中完成异步 close，避免阻塞当前调用线程，
                    # 也避免与已运行事件循环产生嵌套冲突。
                    threading.Thread(
                        target=lambda n: asyncio.run(n.close()),
                        args=(node,),
                        daemon=True,
                    ).start()
                    # 标记找到目标，稍后替换节点列表。
                    removed = True
                else:
                    # 非目标节点继续保留。
                    new_nodes.append(node)
            # 用过滤后的列表原子替换旧节点池。
            self.nodes = new_nodes
            # 只有实际发生删除时才记录日志和刷新指标。
            if removed:
                logger.info(
                    "Removed backend node",
                    extra={"http_url": http_url, "grpc_address": grpc_address},
                )
                self._update_healthy_gauge_locked()

    def isolate_node(self, http_url: str, grpc_address: str) -> bool:
        """手动隔离指定节点：强制从调度池排除，不参与任何请求分发。

        Returns:
            bool: 是否成功找到并隔离了目标节点。
        """
        # 串行化隔离操作和调度节点快照读取。
        with self._nodes_lock:
            clean_url = http_url.rstrip("/")
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    node.admin_state = "isolated"
                    node.is_healthy = False
                    GATEWAY_NODE_ADMIN_STATE.labels(node=node.grpc_address).set(1)
                    logger.warning("Node manually isolated", extra={"http_url": http_url, "grpc_address": grpc_address})
                    self._update_healthy_gauge_locked()
                    # 返回成功，调用方可据此返回管理 API 成功响应。
                    return True
            # 节点不存在时返回 False，保持管理 API 可判断且不抛异常。
            return False

    def drain_node(self, http_url: str, grpc_address: str) -> bool:
        """排空指定节点：不再接受新请求，但在途请求可继续完成。

        Returns:
            bool: 是否成功找到并排空了目标节点。
        """
        # 串行化排空操作，防止新请求在状态切换期间被选中。
        with self._nodes_lock:
            clean_url = http_url.rstrip("/")
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    node.admin_state = "drained"
                    GATEWAY_NODE_ADMIN_STATE.labels(node=node.grpc_address).set(2)
                    logger.warning("Node drained (no new requests)", extra={"http_url": http_url, "grpc_address": grpc_address})
                    self._update_healthy_gauge_locked()
                    # 排空只阻止新请求，不主动取消在途请求。
                    return True
            # 找不到节点时报告失败但保持操作幂等。
            return False

    def activate_node(self, http_url: str, grpc_address: str) -> bool:
        """恢复指定节点为正常活跃状态（取消隔离或排空）。

        Returns:
            bool: 是否成功找到并激活了目标节点。
        """
        # 串行化节点激活和调度读取。
        with self._nodes_lock:
            clean_url = http_url.rstrip("/")
            for node in self.nodes:
                if node.http_url == clean_url and node.grpc_address == grpc_address:
                    node.admin_state = "active"
                    node.mark_healthy()
                    GATEWAY_NODE_ADMIN_STATE.labels(node=node.grpc_address).set(0)
                    logger.info("Node reactivated", extra={"http_url": http_url, "grpc_address": grpc_address})
                    self._update_healthy_gauge_locked()
                    # 节点重新进入可调度状态。
                    return True
            # 节点不存在时返回 False。
            return False

    def get_healthy_nodes(self) -> list[BackendNode]:
        """获取当前处于健康可用状态且未被熔断器阻断的节点列表。

        Returns:
            list[BackendNode]: 可路由的健康节点列表。
        """
        # 锁住节点池，保证筛选期间列表不会被并发修改。
        with self._nodes_lock:
            # 返回满足全部健康条件的节点对象引用。
            return self._get_healthy_nodes_locked(self.nodes)

    async def select_node(self) -> BackendNode | None:
        """根据配置的负载均衡算法从健康节点池中挑选一个目标节点。

        执行步骤：
            1. 加锁 `_selection_lock`，防止多协程并发调度产生竞争；
            2. 在后台线程获取 `_nodes_lock` 并复制一致节点池快照；
            3. 过滤可用节点，若列表为空立即返回 None；
            4. 执行算法分支并返回选中的节点对象。

        Returns:
            BackendNode | None: 选中的后端节点，若无可路由节点返回 None。
        """
        # 同一负载均衡器内串行化选择，保护游标和动态权重更新。
        async with self._selection_lock:

            def _select() -> BackendNode | None:
                # 将同步锁获取和算法计算放到线程中，避免阻塞事件循环。
                with self._nodes_lock:
                    # 先过滤健康、未冷却、未熔断且 active 的节点。
                    healthy = self._get_healthy_nodes_locked(self.nodes)
                    # 没有可用后端时让代理层返回服务不可用。
                    if not healthy:
                        return None

                    # random 和 weighted_random 当前都按静态权重执行随机抽样。
                    if self.strategy in ("random", "weighted_random"):
                        # 按节点权重构造随机抽样权重列表。
                        weights = [n.weight for n in healthy]
                        # 抽取一个节点，k=1 返回单元素列表。
                        node = random.choices(healthy, weights=weights, k=1)[0]
                        # 最终再次原子检查熔断器，避免筛选后状态已变化。
                        return node if node.circuit_breaker.allow_request() else None

                    # P2C 支持两个别名：p2c 和完整策略名。
                    if self.strategy in ("p2c", "power_of_two_choices"):
                        # 只有一个候选时无需随机抽样。
                        if len(healthy) == 1:
                            # 直接使用唯一节点，但仍需占用熔断器通行权。
                            node = healthy[0]
                            return node if node.circuit_breaker.allow_request() else None
                        # 不重复随机选择两个候选节点。
                        n1, n2 = random.sample(healthy, 2)
                        # 以连接数除以权重计算归一化负载分数。
                        score1 = n1.active_connections / max(1, n1.weight)
                        score2 = n2.active_connections / max(1, n2.weight)
                        # 选择归一化负载较低者；相等时保持第一个候选。
                        node = n1 if score1 <= score2 else n2
                        # 对最终选中节点执行熔断器原子放行。
                        return node if node.circuit_breaker.allow_request() else None

                    # 最小连接数策略选择当前在途请求最少的节点。
                    if self.strategy == "least_connections":
                        # min 会在相同连接数时保留列表中靠前的节点。
                        node = min(healthy, key=lambda n: n.active_connections)
                        # 最终确认熔断器仍允许请求。
                        return node if node.circuit_breaker.allow_request() else None

                    # 平滑加权轮询通过动态权重降低大权重节点的瞬时集中。
                    if self.strategy == "weighted_round_robin":
                        # Smooth Weighted Round-Robin (Nginx Algorithm)
                        # 计算本轮健康节点的总静态权重。
                        total_weight = sum(n.weight for n in healthy)
                        # 保存当前最佳节点，初始为空。
                        best_node: BackendNode | None = None
                        # 更新每个节点的动态权重并寻找最大值。
                        for node in healthy:
                            # 当前权重累加静态权重。
                            node.current_weight += node.weight
                            # 记录当前累计权重最高的节点。
                            if best_node is None or node.current_weight > best_node.current_weight:
                                best_node = node
                        # 健康列表非空时理论上一定存在最佳节点。
                        if best_node is not None:
                            # 选中节点扣除总权重，形成平滑分布。
                            best_node.current_weight -= total_weight
                            # 选中后再执行半开探测槽的原子占用。
                            return best_node if best_node.circuit_breaker.allow_request() else None
                        # 防御性返回，避免类型检查和未来改动引入空引用。
                        return None

                    # 未知策略和默认策略都回退到普通轮询。
                    node = healthy[self.rr_index % len(healthy)]
                    # 游标前移并按当前健康节点数量循环。
                    self.rr_index = (self.rr_index + 1) % len(healthy)
                    # 返回轮询节点前再次确认熔断器状态。
                    return node if node.circuit_breaker.allow_request() else None

            # 在线程中完成同步选择，并把结果交回当前事件循环。
            return await asyncio.to_thread(_select)

    async def close_all(self) -> None:
        """优雅关闭节点池中所有后端实例的 gRPC 通道。"""
        # 顺序关闭当前节点列表中的每个 gRPC 通道。
        for node in self.nodes:
            # 等待单个节点清理完成后再处理下一个节点。
            await node.close()

    def _update_healthy_gauge_locked(self) -> None:
        """在已持有 `_nodes_lock` 时更新 Prometheus 健康可用节点数 Gauge 指标。"""
        # 重新筛选可用节点并计算数量。
        count = len(self._get_healthy_nodes_locked(self.nodes))
        # 将健康节点数量写入 Prometheus Gauge。
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
    # 记录后台探针任务启动及配置的巡检间隔。
    logger.info("Starting background health check loop", extra={"interval_seconds": interval})
    # 创建专用 HTTP 客户端，客户端退出时由异步上下文自动释放连接池。
    # 回源 TLS 启用时按配置的 CA 校验后端证书（backend_tls_verify 在配置缺失时抛错）
    async with httpx.AsyncClient(verify=backend_tls_verify()) as client:
        # 持续运行，直到网关关闭流程取消该后台任务。
        while True:
            # 在事件循环安全地获取节点池快照，避免巡检期间节点池被 add/remove 修改
            # 导致迭代器失效或访问已移除节点。
            # 在线程池中获取节点池锁，避免阻塞事件循环。
            await asyncio.to_thread(balancer._nodes_lock.acquire)
            try:
                # 复制快照，使本轮检查不受并发注册/注销影响。
                nodes = balancer.nodes.copy()
            finally:
                # 无论复制是否成功都释放节点池锁。
                balancer._nodes_lock.release()

            # 按快照顺序逐个执行 HTTP 和 gRPC 探针。
            for node in nodes:
                # 1. 检查 REST (HTTP) 服务
                # 默认 HTTP 探针失败，只有完整响应校验通过才置为 True。
                http_ok = False
                try:
                    # 请求后端健康接口，并设置单次请求超时。
                    res = await client.get(f"{node.http_url}/health", timeout=2.0)
                    # 仅接受 HTTP 200 响应作为协议层成功。
                    if res.status_code == 200:
                        # 解析 JSON 响应体。
                        data = res.json()
                        # 业务状态也必须明确为 ok。
                        if data.get("status") == "ok":
                            http_ok = True
                except Exception as e:
                    # 探针异常只记录调试日志，由综合状态逻辑标记节点失败。
                    logger.debug(
                        "HTTP health check failed",
                        extra={"node": node.http_url, "error": str(e)},
                    )

                # 2. 检查 gRPC 服务
                # 默认 gRPC 探针失败，只有响应状态正确才置为 True。
                grpc_ok = False
                try:
                    # 构造标准 Health RPC 请求消息。
                    req = privacy_pb2.HealthRequest()
                    # 通过节点懒加载 Stub 调用后端健康方法。
                    res = await node.grpc_stub.Health(req, timeout=2.0)
                    # 检查 gRPC 业务响应状态字段。
                    if res.status == "ok":
                        grpc_ok = True
                except Exception as e:
                    # 将连接、超时和协议异常统一视为本轮探针失败。
                    logger.debug(
                        "gRPC health check failed",
                        extra={"node": node.grpc_address, "error": str(e)},
                    )

                # 3. 状态决策与更替（节点级状态锁保护，避免与 select_node 读取产生竞态）
                # 保存旧状态，用于后续仅在状态变化时记录日志。
                was_healthy = node.is_healthy
                # 判断节点是否仍处于被动故障冷却期。
                passive_cooldown = time.monotonic() < node.passive_unhealthy_until
                # 两个协议都成功且没有冷却时，节点才算综合健康。
                healthy_now = http_ok and grpc_ok and not passive_cooldown
                # 写入本轮主动探针得出的健康状态。
                node.is_healthy = healthy_now

                # Update circuit breaker based on health result
                # 健康结果成功时清零熔断器，否则累计一次失败。
                if healthy_now:
                    node.circuit_breaker.record_success()
                else:
                    node.circuit_breaker.record_failure()

                # 只有状态发生变化时才输出状态变更日志，避免日志洪泛。
                if was_healthy != healthy_now:
                    # 将布尔状态转换成便于检索的文本状态。
                    status_str = "healthy" if healthy_now else "unhealthy"
                    # 恢复使用 info，故障使用 warning，提高告警可见性。
                    log_func = logger.info if healthy_now else logger.warning
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

                # 上报熔断器状态指标（#7）
                # 读取熔断器状态，读取过程也会执行惰性恢复迁移。
                cb_state = node.circuit_breaker.state
                # 将文字状态映射为 Prometheus 使用的数值：closed=0、open=1、half_open=2。
                cb_val = 0.0 if cb_state == "closed" else (1.0 if cb_state == "open" else 2.0)
                # 上报当前节点熔断器状态。
                GATEWAY_CIRCUIT_BREAKER_STATE.labels(node=node.grpc_address).set(cb_val)

            # Update gauge after each sweep（加锁保证与节点池状态一致）
            # 重新获取节点池锁，保证 Gauge 计算与节点列表一致。
            await asyncio.to_thread(balancer._nodes_lock.acquire)
            try:
                # 使用本轮结束时的节点池状态刷新健康节点数量。
                balancer._update_healthy_gauge_locked()
            finally:
                # 确保指标更新后释放节点池锁。
                balancer._nodes_lock.release()
            # 等待下一轮探针；任务取消会在此 await 点传播。
            await asyncio.sleep(interval)
