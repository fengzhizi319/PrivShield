"""健康检查与探针路由（health / livez / readyz）。"""

import os

from fastapi import APIRouter, Depends, HTTPException

from ..deps import NAMESPACE, service
from ..security.auth import get_current_identity
from ..security.ratelimit import rate_limit_dependency

router = APIRouter()

# 健康检查类端点单独声明认证 + 限速依赖（不使用含权限校验的 SECURITY_DEPS）。
_HEALTH_DEPS = [Depends(get_current_identity), Depends(rate_limit_dependency)]


@router.get("/health", dependencies=_HEALTH_DEPS)
def health():
    """健康检查接口。

    Returns:
        包含状态与当前命名空间的 JSON 字典。
    """
    return {"status": "ok", "namespace": NAMESPACE}


@router.get("/livez", dependencies=_HEALTH_DEPS)
def livez():
    """存活探针接口。"""
    return {"status": "alive"}


@router.get("/readyz", dependencies=_HEALTH_DEPS)
def readyz():
    """就绪探针接口。

    验证关键依赖（配置与隐私预算持久化存储）是否可访问。
    """
    if not service.resolver:
        raise HTTPException(status_code=503, detail="Configuration resolver not initialized")

    db_path = os.environ.get("PRIVACY_BUDGET_DB")
    if db_path:
        import sqlite3
        try:
            conn = sqlite3.connect(db_path, timeout=2.0)
            conn.execute("SELECT 1")
            conn.close()
        except sqlite3.Error as e:
            raise HTTPException(status_code=503, detail=f"Database check failed: {e}") from e

    return {"status": "ready"}
