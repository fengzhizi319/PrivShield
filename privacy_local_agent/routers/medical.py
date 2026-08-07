"""医疗敏感数据处理路由模块 / Medical Data Processing Router Module.
"""

from typing import Any

from fastapi import APIRouter
from pydantic import BaseModel, Field, field_validator

from privacy_local_agent.deps import service

router = APIRouter(prefix="/v1/medical", tags=["medical"])

# 输入规模上限（资源耗尽防护）：单请求记录数、单记录字段数、单字段值长度。
# 脱敏管线含 NER 推理（百毫秒~秒级/字段）与复杂句法正则，无界输入可被用于 DoS。
_MAX_RECORDS = 500
_MAX_FIELDS_PER_RECORD = 100
_MAX_FIELD_VALUE_LENGTH = 100_000


class MedicalProcessRequest(BaseModel):
    records: list[dict[str, Any]] = Field(..., max_length=_MAX_RECORDS, description="医疗与身份数据记录列表")

    @field_validator("records")
    @classmethod
    def _cap_payload_size(cls, records: list[dict[str, Any]]) -> list[dict[str, Any]]:
        """限制单记录字段数与字段值长度（防超大 payload 造成的 CPU/内存耗尽）。"""
        for rec in records:
            if len(rec) > _MAX_FIELDS_PER_RECORD:
                raise ValueError(f"单条记录字段数超过上限 {_MAX_FIELDS_PER_RECORD}")
            for key, value in rec.items():
                if isinstance(value, str) and len(value) > _MAX_FIELD_VALUE_LENGTH:
                    raise ValueError(f"字段 {key!r} 值长度超过上限 {_MAX_FIELD_VALUE_LENGTH}")
        return records


class MedicalProcessResponse(BaseModel):
    classification_report: list[dict[str, Any]] = Field(..., description="分类分级报告")
    sanitized_data: list[dict[str, str]] = Field(..., description="脱敏清洗后的合规数据")
    summary: dict[str, Any] = Field(..., description="处理元数据与统计")


@router.post("/process", response_model=MedicalProcessResponse)

def process_medical(req: MedicalProcessRequest) -> dict[str, Any]:
    """对提交的医疗数据集执行 3-Layer 分类分级与 L4/L5 敏感数据抹平脱敏。"""
    return service.process_medical_data(req.records)
