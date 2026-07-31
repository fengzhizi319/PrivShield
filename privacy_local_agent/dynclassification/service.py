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
from .funnel import ClassificationFunnel
from .llm_adapter import LlmAdapter
from .models import (
    AuditInfo,
    ClassificationResponse,
    ConfidencePolicy,
    FieldClassificationResult,
    RecordClassificationResult,
    SecurityTag,
    TableClassificationResult,
)
from .ner_adapter import NerAdapter
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
        # Initialize the profile loader which handles YAML loading, caching,
        # hot-reload detection, and engine instance construction.
        self.loader = ProfileLoader(rules_dir=rules_dir)
        # Lazy-initialized NER adapter (shared across all requests).
        self._ner_adapter: NerAdapter | None = None
        # Lazy-initialized LLM adapter (shared across all requests).
        self._llm_adapter: LlmAdapter | None = None
        # Funnel instance cache: keyed by engine cache key ("domain:standard").
        self._funnel_cache: dict[str, ClassificationFunnel] = {}

    # ------------------------------------------------------------------
    # 字段级分类 / Field-level Classification
    # ------------------------------------------------------------------

    def classify_field(
        self,
        field_name: str,
        value: Any = None,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> ClassificationResponse:
        """对单个字段进行分类。

        Execution flow:
        1. Obtain (or build from cache) the rule engine for the given domain/standard.
        2. Evaluate the field against all rules to produce security tags.
        3. Resolve final level (highest rank among tags).
        4. Package results with audit info.

        Args:
            field_name: 字段名。
            value: 字段值。
            domain: 领域标识（可选）。
            standard: 标准标识（可选，优先于 domain）。

        Returns:
            ClassificationResponse 包含字段分类结果和审计信息。
        """
        # Start high-resolution timer for duration measurement.
        start = time.monotonic()

        # Step 1: Get or construct the rule engine (cached by domain:standard key).
        engine = self.loader.get_engine(domain=domain, standard=standard)

        # Step 2: Build and execute the 3-layer funnel.
        # The funnel orchestrates: Layer-1 Rule → Layer-2 NER → Layer-3 LLM
        # with confidence policy (conflict detection + decay + LLM arbitration).
        funnel = self._build_funnel(engine)
        funnel_result, suppressed_tags = funnel.classify_field(field_name, value)
        tags = funnel_result.tags

        # Step 3: Use funnel result directly (level, confidence, layer already resolved).
        final_level = funnel_result.final_level

        # Calculate execution duration in milliseconds.
        duration_ms = (time.monotonic() - start) * 1000

        # Step 5: Build the field-level classification result.
        field_result = FieldClassificationResult(
            field_name=field_name,
            field_value=str(value) if value is not None else None,
            tags=tags,
            final_level=final_level,
            confidence=funnel_result.confidence,
            needs_human_review=funnel_result.needs_human_review,
            engine_layer=funnel_result.engine_layer,
            reasoning=funnel_result.reasoning,
            suppressed_tags=suppressed_tags,
        )

        # Build audit metadata for traceability.
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

        Execution flow:
        1. Classify each field individually using the full 3-layer funnel.
        2. Run composite rule engine for multi-field combination detection.
        3. Resolve record-level final level (max of all fields + composite upgrades).

        Args:
            record: 记录字典（字段名 → 字段值）。
            record_index: 记录索引。
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            ClassificationResponse 包含记录分类结果和审计信息。
        """
        start = time.monotonic()

        field_results: dict[str, FieldClassificationResult] = {}
        all_tags: list[SecurityTag] = []
        
        for field_name, value in record.items():
            resp = self.classify_field(field_name, value, domain=domain, standard=standard)
            if resp.field_result:
                field_results[field_name] = resp.field_result
                all_tags.extend(resp.field_result.tags)

        engine = self.loader.get_engine(domain=domain, standard=standard)
        composite_engine = self.loader.get_composite_engine(domain=domain, standard=standard)
        
        composite_tags = composite_engine.evaluate(record, field_results)
        all_tags.extend(composite_tags)

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
            confidence=max((fr.confidence for fr in field_results.values()), default=0.0),
            needs_human_review=any(fr.needs_human_review for fr in field_results.values()),
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

        Iterates all rows, classifies each as a record, then aggregates
        to determine the table-level sensitivity (max across all records).

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

        # Classify each row as an independent record.
        for idx, row in enumerate(rows):
            resp = self.classify_record(row, record_index=idx, domain=domain, standard=standard)
            if resp.record_result:
                record_results.append(resp.record_result)
                # Accumulate tags from all records for table-level aggregation.
                all_tags.extend(resp.record_result.aggregated_tags)

        # Determine table-level final level (highest across all records).
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

        # Audit: total evaluations = rules * rows * columns.
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
    # Dry-Run 预演 / Dry-Run Preview
    # ------------------------------------------------------------------

    def dry_run(
        self,
        sample_data: list[dict[str, Any]],
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> dict[str, Any]:
        """对样本数据集执行规则预演，返回命中分布与等级分布。

        用于在规则正式生效前验证其命中行为是否符合预期，
        避免误配规则造成生产数据过度脱敏或漏判。

        Args:
            sample_data: 样本记录列表，每条为 {field_name: value} 字典。
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            包含 level_distribution、category_distribution、hit_details、
            summary 等统计信息的字典。
        """
        # Counter for tallying level and category distributions.
        from collections import Counter

        level_counter: Counter = Counter()       # level_id -> hit count
        category_counter: Counter = Counter()    # category_id -> hit count
        hit_details: list[dict[str, Any]] = []   # Detailed per-field hit records
        total_fields = 0                          # Total fields evaluated
        total_hits = 0                            # Fields that produced at least one tag

        # Iterate all sample records and classify each field.
        for row_idx, record in enumerate(sample_data):
            for field_name, value in record.items():
                total_fields += 1
                # Run field-level classification.
                resp = self.classify_field(
                    field_name=field_name,
                    value=value,
                    domain=domain,
                    standard=standard,
                )
                # If the field produced tags, record statistics.
                if resp.field_result and resp.field_result.tags:
                    total_hits += 1
                    level_counter[resp.field_result.final_level] += 1
                    for tag in resp.field_result.tags:
                        category_counter[tag.category] += 1
                    # Store hit detail (truncate value for safety).
                    hit_details.append({
                        "row": row_idx,
                        "field_name": field_name,
                        "value": str(value)[:100],  # Truncate to prevent log bloat
                        "level": resp.field_result.final_level,
                        "rules": [t.rule_id for t in resp.field_result.tags],
                    })

        # Fields that did NOT hit any rule are counted under the default level.
        engine = self.loader.get_engine(domain=domain, standard=standard)
        miss_count = total_fields - total_hits
        if miss_count > 0:
            level_counter[engine.taxonomy.default_level] += miss_count

        # Assemble the dry-run report.
        return {
            "summary": {
                "total_records": len(sample_data),
                "total_fields": total_fields,
                "total_hits": total_hits,
                "hit_rate": round(total_hits / max(total_fields, 1), 4),
                "domain": engine.domain,
                "standard_id": engine.standard_id,
                "rules_evaluated": engine.rule_count,
            },
            "level_distribution": dict(level_counter.most_common()),
            "category_distribution": dict(category_counter.most_common()),
            "hit_details": hit_details[:200],  # Cap at 200 entries to limit response size
        }

    # ------------------------------------------------------------------
    # 管理接口 / Management APIs
    # ------------------------------------------------------------------

    def list_standards(self) -> list[str]:
        """列出所有可用标准（扫描 standards/ 目录）。"""
        return self.loader.list_standards()

    def list_domains(self) -> list[str]:
        """列出所有可用领域包（扫描 domains/ 目录）。"""
        return self.loader.list_domains()

    def list_operators(self) -> list[str]:
        """列出所有已注册算子（从 OperatorRegistry 查询）。"""
        return OperatorRegistry.list_operators()

    def reload(self) -> None:
        """热加载：清除缓存，下次请求时重新加载配置。

        同时重置 NER/LLM 适配器单例，确保 taxonomy 中的
        模型路径、标签映射、prompt 模板等配置变更后能生效。
        """
        self.loader.invalidate_cache()
        # 清除 funnel 缓存（引擎重建后 funnel 也需重建）
        self._funnel_cache.clear()
        # 重置 NER/LLM 适配器（全局单例），使新 taxonomy 配置生效
        self._ner_adapter = None
        self._llm_adapter = None

    def generate_profile_from_doc(self, doc_path: str | Path) -> dict[str, str]:
        """从标准 Markdown 文档自动抽取并生成 YAML 配置文件，并重新载入引擎缓存。

        Workflow:
        1. Parse the Markdown document using StandardDocParser.
        2. Generate taxonomy/domain/standard YAML files into rules_dir.
        3. Invalidate cache so next request picks up the new configuration.

        Args:
            doc_path: 标准文档路径（如 'docs/standard/四川省健康医疗大数据应用指南.md'）。

        Returns:
            生成的 3 个 YAML 文件路径字典。
        """
        # Lazy import to avoid loading generator module unless needed.
        from .standard_profile_generator import StandardProfileGenerator
        parser = StandardProfileGenerator(doc_path)
        # Generate the 3 YAML files (taxonomy, domain profile, standard definition).
        generated = parser.generate_files(self.loader.rules_dir)
        # Invalidate cache to force reload on next classification request.
        self.reload()
        return {k: str(v) for k, v in generated.items()}


    # ------------------------------------------------------------------
    # 内部方法 / Internal Methods
    # ------------------------------------------------------------------

    def _resolve_final_level(self, tags: list[SecurityTag], engine: ConfigurableRuleEngine) -> str:
        """从标签列表中解析最终等级（取最高）。

        Logic: If no tags hit, return the taxonomy's default level.
        Otherwise, use taxonomy.max_level() to find the highest-rank level.
        """
        # No tags means no rules matched -> use configured default level.
        if not tags:
            return engine.taxonomy.default_level
        # Extract all level IDs from tags and find the maximum by rank.
        levels = [tag.level for tag in tags]
        return engine.taxonomy.max_level(*levels)

    def _build_funnel(self, engine: ConfigurableRuleEngine) -> ClassificationFunnel:
        """构建三层漏斗编排器（带缓存）。

        根据引擎的 taxonomy 中配置的 confidence_policy 构建漏斗，
        并按需初始化 NER/LLM 适配器（lazy-load，全局单例）。
        Funnel 实例按 engine 的 domain:standard 键缓存，避免高频创建。

        Args:
            engine: 当前请求对应的规则引擎实例。

        Returns:
            配置完成的三层漏斗编排器。
        """
        # 缓存键：与 ProfileLoader 的 engine 缓存键一致
        cache_key = f"{engine.domain}:{engine.standard_id}"
        cached = self._funnel_cache.get(cache_key)
        if cached is not None:
            return cached

        # 从 taxonomy 获取置信度策略配置
        policy = self._get_confidence_policy(engine)

        # 按需初始化 NER 适配器（全局单例，避免重复加载模型）
        ner_adapter = None
        if policy.enable_ner:
            if self._ner_adapter is None:
                taxonomy = engine.taxonomy
                self._ner_adapter = NerAdapter(
                    model_path=taxonomy.ner_model_path,
                    vocab_path=taxonomy.ner_vocab_path,
                    label_mapping=taxonomy.ner_label_mapping,
                )
            ner_adapter = self._ner_adapter

        # 按需初始化 LLM 适配器（全局单例）
        llm_adapter = None
        if policy.enable_llm or policy.enable_llm_arbitration:
            if self._llm_adapter is None:
                self._llm_adapter = LlmAdapter(
                    model_path=engine.taxonomy.llm_model_path,
                    classify_prompt_template=engine.taxonomy.llm_classify_prompt_template,
                )
            llm_adapter = self._llm_adapter

        funnel = ClassificationFunnel(
            engine=engine,
            taxonomy=engine.taxonomy,
            confidence_policy=policy,
            ner_adapter=ner_adapter,
            llm_adapter=llm_adapter,
        )
        self._funnel_cache[cache_key] = funnel
        return funnel

    def _get_confidence_policy(self, engine: ConfigurableRuleEngine) -> ConfidencePolicy:
        """从引擎的 taxonomy 中提取置信度策略配置。

        优先使用 DomainTaxonomy.confidence_policy 显式字段；
        若为 None，回退到 model_extra 中的 confidence_policy dict（向后兼容）。

        Args:
            engine: 规则引擎实例。

        Returns:
            ConfidencePolicy 实例。
        """
        taxonomy = engine.taxonomy
        # 优先使用显式字段（Pydantic 已自动校验）
        if taxonomy.confidence_policy is not None:
            return taxonomy.confidence_policy
        # 向后兼容：从 model_extra 中获取 dict 并手动构造
        if hasattr(taxonomy, "model_extra") and taxonomy.model_extra:
            policy_data = taxonomy.model_extra.get("confidence_policy")
            if policy_data and isinstance(policy_data, dict):
                return ConfidencePolicy(**policy_data)
        return ConfidencePolicy()


# Standardized class alias following full-name naming conventions
DynamicClassificationService = DynClassificationService