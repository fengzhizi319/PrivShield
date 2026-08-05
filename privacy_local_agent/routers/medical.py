"""医疗敏感数据处理路由模块 / Medical Data Processing Router Module.
"""

from typing import Any

from fastapi import APIRouter
from pydantic import BaseModel, Field

from privacy_local_agent.deps import service

router = APIRouter(prefix="/v1/medical", tags=["medical"])


class MedicalProcessRequest(BaseModel):
    records: list[dict[str, Any]] = Field(..., description="医疗与身份数据记录列表")


class MedicalProcessResponse(BaseModel):
    classification_report: list[dict[str, Any]] = Field(..., description="分类分级报告")
    sanitized_data: list[dict[str, str]] = Field(..., description="脱敏清洗后的合规数据")
    summary: dict[str, Any] = Field(..., description="处理元数据与统计")


@router.post("/process", response_model=MedicalProcessResponse)

def process_medical(req: MedicalProcessRequest) -> dict[str, Any]:
    """对提交的医疗数据集执行 3-Layer 分类分级与 L4/L5 敏感数据抹平脱敏。"""
    return service.process_medical_data(req.records)
