"""gRPC 泛化代理服务模块 (gRPC Generic Proxy Service).

基于 grpc.aio (Python AsyncIO gRPC) 实现，承接所有客户端 gRPC 请求并动态分发给
后端健康的工作节点。支持泛化反射转发、元数据双向透传、故障重试、熔断器保护与 Prometheus 指标采集。

核心设计与执行逻辑：
1. **动态泛化反射绑定 (_bind_generic_methods)**：
   在类初始化时自动反射扫描 ``PrivacyServiceServicer`` 基类，为每个公开的 RPC 接口绑定统一的
   转发闭包。Protocol Buffers 增改接口时，仅需重新编译存根，网关自动路由，零代码修改；
2. **全双工元数据透传 (Metadata Forwarding)**：
   - 入站：提取客户端 invocation metadata 并在转发时注入；
   - 出站：拦截后端的 initial metadata (响应头) 与 trailing metadata (响应尾)，原样回传客户端；
3. **高可用故障转移与熔断 (Fault-Tolerant Proxying)**：
   - 遭遇 `UNAVAILABLE` 或连接崩溃：计入重试，毫秒级下线故障节点（5s 冷却），重试其他可用实例；
   - 遭遇业务异常 (如 `INVALID_ARGUMENT`)：直接透传原错误码，不计入节点故障与重试；
4. **大消息体与南北向 TLS 终结**：
   - 全链路配置 64 MiB 消息收发缓冲区，适应高维数据与图片分类传输；
   - 支持南北向 TLS 终结与强约束 mTLS 客户端验签。
"""

from __future__ import annotations

import time

import grpc

from engine import privacy_pb2_grpc
from engine.observability.logging_config import get_logger
from engine.observability.metrics import (
    GATEWAY_LATENCY,
    GATEWAY_REQUESTS_TOTAL,
    GATEWAY_RETRIES_TOTAL,
)

from .balancer import GRPC_MAX_MESSAGE_BYTES, LoadBalancer

logger = get_logger(__name__)


class GatewayGrpcServicer(privacy_pb2_grpc.PrivacyServiceServicer):
    """gRPC 网关泛化服务实现类。

    通过动态反射绑定实现 ``PrivacyService`` 下所有 RPC 方法的透明转发。
    """

    def __init__(self, balancer: LoadBalancer):
        """初始化 gRPC Servicer 并执行方法反射绑定。

        Args:
            balancer: 关联的负载均衡调度器实例。
        """
        self.balancer = balancer
        self._bind_generic_methods()

    def _bind_generic_methods(self) -> None:
        """为 ``PrivacyService`` 中所有 RPC 方法动态绑定统一转发函数。

        执行步骤：
            1. 获取 `PrivacyServiceServicer` 基类属性列表；
            2. 过滤掉内部私有方法（以 `_` 开头）及特殊元方法（如 `__init__`, `_forward` 等）；
            3. 对每个可调用的公开 RPC 方法名，调用 `_make_forwarder(name)` 生成转发闭包；
            4. 将闭包动态挂载到 `self` 实例上，覆盖基类的默认 `UNIMPLEMENTED` 实现。
        """
        base = privacy_pb2_grpc.PrivacyServiceServicer
        for name in dir(base):
            if name.startswith("_"):
                continue
            attr = getattr(base, name)
            if not callable(attr):
                continue
            if name in ("__init__", "_bind_generic_methods", "_forward"):
                continue
            setattr(self, name, self._make_forwarder(name))

    def _make_forwarder(self, method_name: str):
        """构造给定 RPC 方法名的转发协程包装器。

        Args:
            method_name: 目标 RPC 方法名称 (例如 "Mask", "ClassifyField")。

        Returns:
            callable: 接收 (request, context) 的异步处理函数。
        """

        async def _generic_method(request, context):
            return await self._forward(method_name, request, context)

        return _generic_method

    async def _forward(self, method_name: str, request, context):
        """通用底层反向代理转发适配器（含重试、元数据透传、熔断与指标采集）。

        执行步骤：
            1. **准备阶段**：初始化计时器与最大重试次数 (`max_retries = 3`)；
            2. **重试与故障转移循环**：
               - 步骤 2.1：从 `balancer.select_node()` 选取健康后端节点，若无可用节点调用 `context.abort(UNAVAILABLE)`；
               - 步骤 2.2：提取客户端请求中携带的 invocation metadata；
               - 步骤 2.3：在 `node.track_connection()` 上下文内获取对应 RPC Stub 方法并异步发起调用（30.0s 超时）；
               - 步骤 2.4：元数据透传：
                 * 提取后端的 initial metadata 并通过 `context.send_initial_metadata()` 传回；
                 * 提取后端的 trailing metadata 并通过 `context.set_trailing_metadata()` 传回；
               - 步骤 2.5：记录成功指标（Prometheus status='OK'）并复位熔断器；
               - 步骤 2.6：返回后端生成的 response Proto 实例；
               - 步骤 2.7：异常处理：
                 * 若捕获到 `StatusCode.UNAVAILABLE`：记录熔断器失败，递增 `privacy_gateway_retries_total` 指标，
                   触发毫秒级被动下线（5s 冷却），并循环重试下一健康节点；
                 * 若捕获到业务级 `RpcError`（如 `INVALID_ARGUMENT`, `NOT_FOUND` 等）：
                   不进行重试，记录耗时指标后直接调用 `context.abort(exc.code(), exc.details())` 原样回传给客户端；
                 * 若捕获到未知异常：记录熔断器失败与被动下线，尝试故障转移重试；
            3. **重试耗尽处理**：记录 Prometheus status='INTERNAL' 指标，输出 ERROR 日志并调用 `context.abort` 终止调用。
        """
        max_retries = 3
        last_exception: Exception | None = None
        start_time = time.perf_counter()

        for attempt in range(max_retries):
            # 步骤 2.1: 动态挑选健康节点
            node = await self.balancer.select_node()
            if not node:
                duration = time.perf_counter() - start_time
                GATEWAY_REQUESTS_TOTAL.labels(protocol="grpc", method=method_name, status="UNAVAILABLE").inc()
                GATEWAY_LATENCY.labels(protocol="grpc").observe(duration)
                logger.error(
                    "No healthy gRPC nodes available",
                    extra={"method": method_name},
                )
                await context.abort(
                    grpc.StatusCode.UNAVAILABLE,
                    "No healthy backend nodes available",
                )

            assert node is not None
            try:
                # 步骤 2.2 & 2.3: 追踪连接数并调用后端 RPC
                async with node.track_connection():
                    stub_method = getattr(node.grpc_stub, method_name)
                    metadata = None
                    if hasattr(context, "invocation_metadata") and callable(context.invocation_metadata):
                        metadata = context.invocation_metadata()

                    call = stub_method(request, timeout=30.0, metadata=metadata)
                    response = await call

                # 步骤 2.4: 将后端的响应头与响应尾元数据透传给客户端
                try:
                    initial_md = await call.initial_metadata()
                    if initial_md and hasattr(context, "send_initial_metadata") and callable(
                        context.send_initial_metadata
                    ):
                        await context.send_initial_metadata(initial_md)
                except Exception as e:
                    logger.debug(
                        "Failed to forward initial metadata",
                        extra={"method": method_name, "error": str(e)},
                    )

                try:
                    trailing_md = await call.trailing_metadata()
                    if trailing_md and hasattr(context, "set_trailing_metadata") and callable(
                        context.set_trailing_metadata
                    ):
                        context.set_trailing_metadata(trailing_md)
                except Exception as e:
                    logger.debug(
                        "Failed to forward trailing metadata",
                        extra={"method": method_name, "error": str(e)},
                    )

                # 步骤 2.5: 记录成功指标与熔断器复位
                duration = time.perf_counter() - start_time
                GATEWAY_REQUESTS_TOTAL.labels(protocol="grpc", method=method_name, status="OK").inc()
                GATEWAY_LATENCY.labels(protocol="grpc").observe(duration)
                node.circuit_breaker.record_success()
                return response

            except grpc.RpcError as exc:
                # 步骤 2.7: gRPC 状态码精细分流
                if exc.code() == grpc.StatusCode.UNAVAILABLE:
                    last_exception = exc
                    node.circuit_breaker.record_failure()
                    GATEWAY_RETRIES_TOTAL.labels(protocol="grpc", reason="unavailable").inc()
                    logger.warning(
                        "gRPC forward attempt failed (UNAVAILABLE), retrying",
                        extra={
                            "method": method_name,
                            "attempt": attempt + 1,
                            "max_retries": max_retries,
                            "node": node.grpc_address,
                            "circuit_breaker": node.circuit_breaker.state,
                        },
                    )
                    # 毫秒级被动健康下线（5秒冷却）
                    node.mark_unhealthy(cooldown_seconds=5.0)
                else:
                    # 正常的业务级/参数类错误，无需重试，直接透传
                    duration = time.perf_counter() - start_time
                    GATEWAY_REQUESTS_TOTAL.labels(
                        protocol="grpc", method=method_name, status=exc.code().name
                    ).inc()
                    GATEWAY_LATENCY.labels(protocol="grpc").observe(duration)
                    await context.abort(exc.code(), exc.details())
            except Exception as exc:
                last_exception = exc
                node.circuit_breaker.record_failure()
                GATEWAY_RETRIES_TOTAL.labels(protocol="grpc", reason="unexpected_error").inc()
                logger.warning(
                    "gRPC forward attempt failed (unexpected), retrying",
                    extra={
                        "method": method_name,
                        "attempt": attempt + 1,
                        "max_retries": max_retries,
                        "node": node.grpc_address,
                        "error": str(exc),
                        "circuit_breaker": node.circuit_breaker.state,
                    },
                )
                node.mark_unhealthy(cooldown_seconds=5.0)

        # 步骤 3: 若全部重试机会已耗尽
        duration = time.perf_counter() - start_time
        GATEWAY_REQUESTS_TOTAL.labels(protocol="grpc", method=method_name, status="INTERNAL").inc()
        GATEWAY_LATENCY.labels(protocol="grpc").observe(duration)
        logger.error(
            "gRPC forward failed after all retries",
            extra={"method": method_name, "max_retries": max_retries, "last_error": str(last_exception)},
        )
        if isinstance(last_exception, grpc.RpcError):
            await context.abort(last_exception.code(), last_exception.details())
        else:
            await context.abort(
                grpc.StatusCode.INTERNAL,
                "Gateway internal error: all backend retry attempts failed",
            )


async def start_grpc_gateway(
    host: str,
    port: int,
    balancer: LoadBalancer,
    tls_enabled: bool = False,
    tls_cert_file: str = "",
    tls_key_file: str = "",
    tls_ca_file: str = "",
) -> grpc.aio.Server:
    """初始化并启动异步 gRPC 网关服务器（支持 TLS 终结与 mTLS 双向认证）。

    执行步骤：
        1. 步骤 1：实例化 `grpc.aio.server`，显式配置 64 MiB 收发缓冲区限制；
        2. 步骤 2：注册 `GatewayGrpcServicer` 到 gRPC 服务器；
        3. 步骤 3：TLS 终结配置处理：
           - 若开启 TLS (`tls_enabled = True`)：
             * Fail-Fast 校验：若未提供证书或私钥路径，直接抛出 `ValueError` 拒绝启动；
             * 读取服务器证书链与私钥二进制流；
             * 若提供 `tls_ca_file`，读取 CA 证书并开启 `require_client_auth=True`（mTLS 强双向认证）；
             * 调用 `grpc.ssl_server_credentials` 创建凭据并绑定 Secure Port；
           - 若未开启 TLS：
             * 调用 `server.add_insecure_port` 绑定明文端口并输出警告日志；
        4. 步骤 4：调用 `await server.start()` 启动服务监听并返回 Server 对象。

    Args:
        host: 绑定监听的主机名或 IP 地址。
        port: 端口号。
        balancer: 关联的负载均衡调度器实例。
        tls_enabled: 是否启用入站 TLS 终结。
        tls_cert_file: 服务器证书文件路径。
        tls_key_file: 服务器私钥文件路径。
        tls_ca_file: CA 证书文件路径（用于 mTLS 客户端验签）。

    Returns:
        grpc.aio.Server: 启动就绪的 gRPC 异步服务器实例。

    Raises:
        ValueError: 启用 TLS 但未提供证书或私钥文件。
    """
    server = grpc.aio.server(
        options=[
            # 收发消息上限 64 MiB，与后端 grpc_server.serve() 对齐；
            # 默认 4 MiB 对大表/图片分类场景极易超限导致连接重置。
            ("grpc.max_receive_message_length", GRPC_MAX_MESSAGE_BYTES),
            ("grpc.max_send_message_length", GRPC_MAX_MESSAGE_BYTES),
        ]
    )
    privacy_pb2_grpc.add_PrivacyServiceServicer_to_server(
        GatewayGrpcServicer(balancer), server
    )

    if tls_enabled:
        # Fail-fast: TLS requested but cert/key missing — do NOT silently downgrade
        # to plaintext. This mirrors the HTTP side's fail-fast behaviour and the
        # agent's own SecuritySettings validator.
        if not tls_cert_file or not tls_key_file:
            raise ValueError(
                "Gateway gRPC TLS is enabled but tls_cert_file and/or tls_key_file "
                "are missing. Refusing to start with plaintext transport."
            )
        # 读取证书和私钥 / Read cert and key
        with open(tls_cert_file, "rb") as f:
            cert_chain = f.read()
        with open(tls_key_file, "rb") as f:
            private_key = f.read()

        # 如果提供 CA 证书，启用 mTLS 客户端验证
        root_certificates = None
        if tls_ca_file:
            with open(tls_ca_file, "rb") as f:
                root_certificates = f.read()

        credentials = grpc.ssl_server_credentials(
            [(private_key, cert_chain)],
            root_certificates=root_certificates,
            require_client_auth=bool(root_certificates),
        )
        server.add_secure_port(f"{host}:{port}", credentials)
        logger.info(
            "Gateway gRPC server started with TLS",
            extra={"host": host, "port": port, "mtls": bool(root_certificates)},
        )
    else:
        server.add_insecure_port(f"{host}:{port}")
        logger.warning(
            "Gateway gRPC server started WITHOUT TLS — plaintext transport",
            extra={"host": host, "port": port},
        )

    await server.start()
    return server
