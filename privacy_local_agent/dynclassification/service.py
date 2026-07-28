"""动态分类分级服务 / Dynamic Classification Service.

提供高层 API 入口，封装 ProfileLoader、ConfigurableRuleEngine、
CompositeRuleEngine 的调用逻辑，支持字段级、记录级和表级分类。

使用示例：
    from privacy_local_agent.dynclassification import DynClassificationService

    service = DynClassificationService(rules_dir="rules")

    # 字段级分类
    result = service.classify_field("phone_number", "13800138000")

    # 指定标准
    result = service.classify_field(
        "bank_card", "6222021234567890123",
        standard="jrt0197"
    )

    # 记录级分类
    record = {"name": "张三", "id_card": "110101199001011237", "phone": "13800138000"}
    result = service.classify_record(record)
"""

from __future__ import annotations

import time
from pathlib import Path
from typing import Any, Optional

from .composite import CompositeRuleEngine
from .engine import ConfigurableRuleEngine
from .models import (
    AuditInfo,
    ClassificationResponse,
    FieldClassificationResult,
    RecordClassificationResult,
    SecurityTag,
    TableClassificationResult,
)
from .operator_registry import OperatorRegistry
from .profile_loader import ProfileLoader


class DynClassificationService:
    """动态分类分级服务。

    高层 API 入口，根据请求上下文（domain/standard）动态加载规则，
    执行字段级、记录级和表级分类。

    Attributes:
        loader: Profile 加载器。
    """

    def __init__(self, rules_dir: str | Path = "rules"):
        """初始化服务。

        Args:
            rules_dir: 规则配置根目录路径。
        """
        self.loader = ProfileLoader(rules_dir=rules_dir)

    # ------------------------------------------------------------------
    # 字段级分类 / Field-level Classification
    # ------------------------------------------------------------------

    def classify_field(
        self,
        field_name: str,
        value: Any = None,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
        shadow_mode: bool = False,
    ) -> ClassificationResponse:
        """对单个字段进行分类。

        Args:
            field_name: 字段名。
            value: 字段值。
            domain: 领域标识（可选）。
            standard: 标准标识（可选，优先于 domain）。
            shadow_mode: 是否开启影子模式（对比新旧引擎输出差异）。

        Returns:
            ClassificationResponse 包含字段分类结果和审计信息。
        """
        start = time.monotonic()

        engine = self.loader.get_engine(domain=domain, standard=standard)
        tags = engine.evaluate(field_name, value)

        # 影子模式对比（无风险在线比对）
        if shadow_mode:
            try:
                from ..privacy.classification.classification import ClassificationAPI
                legacy_api = ClassificationAPI()
                legacy_resp = legacy_api.classify_field(field_name, value)
                legacy_level = str(legacy_resp.final_level)
                new_level = self._resolve_final_level(tags, engine)
                if legacy_level != new_level:
                    from ..observability.logging_config import get_logger
                    logger = get_logger(__name__)
                    logger.warning(
                        "dynclassification_shadow_mismatch",
                        extra={
                            "field_name": field_name,
                            "legacy_level": legacy_level,
                            "new_level": new_level,
                            "domain": domain,
                            "standard": standard,
                        },
                    )
            except Exception:
                pass

        # 确定最终等级
        final_level = self._resolve_final_level(tags, engine)

        duration_ms = (time.monotonic() - start) * 1000

        field_result = FieldClassificationResult(
            field_name=field_name,
            field_value=str(value) if value is not None else None,
            tags=tags,
            final_level=final_level,
            confidence=1.0 if tags else 0.0,
            needs_human_review=any(t.needs_human_review for t in tags),
        )

        audit = AuditInfo(
            domain=engine.domain,
            standard_id=engine.standard_id,
            rule_set_version=engine.taxonomy.version,
            rules_evaluated=engine.rule_count,
            rules_hit=len(tags),
            duration_ms=round(duration_ms, 3),
        )

        return ClassificationResponse(field_result=field_result, audit_info=audit)


    # ------------------------------------------------------------------
    # 记录级分类 / Record-level Classification
    # ------------------------------------------------------------------

    def classify_record(
        self,
        record: dict[str, Any],
        record_index: int = 0,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> ClassificationResponse:
        """对单条记录（多字段）进行分类。

        Args:
            record: 记录字典（字段名 → 字段值）。
            record_index: 记录索引。
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            ClassificationResponse 包含记录分类结果和审计信息。
        """
        start = time.monotonic()

        engine = self.loader.get_engine(domain=domain, standard=standard)
        composite_engine = self.loader.get_composite_engine(domain=domain, standard=standard)

        # 逐字段分类
        field_results: dict[str, FieldClassificationResult] = {}
        all_tags: list[SecurityTag] = []

        for field_name, value in record.items():
            tags = engine.evaluate(field_name, value)
            final_level = self._resolve_final_level(tags, engine)
            field_results[field_name] = FieldClassificationResult(
                field_name=field_name,
                field_value=str(value) if value is not None else None,
                tags=tags,
                final_level=final_level,
                confidence=1.0 if tags else 0.0,
            )
            all_tags.extend(tags)

        # 复合规则后处理
        composite_tags = composite_engine.evaluate(record, field_results)
        all_tags.extend(composite_tags)

        # 确定记录级最终等级
        record_level = self._resolve_final_level(all_tags, engine)
        record_level = composite_engine.apply_to_record_level(
            record_level, composite_tags, engine.taxonomy
        )

        duration_ms = (time.monotonic() - start) * 1000

        record_result = RecordClassificationResult(
            record_index=record_index,
            field_results=field_results,
            aggregated_tags=all_tags,
            final_level=record_level,
            confidence=1.0 if all_tags else 0.0,
            needs_human_review=any(t.needs_human_review for t in all_tags),
        )

        audit = AuditInfo(
            domain=engine.domain,
            standard_id=engine.standard_id,
            rule_set_version=engine.taxonomy.version,
            rules_evaluated=engine.rule_count * len(record),
            rules_hit=len(all_tags),
            duration_ms=round(duration_ms, 3),
        )

        return ClassificationResponse(record_result=record_result, audit_info=audit)

    # ------------------------------------------------------------------
    # 表级分类 / Table-level Classification
    # ------------------------------------------------------------------

    def classify_table(
        self,
        schema: list[str],
        rows: list[dict[str, Any]],
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> ClassificationResponse:
        """对整张表进行分类。

        Args:
            schema: 列名列表。
            rows: 记录列表。
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            ClassificationResponse 包含表分类结果和审计信息。
        """
        start = time.monotonic()

        record_results: list[RecordClassificationResult] = []
        all_tags: list[SecurityTag] = []

        for idx, row in enumerate(rows):
            resp = self.classify_record(row, record_index=idx, domain=domain, standard=standard)
            if resp.record_result:
                record_results.append(resp.record_result)
                all_tags.extend(resp.record_result.aggregated_tags)

        # 确定表级最终等级
        engine = self.loader.get_engine(domain=domain, standard=standard)
        table_level = self._resolve_final_level(all_tags, engine)

        duration_ms = (time.monotonic() - start) * 1000

        table_result = TableClassificationResult(
            schema_=schema,
            record_results=record_results,
            aggregated_tags=all_tags,
            final_level=table_level,
            confidence=1.0 if all_tags else 0.0,
        )

        audit = AuditInfo(
            domain=engine.domain,
            standard_id=engine.standard_id,
            rule_set_version=engine.taxonomy.version,
            rules_evaluated=engine.rule_count * len(rows) * max(len(schema), 1),
            rules_hit=len(all_tags),
            duration_ms=round(duration_ms, 3),
        )

        return ClassificationResponse(table_result=table_result, audit_info=audit)

    # ------------------------------------------------------------------
    # 管理接口 / Management APIs
    # ------------------------------------------------------------------

    def list_standards(self) -> list[str]:
        """列出所有可用标准。"""
        return self.loader.list_standards()

    def list_domains(self) -> list[str]:
        """列出所有可用领域包。"""
        return self.loader.list_domains()

    def list_operators(self) -> list[str]:
        """列出所有已注册算子。"""
        return OperatorRegistry.list_operators()

    def reload(self) -> None:
        """热加载：清除缓存，下次请求时重新加载配置。"""
        self.loader.invalidate_cache()

    def generate_profile_from_doc(self, doc_path: str | Path) -> dict[str, str]:
        """从标准 Markdown 文档自动抽取并生成 YAML 配置文件，并重新载入引擎缓存。

        Args:
            doc_path: 标准文档路径（如 'docs/standard/四川省健康医疗大数据应用指南.md'）。

        Returns:
            生成的 3 个 YAML 文件路径字典。
        """
        from .generator import StandardDocParser
        parser = StandardDocParser(doc_path)
        generated = parser.generate_files(self.loader.rules_dir)
        self.reload()
        return {k: str(v) for k, v in generated.items()}


    # ------------------------------------------------------------------
    # 内部方法 / Internal Methods
    # ------------------------------------------------------------------

    def _resolve_final_level(self, tags: list[SecurityTag], engine: ConfigurableRuleEngine) -> str:
        """从标签列表中解析最终等级（取最高）。"""
        if not tags:
            return engine.taxonomy.default_level
        levels = [tag.level for tag in tags]
        return engine.taxonomy.max_level(*levels)
