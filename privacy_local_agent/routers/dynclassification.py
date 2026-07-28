"""动态分类分级 REST 路由 / Dynamic Classification REST Router.

暴露动态分类分级相关的 HTTP API 入口：
- POST /v1/dynclassification/eval              : 单字段/批次动态分类分级
- POST /v1/dynclassification/profiles/reload   : 热加载重载规则缓存
- POST /v1/dynclassification/generate_profile  : 从标准 Markdown 文档一键生成配置
- GET  /v1/dynclassification/standards         : 列出所有可用标准
- GET  /v1/dynclassification/domains           : 列出所有可用领域包
- GET  /v1/dynclassification/operators         : 列出所有已注册算子
- POST /v1/dynclassification/validate          : 校验规则 YAML 文件合法性
"""

from __future__ import annotations

from typing import Any, Optional
from fastapi import APIRouter, HTTPException, Query, status
from pydantic import BaseModel, Field

from ..dynclassification import DynClassificationService
from ..dynclassification.validator import validate_rules_dir

router = APIRouter(prefix="/v1/dynclassification", tags=["Dynamic Classification"])

# 实例化通用单例 service
_service: Optional[DynClassificationService] = None


def get_service() -> DynClassificationService:
    global _service
    if _service is None:
        _service = DynClassificationService(rules_dir="rules")
    return _service


class DynEvalRequest(BaseModel):
    field_name: str = Field(description="字段名称", alias="fieldName")
    value: Optional[Any] = Field(default=None, description="字段值")
    domain: Optional[str] = Field(default=None, description="领域标识")
    standard: Optional[str] = Field(default=None, description="标准标识")

    model_config = {"populate_by_name": True}


class GenerateProfileRequest(BaseModel):
    doc_path: str = Field(description="标准 Markdown 文档文件路径", alias="docPath")

    model_config = {"populate_by_name": True}


@router.post("/eval", summary="动态分类分级评估")
def evaluate_field(req: DynEvalRequest):
    svc = get_service()
    svc.loader.check_and_reload()  # 触发轻量级修改检测
    resp = svc.classify_field(
        field_name=req.field_name,
        value=req.value,
        domain=req.domain,
        standard=req.standard,
    )
    return resp.model_dump(by_alias=True, exclude_none=True)


@router.post("/profiles/reload", summary="手动触发规则配置热加载")
def reload_profiles():
    svc = get_service()
    svc.reload()
    return {
        "status": "ok",
        "message": "Classification profiles and engines reloaded successfully",
    }


@router.post("/generate_profile", summary="从标准 Markdown 文档生成 YAML 配置")
def generate_profile(req: GenerateProfileRequest):
    svc = get_service()
    try:
        generated = svc.generate_profile_from_doc(req.doc_path)
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


@router.get("/standards", summary="列出所有可用的分类分级标准")
def list_standards():
    svc = get_service()
    return {"standards": svc.list_standards()}


@router.get("/domains", summary="列出所有可用的领域规则包")
def list_domains():
    svc = get_service()
    return {"domains": svc.list_domains()}


@router.get("/operators", summary="列出所有已注册的匹配算子")
def list_operators():
    svc = get_service()
    return {"operators": svc.list_operators()}


@router.post("/validate", summary="校验规则配置 YAML 合法性")
def validate_rules(rules_dir: str = Query(default="rules")):
    res = validate_rules_dir(rules_dir)
    return {
        "is_valid": res.is_valid,
        "errors": res.errors,
        "warnings": res.warnings,
    }
