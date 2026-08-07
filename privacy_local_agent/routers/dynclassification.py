"""动态分类分级 REST 路由 / Dynamic Classification REST Router.

暴露动态分类分级相关的 HTTP API 入口：
- POST /v1/dynclassification/eval              : 单字段/批次动态分类分级
- POST /v1/dynclassification/eval_record       : 单记录动态分类分级
- POST /v1/dynclassification/eval_table        : 表格动态分类分级
- POST /v1/dynclassification/dry_run           : 规则预演（样本数据集命中分布）
- POST /v1/dynclassification/profiles/reload   : 热加载重载规则缓存
- POST /v1/dynclassification/generate_profile  : 从标准 Markdown 文档一键生成配置
- GET  /v1/dynclassification/standards         : 列出所有可用标准
- GET  /v1/dynclassification/domains           : 列出所有可用领域包
- GET  /v1/dynclassification/operators         : 列出所有已注册算子
- POST /v1/dynclassification/validate          : 校验规则 YAML 文件合法性
"""

from __future__ import annotations

import functools
import os
import threading
from pathlib import Path
from typing import Any, Optional
from fastapi import APIRouter, HTTPException, Query, status
from pydantic import BaseModel, Field

from ..deps import SECURITY_DEPS
from ..dynclassification import DynClassificationService
from ..dynclassification.validator import validate_rules_dir
from ..security.auth import require_permission

router = APIRouter(prefix="/v1/dynclassification", tags=["Dynamic Classification"])


def _bad_request_on_value_error(func):
    """将服务层抛出的 ValueError（如非法 domain/standard 名称）转换为 HTTP 400。

    ProfileLoader 对配置名称做白名单校验，非法名称（路径穿越尝试等）抛出
    ValueError；路由层在此转换为干净的 4xx，避免泄漏为 500。
    """

    @functools.wraps(func)
    def wrapper(*args, **kwargs):
        try:
            return func(*args, **kwargs)
        except ValueError as exc:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(exc),
            ) from exc

    return wrapper

# 文件路径参数校验的基准目录（默认为服务工作目录）。
# 所有用户提供的文件路径解析后必须位于该目录内，防止路径遍历攻击。
_PATH_BASE = Path(os.environ.get("PRIVACY_DYNCLASSIFICATION_PATH_BASE", ".")).resolve()


def _safe_path(raw: str) -> Path:
    """校验用户提供的文件路径，确保其位于允许的基准目录内（路径遍历防护）。

    允许相对路径与绝对路径，但解析后必须位于 ``_PATH_BASE`` 之内，
    否则抛出 400 错误。可拦截形如 ``../../etc/passwd`` 的目录穿越攻击。
    """
    candidate = Path(raw).expanduser()
    resolved = candidate.resolve() if candidate.is_absolute() else (_PATH_BASE / candidate).resolve()
    if not resolved.is_relative_to(_PATH_BASE):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"路径 '{raw}' 超出允许的基准目录",
        )
    return resolved

# 实例化通用单例 service（线程安全懒初始化）
_service: Optional[DynClassificationService] = None
_service_lock = threading.Lock()


def get_service() -> DynClassificationService:
    global _service
    if _service is None:
        with _service_lock:
            # Double-checked locking: 避免每次请求都加锁
            if _service is None:
                rules_dir = os.environ.get("PRIVACY_DYNCLASSIFICATION_RULES_DIR", "rules")
                _service = DynClassificationService(rules_dir=rules_dir)
    return _service


class DynEvalFieldRequest(BaseModel):
    field_name: str = Field(description="字段名称", alias="fieldName")
    value: Optional[Any] = Field(default=None, description="字段值")
    domain: Optional[str] = Field(default=None, description="领域标识")
    standard: Optional[str] = Field(default=None, description="标准标识")

    model_config = {"populate_by_name": True}


class DynEvalRecordRequest(BaseModel):
    record: dict[str, Any] = Field(description="记录字典")
    domain: Optional[str] = Field(default=None, description="领域标识")
    standard: Optional[str] = Field(default=None, description="标准标识")

    model_config = {"populate_by_name": True}


class DynEvalTableRequest(BaseModel):
    schema_: list[str] = Field(description="列名列表", alias="schema")
    rows: list[dict[str, Any]] = Field(description="记录列表")
    domain: Optional[str] = Field(default=None, description="领域标识")
    standard: Optional[str] = Field(default=None, description="标准标识")

    model_config = {"populate_by_name": True}


class DryRunRequest(BaseModel):
    sample_data: list[dict[str, Any]] = Field(description="样本记录列表")
    domain: Optional[str] = Field(default=None, description="领域标识")
    standard: Optional[str] = Field(default=None, description="标准标识")

    model_config = {"populate_by_name": True}


class GenerateProfileRequest(BaseModel):
    doc_path: str = Field(description="标准 Markdown 文档文件路径", alias="docPath")

    model_config = {"populate_by_name": True}


@router.post(
    "/eval",
    summary="动态分类分级评估（字段级）",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
@_bad_request_on_value_error
def evaluate_field(req: DynEvalFieldRequest):
    svc = get_service()
    svc.loader.check_and_reload()  # 触发轻量级修改检测
    resp = svc.classify_field(
        field_name=req.field_name,
        value=req.value,
        domain=req.domain,
        standard=req.standard,
    )
    return resp.model_dump(by_alias=True, exclude_none=True)


@router.post(
    "/eval_record",
    summary="动态分类分级评估（记录级）",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
@_bad_request_on_value_error
def evaluate_record(req: DynEvalRecordRequest):
    svc = get_service()
    svc.loader.check_and_reload()
    resp = svc.classify_record(
        record=req.record,
        domain=req.domain,
        standard=req.standard,
    )
    return resp.model_dump(by_alias=True, exclude_none=True)


@router.post(
    "/eval_table",
    summary="动态分类分级评估（表格级）",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
@_bad_request_on_value_error
def evaluate_table(req: DynEvalTableRequest):
    svc = get_service()
    svc.loader.check_and_reload()
    resp = svc.classify_table(
        schema=req.schema_,
        rows=req.rows,
        domain=req.domain,
        standard=req.standard,
    )
    return resp.model_dump(by_alias=True, exclude_none=True)


@router.post(
    "/dry_run",
    summary="规则预演：对样本数据集执行命中分布分析",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
@_bad_request_on_value_error
def dry_run(req: DryRunRequest):
    svc = get_service()
    svc.loader.check_and_reload()
    if not req.sample_data:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="sample_data 不能为空",
        )
    result = svc.dry_run(
        sample_data=req.sample_data,
        domain=req.domain,
        standard=req.standard,
    )
    return result


@router.post(
    "/profiles/reload",
    summary="手动触发规则配置热加载",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:write")],
)
def reload_profiles():
    svc = get_service()
    svc.reload()
    return {
        "status": "ok",
        "message": "Classification profiles and engines reloaded successfully",
    }


@router.post(
    "/generate_profile",
    summary="从标准 Markdown 文档生成 YAML 配置",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:write")],
)
def generate_profile(req: GenerateProfileRequest):
    svc = get_service()
    # 路径遍历防护：确保 doc_path 位于允许的基准目录内。
    doc_path = _safe_path(req.doc_path)
    try:
        generated = svc.generate_profile_from_doc(doc_path)
        return {
            "status": "ok",
            "message": f"Successfully generated profiles from {req.doc_path}",
            "generated_files": generated,
        }
    except Exception as exc:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"解析或生成配置文件失败: {exc}",
        )


@router.get(
    "/standards",
    summary="列出所有可用的分类分级标准",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
def list_standards():
    """列出所有可用标准。

    返回结构（向后兼容）：
    - ``standards``：标准 ID 字符串列表（旧版字段，保持不变）；
    - ``details``：标准详情列表，每项含 standard_id / description /
      taxonomy / domains / default_level / levels（按 rank 升序），
      供前端标准切换器展示“当前标准及其等级体系”。
    """
    svc = get_service()
    return {
        "standards": svc.list_standards(),
        "details": svc.list_standards_detail(),
    }


@router.get(
    "/domains",
    summary="列出所有可用的领域规则包",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
def list_domains():
    svc = get_service()
    return {"domains": svc.list_domains()}


@router.get(
    "/operators",
    summary="列出所有已注册的匹配算子",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
def list_operators():
    svc = get_service()
    return {"operators": svc.list_operators()}


@router.post(
    "/validate",
    summary="校验规则配置 YAML 合法性",
    dependencies=[*SECURITY_DEPS, require_permission("dynclassification:read")],
)
def validate_rules(rules_dir: str = Query(default="rules")):
    # 路径遍历防护：确保 rules_dir 位于允许的基准目录内。
    rules_dir = str(_safe_path(rules_dir))
    res = validate_rules_dir(rules_dir)
    return {
        "is_valid": res.is_valid,
        "errors": res.errors,
        "warnings": res.warnings,
    }