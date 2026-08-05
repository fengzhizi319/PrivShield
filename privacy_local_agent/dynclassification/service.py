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

import logging
import os
import threading
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

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# fork-after-warmup 预加载适配器注册表 / Preloaded adapter registry (COW)
# ---------------------------------------------------------------------------
# 高并发优化：launcher.py 的 --warmup 模式在主进程 fork 前加载 ML 模型，
# 并把加载好的 adapter 实例注册到此处。fork 后子进程（worker）首次需要
# NER/LLM 适配器时优先复用注册表中的实例——模型权重内存页在主进程 fork
# 时已驻留，子进程直接继承（Copy-on-Write 共享只读页），避免 N 份模型
# 内存翻倍，也省去重新读盘 + 重新构建模型的开销。
#
# 注意：注册表是 fork 前填充的，子进程各自持有独立副本，互不影响；
# 适配器内部使用锁保护初始化，fork 时锁未被持有，可安全复用。
_preloaded_adapters: dict[str, Any] = {}
_preloaded_lock = threading.Lock()


def register_preloaded_adapter(kind: str, adapter: Any) -> None:
    """注册一个 fork 前预加载的适配器实例（launcher --warmup 使用）。"""
    with _preloaded_lock:
        _preloaded_adapters[kind] = adapter


def consume_preloaded_adapter(kind: str) -> Any | None:
    """获取 fork 前预加载的适配器实例；未注册时返回 None。"""
    with _preloaded_lock:
        return _preloaded_adapters.get(kind)


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
        cache_size = int(os.environ.get("PRIVACY_CLASSIFICATION_CACHE_SIZE", "10000"))
        from ..privacy.high_concurrency import HighConcurrencyLRUCache
        self._classification_cache = HighConcurrencyLRUCache[tuple[str, str, str, str, bool], ClassificationResponse](
            capacity=cache_size
        )

    def _build_funnel(self, engine: ConfigurableRuleEngine) -> ClassificationFunnel:
        """构建分类漏斗实例。"""
        cache_key = f"{engine.domain}:{engine.standard_id}"
        if cache_key not in self._funnel_cache:
            self._funnel_cache[cache_key] = ClassificationFunnel(
                engine=engine,
                ner_adapter=self._ner_adapter or consume_preloaded_adapter("ner"),
                llm_adapter=self._llm_adapter or consume_preloaded_adapter("llm"),
            )
        return self._funnel_cache[cache_key]

    def _resolve_final_level(self, tags: list[SecurityTag], engine: ConfigurableRuleEngine) -> str:
        """根据所有命中标签解析最终等级。"""
        if not tags:
            return "L0"
        return max((tag.level for tag in tags), key=lambda l: engine.taxonomy.level_rank.get(l, 0))

    def clear_cache(self) -> None:
        """清空分类缓存（规则重载或配置更新时调用）。"""
        self._classification_cache.clear()

    # ------------------------------------------------------------------
    # 字段级分类 / Field-level Classification
    # ------------------------------------------------------------------

    def classify_field(
        self,
        field_name: str,
        value: Any,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
        sanitize: bool = False,
    ) -> ClassificationResponse:
        """对单个字段进行分类与智能抹平脱敏。

        Execution flow:
        1. Check high-concurrency LRU cache for identical (domain, standard, field_name, value).
        2. Obtain (or build from cache) the rule engine for the given domain/standard.
        3. Evaluate the field against all rules/NER/LLM funnel to produce security tags.
        4. Resolve final level and package results with audit info.
        5. If sanitize=True, compute smart sanitized_value via masking/LLM.

        Args:
            field_name: 字段名。
            value: 字段值。
            domain: 领域标识（可选）。
            standard: 标准标识（可选，优先于 domain）。
            sanitize: 是否计算并生成智能抹平/脱敏后的字段值（默认 False）。

        Returns:
            ClassificationResponse 包含字段分类结果和审计信息。
        """
        # Start high-resolution timer for duration measurement.
        start = time.monotonic()

        # Step 0: 高并发 LRU 缓存查找（防超长 Base64 内存暴涨）
        val_str_raw = str(value) if value is not None else ""
        val_key = val_str_raw if len(val_str_raw) <= 200 else (len(val_str_raw), val_str_raw[:100], hash(val_str_raw))
        cache_key = (domain or "", standard or "", field_name, val_key, sanitize)
        cached_resp = self._classification_cache.get(cache_key)
        if cached_resp is not None:
            if sanitize and cached_resp.field_result.sanitized_value is None:
                val_str = str(value) if value is not None else ""
                from .image_redaction import sanitize_image_input
                is_image = (
                    len(val_str.strip()) < 512
                    and any(val_str.strip().lower().endswith(ext) for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".dcm", ".dicom"))
                ) or val_str.strip().lower().startswith("data:image/")
                if is_image:
                    cached_resp.field_result.sanitized_value = sanitize_image_input(val_str)
                else:
                    from privacy_local_agent.privacy.masking import mask_value
                    from privacy_local_agent.medical_pipeline.rules import L4_PATTERNS, L5_PATTERNS
                    s_text = val_str
                    for pat, replacement in L4_PATTERNS:
                        s_text = pat.sub(replacement, s_text)
                    for pat, replacement in L5_PATTERNS:
                        s_text = pat.sub(replacement, s_text)
                    if cached_resp.field_result.final_level in ["L3", "L4", "L5", "C4", "C5"]:
                        s_text = mask_value(field_name, s_text)
                    cached_resp.field_result.sanitized_value = s_text

            duration_ms = (time.monotonic() - start) * 1000
            # 构造新的 response 保持独立的执行耗时审计
            audit = AuditInfo(
                domain=cached_resp.audit_info.domain,
                standard_id=cached_resp.audit_info.standard_id,
                rule_set_version=cached_resp.audit_info.rule_set_version,
                rules_evaluated=cached_resp.audit_info.rules_evaluated,
                rules_hit=cached_resp.audit_info.rules_hit,
                duration_ms=round(duration_ms, 3),
            )
            return ClassificationResponse(field_result=cached_resp.field_result, audit_info=audit)

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

        # Compute smart sanitized_value if requested
        sanitized_val: str | None = None
        if sanitize:
            val_str = str(value) if value is not None else ""
            from .image_redaction import sanitize_image_input
            # 判断入参是否为图片病例 (本地文件路径或 Base64 Data URI)
            is_image = (
                len(val_str.strip()) < 512
                and any(val_str.strip().lower().endswith(ext) for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".dcm", ".dicom"))
            ) or val_str.strip().lower().startswith("data:image/")
            
            if is_image:
                # 图片病例脱敏：执行图像盲区打码，返回相同格式的图像出参 (新文件路径或 Base64 Data URI)
                sanitized_val = sanitize_image_input(val_str)
            elif getattr(funnel_result, "sanitized_value", None):
                sanitized_val = funnel_result.sanitized_value
            else:
                from privacy_local_agent.privacy.masking import mask_value
                from privacy_local_agent.medical_pipeline.rules import L4_PATTERNS, L5_PATTERNS
                s_text = val_str
                for pat, replacement in L4_PATTERNS:
                    s_text = pat.sub(replacement, s_text)
                for pat, replacement in L5_PATTERNS:
                    s_text = pat.sub(replacement, s_text)
                if final_level in ["L3", "L4", "L5", "C4", "C5"]:
                    s_text = mask_value(field_name, s_text)
                sanitized_val = s_text

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
            sanitized_value=sanitized_val,
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

        response = ClassificationResponse(field_result=field_result, audit_info=audit)
        # 写入高并发 LRU 缓存
        self._classification_cache.put(cache_key, response)
        return response


    # ------------------------------------------------------------------
    # 记录级分类 / Record-level Classification
    # ------------------------------------------------------------------

    def classify_record(
        self,
        record: dict[str, Any],
        record_index: int = 0,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
        sanitize: bool = False,
    ) -> ClassificationResponse:
        """对单条记录（多字段）进行分类与智能抹平脱敏。

        Execution flow:
        1. Classify each field individually using the full 3-layer funnel (with sanitize parameter).
        2. Run composite rule engine for multi-field combination detection.
        3. Resolve record-level final level (max of all fields + composite upgrades).

        Args:
            record: 记录字典（字段名 → 字段值）。
            record_index: 记录索引。
            domain: 领域标识。
            standard: 标准标识。
            sanitize: 是否进行高敏与 PII 脱敏抹平（默认 False）。

        Returns:
            ClassificationResponse 包含记录分类结果和审计信息。
        """
        start = time.monotonic()

        field_results: dict[str, FieldClassificationResult] = {}
        all_tags: list[SecurityTag] = []
        
        for field_name, value in record.items():
            resp = self.classify_field(field_name, value, domain=domain, standard=standard, sanitize=sanitize)
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

        # Classify rows (parallelize using ThreadPoolExecutor when rows count > 16)
        if len(rows) > 16:
            import concurrent.futures
            max_workers = min(16, os.cpu_count() or 4)
            with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
                futures = [
                    executor.submit(self.classify_record, row, idx, domain, standard)
                    for idx, row in enumerate(rows)
                ]
                for fut in futures:
                    resp = fut.result()
                    if resp.record_result:
                        record_results.append(resp.record_result)
                        all_tags.extend(resp.record_result.aggregated_tags)
        else:
            for idx, row in enumerate(rows):
                resp = self.classify_record(row, record_index=idx, domain=domain, standard=standard)
                if resp.record_result:
                    record_results.append(resp.record_result)
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

    def list_standards_detail(self) -> list[dict[str, Any]]:
        """列出所有可用标准的详细信息（含等级体系），供前端标准切换器渲染。

        每个标准返回：standard_id、description、taxonomy、domains、
        default_level、按 rank 升序排列的等级列表（levels）、
        该标准组合下的规则总数（rule_count）以及引用 taxonomy 下的
        分类总数（category_count）。
        配置损坏的标准会被跳过（仅记录警告），避免单个坏文件拖垮整个列表。

        Returns:
            标准详情字典列表（按 standard_id 排序，保证前端顺序稳定）。
        """
        details: list[dict[str, Any]] = []
        for sid in sorted(self.list_standards()):
            try:
                std_def = self.loader.load_standard(sid)
                taxonomy = self.loader.load_taxonomy(std_def.taxonomy)
            except Exception as exc:  # noqa: BLE001
                # 单个标准配置损坏不应影响其余标准的可用性。
                logger.warning(
                    "Skip broken standard '%s' in list_standards_detail: %s", sid, exc
                )
                continue
            # 统计该标准组合下所有领域包的规则总数（普通 + 降级 + 复合）。
            # Count total rules (normal + downgrade + composite) across the standard's domain packs.
            # 单个领域包损坏仅跳过计数，不影响标准本身的可用性。
            rule_count = 0
            for domain in std_def.domains:
                try:
                    profile = self.loader.load_profile(domain)
                except Exception as exc:  # noqa: BLE001
                    logger.warning(
                        "Skip broken domain '%s' while counting rules for standard '%s': %s",
                        domain, sid, exc,
                    )
                    continue
                rule_count += (
                    len(profile.rules)
                    + len(profile.downgrade_rules)
                    + len(profile.composite_rules)
                )
            details.append(
                {
                    "standard_id": sid,
                    "description": std_def.description,
                    "taxonomy": std_def.taxonomy,
                    "domains": list(std_def.domains),
                    "default_level": taxonomy.default_level,
                    "levels": [
                        {"id": lv.id, "name": lv.name, "rank": lv.rank}
                        for lv in sorted(taxonomy.levels.values(), key=lambda x: x.rank)
                    ],
                    "rule_count": rule_count,
                    # taxonomy 下定义的分类总数，供前端展示标准分类体系规模。
                    # Total categories defined in the taxonomy, for showing the standard's category scope.
                    "category_count": len(taxonomy.categories),
                }
            )
        return details

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
        1. Parse the Markdown document using StandardProfileGenerator.
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
                # fork-after-warmup：优先复用主进程预加载的适配器（COW 共享模型页）
                preloaded = consume_preloaded_adapter("ner")
                if preloaded is not None and preloaded._model_path == taxonomy.ner_model_path:
                    self._ner_adapter = preloaded
                    logger.info("ner_adapter_reused_preloaded")
                else:
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
                # fork-after-warmup：优先复用主进程预加载的适配器（COW 共享模型页）
                preloaded = consume_preloaded_adapter("llm")
                if preloaded is not None and preloaded._model_path == engine.taxonomy.llm_model_path:
                    self._llm_adapter = preloaded
                    logger.info("llm_adapter_reused_preloaded")
                else:
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