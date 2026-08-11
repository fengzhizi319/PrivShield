"""Authentication and authorization for REST and gRPC.

提供 FastAPI dependency 与 gRPC server interceptor，支持：
- 静态 API Key（内部 / 外部服务）
- gRPC mTLS 客户端证书身份提取
- 接口级权限校验

Provides FastAPI dependencies and a gRPC server interceptor supporting static API
keys (internal/external), gRPC mTLS client-certificate identity extraction, and
per-method permission checks.
"""

from __future__ import annotations

import hmac
import time
from typing import TYPE_CHECKING, Any

import grpc
from fastapi import Depends, HTTPException, Request

from ..observability.logging_config import get_logger
from ..observability.middleware import record_auth_denial
from ..observability.metrics import AUTH_DURATION
from .config import SecuritySettings, get_security_settings
from .identity import (
    ANONYMOUS_IDENTITY,
    Identity,
    is_health_path_or_method,
    permission_for_grpc_method,
    permission_for_rest_path,
)
from .whitelist import get_whitelist_manager

if TYPE_CHECKING:
    from collections.abc import Callable

logger = get_logger(__name__)


def _extract_bearer_token(header_value: str | None) -> str | None:
    """Extract the bearer token from an Authorization header value."""
    if not header_value:
        return None
    parts = header_value.split()
    if len(parts) == 2 and parts[0].lower() == "bearer":
        return parts[1]
    return None


def _constant_time_lookup(keys: dict[str, Any], token: str) -> Any | None:
    """Constant-time lookup of ``token`` among ``keys`` to mitigate timing attacks.

    A plain ``dict.get`` is a hash-based probe whose timing can correlate with the
    stored keys, theoretically leaking information about key existence/prefixes.
    Here every stored key is compared with :func:`hmac.compare_digest` and the loop
    never short-circuits, so the running time depends only on the number of stored
    keys, not on the secret contents.
    """
    token_bytes = token.encode("utf-8")
    matched = None
    for key, value in keys.items():
        if hmac.compare_digest(key.encode("utf-8"), token_bytes):
            matched = value
    return matched


def _authenticate_api_key(settings: SecuritySettings, token: str) -> Identity | None:
    """Look up an API key in internal and external key stores.

    Internal keys are checked first so an internal token can never be shadowed by an
    external one.
    """
    internal = _constant_time_lookup(settings.internal_keys, token)
    if internal:
        return Identity("internal", internal.name, internal.scopes)
    external = _constant_time_lookup(settings.external_keys, token)
    if external:
        return Identity("external", external.name, external.scopes)
    return None


def _authenticate_mtls(
    settings: SecuritySettings, auth_context: dict[str, Any]
) -> Identity | None:
    """Derive an internal Identity from a verified mTLS client certificate.

    The gRPC auth_context is populated only when the connection uses TLS and the
    client presented a certificate. A certificate passing CA verification only
    proves possession of *a* certificate, not authorization to access this
    service — so mTLS auth is disabled by default and, when enabled via
    ``PRIVACY_AUTH_INTERNAL_MTLS_ENABLED``, the certificate Common Name must
    additionally match the whitelist.

    Whitelist lookup uses the :class:`WhitelistManager` singleton which supports:
    - YAML config file with per-CN scopes (``PRIVACY_AUTH_MTLS_WHITELIST_FILE``)
    - Fallback to static CN list (``PRIVACY_AUTH_MTLS_ALLOWED_CNS``) with ["*"] scope
    - Hot-reload when the config file changes

    An empty whitelist rejects every certificate (fail-closed).
    """
    if not settings.auth_internal_mtls_enabled:
        return None
    transport = auth_context.get("transport_security_type", [b""])[0]
    if transport != b"ssl":
        return None
    cn_bytes = auth_context.get("x509_common_name", [b""])[0]
    if not cn_bytes:
        return None
    cn = cn_bytes.decode("utf-8", errors="replace")

    # Always use the module-level singleton WhitelistManager which supports:
    # - YAML config file with per-CN scopes (PRIVACY_AUTH_MTLS_WHITELIST_FILE)
    # - Fallback to static CN list (PRIVACY_AUTH_MTLS_ALLOWED_CNS) with ["*"] scope
    # - Hot-reload via mtime detection (request-driven, passive check)
    # Creating a new WhitelistManager per RPC would re-read and re-parse the YAML
    # file on every call, causing unnecessary disk I/O under high concurrency.
    manager = get_whitelist_manager()

    entry = manager.get_entry(cn)
    if entry is None:
        logger.warning(
            "mTLS client certificate rejected: CN not in whitelist",
            extra={"cn": cn, "reason": "cn_not_allowed"},
        )
        return None
    return Identity("internal", cn, entry.scopes)


def _extract_identity_from_grpc_context(
    settings: SecuritySettings,
    context: grpc.ServicerContext,
    method: str,
) -> Identity | None:
    """Extract identity from gRPC metadata and/or mTLS auth context."""
    # First try mTLS because a verified certificate is stronger than a bearer token.
    auth_context = context.auth_context()
    if auth_context:
        identity = _authenticate_mtls(settings, auth_context)
        if identity:
            return identity

    # Health endpoints may be exempt from authentication.
    if is_health_path_or_method(method) and settings.health_no_auth:
        return Identity("internal", "health-probe", ["*"])

    metadata = dict(context.invocation_metadata() or [])
    auth_header = metadata.get("authorization", "")
    token = _extract_bearer_token(auth_header)
    if token:
        return _authenticate_api_key(settings, token)
    return None


# ---------------------------------------------------------------------------
# FastAPI dependencies
# ---------------------------------------------------------------------------

async def get_current_identity(request: Request) -> Identity:
    """FastAPI dependency that resolves the caller identity.

    When auth is disabled this returns an anonymous admin identity so downstream
    code can treat every request uniformly. Health endpoints are exempt when
    configured.
    """
    start = time.perf_counter()
    settings = get_security_settings()
    if not settings.auth_enabled:
        AUTH_DURATION.labels(result="disabled").observe(time.perf_counter() - start)
        return ANONYMOUS_IDENTITY

    path = request.url.path
    if is_health_path_or_method(path) and settings.health_no_auth:
        AUTH_DURATION.labels(result="exempt").observe(time.perf_counter() - start)
        return Identity("internal", "health-probe", ["*"])

    token = _extract_bearer_token(request.headers.get("authorization"))
    if not token:
        record_auth_denial("unauthenticated")
        AUTH_DURATION.labels(result="denied").observe(time.perf_counter() - start)
        logger.warning(
            "Authentication failed: missing credentials",
            extra={"path": path, "reason": "missing_token"},
        )
        raise HTTPException(status_code=401, detail="Unauthorized: missing credentials")

    identity = _authenticate_api_key(settings, token)
    if identity is None:
        record_auth_denial("unauthenticated")
        AUTH_DURATION.labels(result="denied").observe(time.perf_counter() - start)
        logger.warning(
            "Authentication failed: invalid credentials",
            extra={"path": path, "reason": "invalid_token"},
        )
        raise HTTPException(status_code=401, detail="Unauthorized: invalid credentials")

    # Stash identity on request.state so rate limiting can reuse it without
    # re-authenticating.
    request.state.identity = identity
    AUTH_DURATION.labels(result="success").observe(time.perf_counter() - start)
    logger.debug(
        "Authentication successful",
        extra={"path": path, "identity_type": identity.service_type, "identity_name": identity.name},
    )
    return identity


def require_permission(permission: str) -> Any:
    """Return a FastAPI dependency that enforces a specific permission.

    Usage:
        @app.post("/v1/privacy/mask", dependencies=[require_permission("privacy:mask")])
    """

    async def _checker(identity: Identity = Depends(get_current_identity)) -> None:
        if not identity.has_permission(permission):
            record_auth_denial("forbidden")
            logger.warning(
                "Authorization failed: insufficient scope",
                extra={"required_permission": permission, "identity_name": identity.name},
            )
            raise HTTPException(status_code=403, detail="Forbidden: insufficient scope")

    return Depends(_checker)


def require_rest_path_permission(path: str) -> Any:
    """Convenience wrapper that enforces the permission for a REST path.

    .. note::
        **参考实现，当前未接线**：各路由显式使用 ``require_permission(...)``
        声明权限，本便捷函数依赖 ``permission_for_rest_path`` 的参考映射，
        仅供文档/示例参考。
    """
    return require_permission(permission_for_rest_path(path))


# ---------------------------------------------------------------------------
# gRPC interceptor
# ---------------------------------------------------------------------------

class AuthInterceptor(grpc.ServerInterceptor):
    """gRPC server interceptor enforcing authentication and authorization."""

    def __init__(self, settings: SecuritySettings | None = None):
        self._settings = settings or get_security_settings()

    def _check(self, context: grpc.ServicerContext, method: str) -> Identity:
        """Authenticate and authorize the current gRPC call."""
        if not self._settings.auth_enabled:
            return ANONYMOUS_IDENTITY
        identity = _extract_identity_from_grpc_context(self._settings, context, method)
        if identity is None:
            record_auth_denial("unauthenticated")
            context.abort(grpc.StatusCode.UNAUTHENTICATED, "Missing or invalid credentials")
        assert identity is not None
        permission = permission_for_grpc_method(method)
        if not identity.has_permission(permission):
            record_auth_denial("forbidden")
            context.abort(grpc.StatusCode.PERMISSION_DENIED, "Insufficient scope")
        return identity

    def intercept_service(
        self,
        continuation: Callable[[grpc.HandlerCallDetails], grpc.RpcMethodHandler],
        handler_call_details: grpc.HandlerCallDetails,
    ) -> grpc.RpcMethodHandler:
        handler = continuation(handler_call_details)
        if handler is None:
            return handler  # type: ignore[return-value]

        method = handler_call_details.method
        kwargs = {
            "request_deserializer": handler.request_deserializer,
            "response_serializer": handler.response_serializer,
        }

        def _wrap(handler_fn: Callable[..., Any]) -> Callable[..., Any]:
            def _wrapper(request_or_iterator: Any, context: grpc.ServicerContext) -> Any:
                self._check(context, method)
                return handler_fn(request_or_iterator, context)

            return _wrapper

        if not handler.request_streaming and not handler.response_streaming:
            return grpc.unary_unary_rpc_method_handler(
                _wrap(handler.unary_unary), **kwargs
            )
        if not handler.request_streaming and handler.response_streaming:
            return grpc.unary_stream_rpc_method_handler(
                _wrap(handler.unary_stream), **kwargs
            )
        if handler.request_streaming and not handler.response_streaming:
            return grpc.stream_unary_rpc_method_handler(
                _wrap(handler.stream_unary), **kwargs
            )
        return grpc.stream_stream_rpc_method_handler(
            _wrap(handler.stream_stream), **kwargs
        )


# Helper used by the rate-limit interceptor to avoid duplicating auth extraction.
def get_identity_from_grpc_context(
    context: grpc.ServicerContext, method: str
) -> Identity:
    """Extract identity from a gRPC context, falling back to anonymous if auth off."""
    settings = get_security_settings()
    if not settings.auth_enabled:
        return ANONYMOUS_IDENTITY
    identity = _extract_identity_from_grpc_context(settings, context, method)
    if identity is None:
        # The auth interceptor would have already rejected; this fallback avoids
        # leaking anonymous rate-limit budget if called in isolation.
        context.abort(grpc.StatusCode.UNAUTHENTICATED, "Missing or invalid credentials")
    assert identity is not None
    return identity


# ---------------------------------------------------------------------------
# ASGI API-key middleware (for mounted sub-apps such as /metrics)
# ---------------------------------------------------------------------------

class ApiKeyAuthAsgiMiddleware:
    """Pure-ASGI middleware enforcing API-key auth on a mounted sub-application.

    ``app.mount()`` 挂载的子应用（如 Prometheus ``/metrics``）绕过 FastAPI 路由
    依赖体系，``get_current_identity`` 不会执行。本中间件在 ASGI 层直接校验
    ``Authorization: Bearer <key>``，复用与 REST 相同的常量时间 key 比对逻辑。

    When auth is enabled, the middleware also enforces that the authenticated
    identity holds the ``ops:metrics`` scope (or wildcard ``*``), preventing
    low-privilege API keys from accessing operational metrics.

    Behaviour:
    - ``PRIVACY_AUTH_ENABLED`` 关闭时完全透传（行为与未包裹一致）；
    - 开启时缺失/无效 key 返回 401 并计入 ``privacy_auth_denials_total``；
    - 开启时合法 key 但缺少 ``ops:metrics`` scope 返回 403。
    """

    def __init__(self, app: Any) -> None:
        self.app = app

    async def __call__(self, scope: dict[str, Any], receive: Any, send: Any) -> None:
        if scope.get("type") != "http":
            await self.app(scope, receive, send)
            return

        settings = get_security_settings()
        if settings.auth_enabled:
            headers = {
                k.decode("latin-1").lower(): v.decode("latin-1")
                for k, v in scope.get("headers", [])
            }
            token = _extract_bearer_token(headers.get("authorization"))
            identity = _authenticate_api_key(settings, token) if token else None
            if identity is None:
                record_auth_denial("unauthenticated")
                logger.warning(
                    "Authentication failed on mounted sub-app",
                    extra={"path": scope.get("path", ""), "reason": "missing_or_invalid_token"},
                )
                body = b"Unauthorized: missing or invalid credentials"
                await send(
                    {
                        "type": "http.response.start",
                        "status": 401,
                        "headers": [
                            (b"content-type", b"text/plain; charset=utf-8"),
                            (b"content-length", str(len(body)).encode("ascii")),
                        ],
                    }
                )
                await send({"type": "http.response.body", "body": body})
                return

            # Scope check: /metrics requires ops:metrics (or wildcard *)
            if not identity.has_permission("ops:metrics"):
                record_auth_denial("forbidden")
                logger.warning(
                    "Authorization failed on mounted sub-app: insufficient scope",
                    extra={"path": scope.get("path", ""), "identity_name": identity.name},
                )
                body = b"Forbidden: insufficient scope for metrics access"
                await send(
                    {
                        "type": "http.response.start",
                        "status": 403,
                        "headers": [
                            (b"content-type", b"text/plain; charset=utf-8"),
                            (b"content-length", str(len(body)).encode("ascii")),
                        ],
                    }
                )
                await send({"type": "http.response.body", "body": body})
                return

        await self.app(scope, receive, send)
