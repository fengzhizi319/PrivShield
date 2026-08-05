"""REST API 入口模块（应用装配）。

基于 FastAPI 构建，负责应用级装配：生命周期管理、可观测性中间件、
Prometheus metrics 暴露，以及把按域拆分到 ``routers/*`` 的子路由统一挂载。

各端点的请求模型定义在 ``schemas.py``，跨路由共享的服务单例、安全依赖与
异常映射定义在 ``deps.py``；本模块仅做组装，不再内联具体端点实现。

REST API entrypoint built with FastAPI. Endpoint implementations are split into
``routers/*``; request models live in ``schemas.py`` and shared dependencies
(service singleton, security deps, exception mapping) live in ``deps.py``.
"""

import os
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.gzip import GZipMiddleware
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

from .deps import (
    service,  # 重新导出，保持 ``from privacy_local_agent.main import service`` 可用
)
from .observability.logging_config import configure_logging
from .observability.metrics import make_asgi_app
from .observability.middleware import ObservabilityMiddleware
from .pipeline import router as pipeline_router
from .routers import budget, dp, dynclassification, file, health, kano, ldp, mask, medical, ops, profile, qol


class SecurityHeadersMiddleware(BaseHTTPMiddleware):
    """安全响应头中间件 / Security response headers middleware.

    自动为所有 HTTP 响应注入通用安全响应头，防范 MIME 嗅探、点击劫持与跨站脚本攻击。
    """

    async def dispatch(self, request: Request, call_next) -> Response:
        response: Response = await call_next(request)
        response.headers["X-Content-Type-Options"] = "nosniff"
        response.headers["X-Frame-Options"] = "DENY"
        response.headers["X-XSS-Protection"] = "1; mode=block"
        return response


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理器。

    启动时初始化结构化日志、可选的 OpenTelemetry tracing。
    关闭时执行清理逻辑。

    Args:
        app: FastAPI 应用实例。
    """
    configure_logging(
        log_level=os.environ.get("PRIVACY_LOG_LEVEL", "INFO"),
        json_format=os.environ.get("PRIVACY_LOG_FORMAT", "text").lower() == "json",
        service_name=os.environ.get("PRIVACY_SERVICE_NAME", "privacy-local-agent"),
    )
    init_tracing(
        endpoint=os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT"),
        service_name=os.environ.get(
            "OTEL_SERVICE_NAME",
            os.environ.get("PRIVACY_SERVICE_NAME", "privacy-local-agent"),
        ),
    )

    try:
        yield
    finally:
        pass


# FastAPI 应用实例；title 用于 OpenAPI 文档，lifespan 用于生命周期钩子
app = FastAPI(title="SecretFlow Local Privacy Agent", lifespan=lifespan)

# 安全响应头中间件
app.add_middleware(SecurityHeadersMiddleware)

# 高并发优化：GZip 响应压缩，减少大响应体的网络传输开销
# minimum_size=1000 表示仅压缩 >= 1KB 的响应，避免小响应压缩后反而变大
app.add_middleware(GZipMiddleware, minimum_size=1000)

# 注册可观测性中间件：request_id 透传、访问日志、Prometheus metrics。
# 注意：/metrics 本身会被中间件排除，避免自引用。
app.add_middleware(ObservabilityMiddleware)

# 暴露 Prometheus metrics。
app.mount("/metrics", make_asgi_app())

# 挂载动态分类路由
app.include_router(dynclassification.router)

# 挂载按域拆分的子路由（健康检查 / 脱敏 / DP / LDP / K-匿名 / QoL / 预算 / 推荐 / 文件处理）。
app.include_router(health.router)
app.include_router(mask.router)
app.include_router(dp.router)
app.include_router(ldp.router)
app.include_router(kano.router)
app.include_router(qol.router)
app.include_router(budget.router)
app.include_router(profile.router)
app.include_router(file.router)
app.include_router(ops.router)
app.include_router(medical.router)
app.include_router(pipeline_router)



if __name__ == "__main__":
    import argparse

    import uvicorn

    parser = argparse.ArgumentParser(
        prog="privacy_local_agent.main",
        description="SecretFlow Local Privacy Agent REST server.",
    )
    parser.add_argument(
        "--host",
        default="127.0.0.1",
        help="REST server host (default: 127.0.0.1).",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=8079,
        help="REST server port (default: 8079).",
    )
    args = parser.parse_args()
    uvicorn.run(app, host=args.host, port=args.port)
