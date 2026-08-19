"""网关统一启动入口模块 (Gateway Unified Server Entrypoint).

读取配置文件与环境变量，初始化负载均衡器，并在同一 AsyncIO 事件循环内并发托管
HTTP/REST (FastAPI + Uvicorn) 与 gRPC 异步网关服务器。

核心职责与执行逻辑：
1. **配置层级合并 (load_config)**：
   - 步骤 1：装载默认配置模板；
   - 步骤 2：加载 YAML 文件配置 (`PRIVACY_GATEWAY_CONFIG`)；
   - 步骤 3：应用环境变量覆盖 (`GATEWAY_*`) 并解析 `GATEWAY_BACKENDS`；
2. **服务并行启动 (async_main)**：
   - 步骤 1：实例化 `LoadBalancer` 并注入初始后端节点；
   - 步骤 2：启动异步 gRPC 网关服务 (`start_grpc_gateway`)；
   - 步骤 3：配置 Uvicorn 并启动 HTTP 网关（支持 TLS 终结与 mTLS `ssl.CERT_REQUIRED`）；
   - 步骤 4：拉起后台双协议主动健康检查守护协程 (`health_check_loop`)；
   - 步骤 5：使用 `asyncio.gather` 并发运行双协议服务；
3. **优雅停机与资源排空 (Graceful Shutdown)**：
   - 取消并彻底等待健康检查任务退出（消除 pending 警告）；
   - 给予在途 gRPC 请求 1 秒优雅排空期；
   - 释放所有后端 gRPC 通道。
"""

from __future__ import annotations

import argparse
import asyncio
import contextlib
import os
from typing import Any

import yaml

from PrivShield.observability.logging_config import configure_logging, get_logger

from .balancer import LoadBalancer, health_check_loop
from .grpc_proxy import start_grpc_gateway
from .http_proxy import create_http_gateway_app

# 配置结构化日志 / Configure structured logging
configure_logging(
    log_level=os.environ.get("PRIVACY_LOG_LEVEL", "INFO"),
    json_format=os.environ.get("PRIVACY_LOG_FORMAT", "text").lower() == "json",
)
logger = get_logger(__name__)


def load_config() -> dict[str, Any]:
    """从配置文件与环境变量中层级加载并合并网关配置。

    执行流程：
        1. 步骤 1：初始化默认配置字典（默认监听 0.0.0.0:8000 与 0.0.0.0:50000，策略为 round_robin）；
        2. 步骤 2：若指定了 `PRIVACY_GATEWAY_CONFIG` 环境变量且文件存在，解析 YAML 文件合并入配置；
        3. 步骤 3：检测环境变量覆盖（`GATEWAY_REST_HOST`, `GATEWAY_REST_PORT`, `GATEWAY_GRPC_PORT`,
           `GATEWAY_STRATEGY`, `GATEWAY_TLS_*` 等），环境变量具备最高优先级；
        4. 步骤 4：解析 `GATEWAY_BACKENDS` 字符串（格式如 "http://127.0.0.1:8079|127.0.0.1:50051,..."）
           并填充至后端列表。

    Returns:
        dict[str, Any]: 合并后的网关与后端配置字典。
    """
    config: dict[str, Any] = {
        "gateway": {
            "rest_host": "0.0.0.0",
            "rest_port": 8000,
            "grpc_host": "0.0.0.0",
            "grpc_port": 50000,
            "strategy": "round_robin",
            "health_check_interval": 5.0,
            # TLS 终结配置 / TLS termination config
            "tls_enabled": False,
            "tls_cert_file": "",
            "tls_key_file": "",
            "tls_ca_file": "",  # 用于 mTLS 客户端证书验证
        },
        "backends": [],
    }

    # 步骤 2: 尝试从指定配置文件读取
    config_path = os.environ.get("PRIVACY_GATEWAY_CONFIG")
    if config_path and os.path.exists(config_path):
        try:
            with open(config_path, encoding="utf-8") as f:
                loaded = yaml.safe_load(f)
                if loaded:
                    if "gateway" in loaded:
                        config["gateway"].update(loaded["gateway"])
                    if "backends" in loaded:
                        config["backends"] = loaded["backends"]
            logger.info("Loaded config from yaml", extra={"config_path": config_path})
        except Exception as e:
            logger.error("Failed to load config file", extra={"error": str(e)})

    # 步骤 3: 尝试使用环境变量进行覆盖/补充
    gw = config["gateway"]
    gw["rest_host"] = os.environ.get("GATEWAY_REST_HOST", gw["rest_host"])
    gw["rest_port"] = int(os.environ.get("GATEWAY_REST_PORT", str(gw["rest_port"])))
    gw["grpc_host"] = os.environ.get("GATEWAY_GRPC_HOST", gw["grpc_host"])
    gw["grpc_port"] = int(os.environ.get("GATEWAY_GRPC_PORT", str(gw["grpc_port"])))
    gw["strategy"] = os.environ.get("GATEWAY_STRATEGY", gw["strategy"])
    gw["health_check_interval"] = float(
        os.environ.get("GATEWAY_HEALTH_INTERVAL", str(gw["health_check_interval"]))
    )
    # TLS 终结环境变量
    gw["tls_enabled"] = os.environ.get("GATEWAY_TLS_ENABLED", str(gw["tls_enabled"])).lower() == "true"
    gw["tls_cert_file"] = os.environ.get("GATEWAY_TLS_CERT", gw["tls_cert_file"])
    gw["tls_key_file"] = os.environ.get("GATEWAY_TLS_KEY", gw["tls_key_file"])
    gw["tls_ca_file"] = os.environ.get("GATEWAY_TLS_CA", gw["tls_ca_file"])

    # 步骤 4: 解析后端节点列表字符串
    env_backends = os.environ.get("GATEWAY_BACKENDS")
    if env_backends:
        backends = []
        # 格式示例：http://127.0.0.1:8079|127.0.0.1:50051,http://127.0.0.1:8080|127.0.0.1:50052
        for item in env_backends.split(","):
            item = item.strip()
            if "|" in item:
                parts = item.split("|")
                backends.append(
                    {
                        "http_url": parts[0],
                        "grpc_address": parts[1],
                        "weight": 1,
                    }
                )
        config["backends"] = backends

    return config


async def async_main(
    rest_host: str | None = None,
    rest_port: int | None = None,
    grpc_host: str | None = None,
    grpc_port: int | None = None,
):
    """异步主函数，加载配置、初始化节点、启动双协议服务及健康检查后台任务。

    执行流程：
        1. 步骤 1：加载配置并应用 CLI 显式覆盖参数；
        2. 步骤 2：初始化 `LoadBalancer` 并将配置的静态后端节点注入调度池；
        3. 步骤 3：启动异步 gRPC 网关服务器 (`start_grpc_gateway`)；
        4. 步骤 4：构建 HTTP 反向代理应用，配置 Uvicorn（处理 TLS 证书与 mTLS 验签）；
        5. 步骤 5：启动后台主动健康检查守护任务 (`health_check_loop`)；
        6. 步骤 6：通过 `asyncio.gather` 并发运行 Uvicorn 服务与 gRPC 监听服务；
        7. 步骤 7 (优雅停机)：捕获取消或中断信号，取消健康检查任务，优雅终止 gRPC 服务并关闭所有通道。
    """
    # 步骤 1: 加载并合并配置
    config = load_config()
    gw = config["gateway"]

    # 命令行参数覆盖配置
    if rest_host is not None:
        gw["rest_host"] = rest_host
    if rest_port is not None:
        gw["rest_port"] = rest_port
    if grpc_host is not None:
        gw["grpc_host"] = grpc_host
    if grpc_port is not None:
        gw["grpc_port"] = grpc_port

    # 步骤 2: 初始化负载均衡器
    balancer = LoadBalancer(strategy=gw["strategy"])
    backends = config["backends"]

    if not backends:
        logger.warning("No backend nodes configured. Gateway will reject all forwarding requests!")

    for node_cfg in backends:
        balancer.add_node(
            http_url=node_cfg["http_url"],
            grpc_address=node_cfg["grpc_address"],
            weight=node_cfg.get("weight", 1),
        )

    # 步骤 3: 启动异步 gRPC 网关服务器（支持 TLS 终结）
    grpc_server = await start_grpc_gateway(
        host=gw["grpc_host"],
        port=gw["grpc_port"],
        balancer=balancer,
        tls_enabled=gw.get("tls_enabled", False),
        tls_cert_file=gw.get("tls_cert_file", ""),
        tls_key_file=gw.get("tls_key_file", ""),
        tls_ca_file=gw.get("tls_ca_file", ""),
    )

    # 步骤 4: 启动 HTTP 网关 FastAPI + Uvicorn 服务器（支持 TLS 终结与 mTLS）
    http_app = create_http_gateway_app(balancer)
    import ssl as _ssl

    import uvicorn

    # mTLS：配置了 CA 文件时必须显式要求并校验客户端证书，
    # 否则 uvicorn 默认 ssl.CERT_NONE，ssl_ca_certs 形同虚设（客户端证书根本不会被请求）。
    uv_config = uvicorn.Config(
        app=http_app,
        host=gw["rest_host"],
        port=gw["rest_port"],
        log_level="info",
        ssl_certfile=gw["tls_cert_file"] if gw.get("tls_enabled") else None,
        ssl_keyfile=gw["tls_key_file"] if gw.get("tls_enabled") else None,
        ssl_ca_certs=gw["tls_ca_file"] if gw.get("tls_enabled") and gw.get("tls_ca_file") else None,
        ssl_cert_reqs=(
            _ssl.CERT_REQUIRED if gw.get("tls_enabled") and gw.get("tls_ca_file") else _ssl.CERT_NONE
        ),
    )
    uv_server = uvicorn.Server(uv_config)

    # 步骤 5: 注册健康检查后台任务
    health_interval = gw["health_check_interval"]
    health_task = asyncio.create_task(health_check_loop(balancer, health_interval))

    logger.info(
        "Gateway services successfully launched",
        extra={"rest_port": gw["rest_port"], "grpc_port": gw["grpc_port"], "strategy": gw["strategy"]},
    )

    try:
        # 步骤 6: 并发挂起运行 Uvicorn 服务器和 gRPC 服务器，保持主流程运行
        await asyncio.gather(
            uv_server.serve(),
            grpc_server.wait_for_termination(),
        )
    except (asyncio.CancelledError, KeyboardInterrupt):
        logger.info("Gateway is shutting down...")
    finally:
        # 步骤 7: 优雅清理资源
        health_task.cancel()
        # 等待健康检查任务真正退出，避免事件循环关闭时
        # 出现 "Task was destroyed but it is pending" 警告。
        with contextlib.suppress(asyncio.CancelledError):
            await health_task
        await grpc_server.stop(grace=1.0)
        await balancer.close_all()
        logger.info("Gateway services safely stopped.")


def main():
    """网关同步启动入口，负责命令行解析与 asyncio 事件循环拉起。"""

    parser = argparse.ArgumentParser(
        prog="PrivShield.gateway.server",
        description="PrivShield REST + gRPC gateway / load balancer.",
    )
    parser.add_argument(
        "--rest-host",
        default=os.environ.get("GATEWAY_REST_HOST", "0.0.0.0"),
        help="Gateway REST host (default: 0.0.0.0 or GATEWAY_REST_HOST).",
    )
    parser.add_argument(
        "--rest-port",
        type=int,
        default=int(os.environ.get("GATEWAY_REST_PORT", "8000")),
        help="Gateway REST port (default: 8000 or GATEWAY_REST_PORT).",
    )
    parser.add_argument(
        "--grpc-host",
        default=os.environ.get("GATEWAY_GRPC_HOST", "0.0.0.0"),
        help="Gateway gRPC host (default: 0.0.0.0 or GATEWAY_GRPC_HOST).",
    )
    parser.add_argument(
        "--grpc-port",
        type=int,
        default=int(os.environ.get("GATEWAY_GRPC_PORT", "50000")),
        help="Gateway gRPC port (default: 50000 or GATEWAY_GRPC_PORT).",
    )
    args = parser.parse_args()

    with contextlib.suppress(KeyboardInterrupt):
        asyncio.run(
            async_main(
                rest_host=args.rest_host,
                rest_port=args.rest_port,
                grpc_host=args.grpc_host,
                grpc_port=args.grpc_port,
            )
        )


if __name__ == "__main__":
    main()
