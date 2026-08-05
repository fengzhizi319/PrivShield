"""流水线 REST 路由 / Pipeline REST Router.

提供 /v1/pipeline REST API 端点。
"""

from __future__ import annotations

import csv
import io
from typing import Any, Optional

from fastapi import APIRouter, HTTPException, File, UploadFile, Query
from pydantic import BaseModel, Field

from .models import PipelineResult
from .service import PipelineService


router = APIRouter(prefix="/v1/pipeline", tags=["Pipeline"])
_service = PipelineService()


class ProcessRecordsRequest(BaseModel):
    """process_records 请求模型。"""

    records: list[dict[str, Any]] = Field(description="记录数组")
    standard: Optional[str] = Field(default="jrt0197", description="分类标准")
    mask_l4: bool = Field(default=True, description="是否掩码 L4 数据")
    mask_l5: bool = Field(default=True, description="是否掩码 L5 数据")


@router.post("/process_records", response_model=PipelineResult)
async def process_records(req: ProcessRecordsRequest) -> PipelineResult:
    """对 JSON 记录数组执行分类分级与脱敏流水线。"""
    try:
        return _service.process_records(
            records=req.records,
            standard=req.standard,
            mask_l4=req.mask_l4,
            mask_l5=req.mask_l5,
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Pipeline processing failed: {e}")


@router.post("/process_csv", response_model=PipelineResult)
async def process_csv(
    file: UploadFile = File(..., description="上传的 CSV 文件"),
    standard: Optional[str] = Query(default="jrt0197", description="分类标准"),
    mask_l4: bool = Query(default=True, description="是否掩码 L4 数据"),
    mask_l5: bool = Query(default=True, description="是否掩码 L5 数据"),
) -> PipelineResult:
    """上传 CSV 文件，执行分类分级与脱敏流水线。"""
    if not file.filename or not file.filename.endswith(".csv"):
        raise HTTPException(status_code=400, detail="Only CSV files are supported")

    try:
        content = await file.read()
        text = content.decode("utf-8", errors="replace")
        reader = csv.DictReader(io.StringIO(text))
        records = [dict(row) for row in reader]

        return _service.process_records(
            records=records,
            standard=standard,
            mask_l4=mask_l4,
            mask_l5=mask_l5,
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to process uploaded CSV: {e}")
