"""流水线服务编排器 / Pipeline Service Orchestrator.

串联 DynClassificationService 分类分级与 privacy/masking 脱敏，输出 PipelineResult。
"""

from __future__ import annotations

import csv
import time
from pathlib import Path
from typing import Any, Sequence

from .classifier import classify_records
from .masker import mask_records
from .models import (
    ClassificationSummary,
    PipelineResult,
    RecordClassificationDetail,
)


class PipelineService:
    """医疗数据分类分级 + 脱敏流水线编排器。"""

    def __init__(
        self,
        rules_dir: str | Path | None = None,
        standard: str = "jrt0197",
        profile_path: str | Path | None = None,
    ) -> None:
        self.rules_dir = rules_dir
        self.standard = standard
        self.profile_path = profile_path

    def process_records(
        self,
        records: list[dict[str, Any]],
        *,
        standard: str | None = None,
        mask_l4: bool = True,
        mask_l5: bool = True,
    ) -> PipelineResult:
        """对字典记录列表执行分类分级与脱敏流水线。

        Args:
            records: 输入记录字典列表。
            standard: 使用的分类标准（默认使用初始化标准）。
            mask_l4: 是否掩码/剥离 L4 级数据。
            mask_l5: 是否掩码/剥离 L5 级数据。

        Returns:
            PipelineResult 包含分类汇总、明细与脱敏后记录。
        """
        start_t = time.perf_counter()
        std = standard or self.standard

        if not records:
            return PipelineResult(
                classification_summary=ClassificationSummary(
                    total_records=0,
                    level_distribution={"L1": 0, "L2": 0, "L3": 0, "L4": 0, "L5": 0},
                    high_risk_fields=[],
                    standard_id=std,
                    duration_ms=0.0,
                ),
                record_details=[],
                masked_records=[],
                masking_details=[],
            )

        # 1. 逐条执行 DynClassification 分类分级
        record_details: list[RecordClassificationDetail] = classify_records(
            records=records,
            standard=std,
            rules_dir=self.rules_dir,
        )

        # 2. 统计级别分布与 L4/L5 高风险字段
        level_dist: dict[str, int] = {"L1": 0, "L2": 0, "L3": 0, "L4": 0, "L5": 0}
        high_risk_set: set[str] = set()

        for rd in record_details:
            lvl = rd.final_level
            level_dist[lvl] = level_dist.get(lvl, 0) + 1
            for fd in rd.field_details:
                if fd.sensitivity_level in ("L4", "L5"):
                    high_risk_set.add(fd.field_name)

        # 3. 对记录与字段执行脱敏抹平
        masked_recs, mask_details = mask_records(
            records=records,
            record_details=record_details,
            mask_l4=mask_l4,
            mask_l5=mask_l5,
        )

        duration_ms = round((time.perf_counter() - start_t) * 1000, 2)

        summary = ClassificationSummary(
            total_records=len(records),
            level_distribution=level_dist,
            high_risk_fields=sorted(list(high_risk_set)),
            standard_id=std,
            duration_ms=duration_ms,
        )

        return PipelineResult(
            classification_summary=summary,
            record_details=record_details,
            masked_records=masked_recs,
            masking_details=mask_details,
        )

    def process_csv(
        self,
        csv_path: str | Path,
        *,
        standard: str | None = None,
        mask_l4: bool = True,
        mask_l5: bool = True,
        encoding: str = "utf-8-sig",
    ) -> PipelineResult:
        """从 CSV 文件读取数据并执行流水线。

        Args:
            csv_path: CSV 文件路径。
            standard: 分类标准。
            mask_l4: 是否掩码 L4 级数据。
            mask_l5: 是否掩码 L5 级数据。
            encoding: 文件编码。

        Returns:
            PipelineResult 处理结果。
        """
        path = Path(csv_path)
        if not path.is_file():
            raise FileNotFoundError(f"CSV file not found: {path}")

        records: list[dict[str, Any]] = []
        with open(path, "r", encoding=encoding, errors="replace") as f:
            reader = csv.DictReader(f)
            for row in reader:
                records.append(dict(row))

        return self.process_records(
            records=records,
            standard=standard,
            mask_l4=mask_l4,
            mask_l5=mask_l5,
        )
