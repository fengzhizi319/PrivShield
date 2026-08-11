"""医疗数据分类分级与脱敏 Pipeline 核心实现模块。
Core implementation of Medical Data Classification & Desensitization Pipeline.

中文说明：
本模块实现「医疗敏感数据全流程治理 Pipeline」——对医疗数据集中的每条记录，
逐字段执行如下闭环流程：

  1. 动态分类分级：集成 dynclassification 3 层漏斗（Rule -> NER -> LLM）识别字段风险等级（L1~L5）；
  2. 身份与特高风险脱敏：抹平 PII 身份信息、强剥离 L4/L5 高风险病史词汇；
  3. 输出双结构数据：分级报告（classification_report）+ 合规脱敏数据（sanitized_data）。

核心执行入口为 `MedicalPrivacyPipeline.process_records()`，
其中每个字段的处理由 `_classify_field()`（分级）与 `_sanitize_field()`（脱敏）协同完成，
并通过双层缓存（sanitized 缓存 + NER 缓存）避免同一字段被重复推理。

English Description:
Core implementation of the Medical Data Classification & Desensitization Pipeline.
For each record in a medical dataset, every field goes through a closed-loop flow:
dynamic classification (3-layer funnel: Rule -> NER -> LLM), PII / high-risk
redaction (L4/L5), and dual-output (classification report + sanitized data).
"""

# === 导入区 / Imports ===
# 启用 PEP 563 延迟注解求值，允许在类型注解中引用尚未定义的类（如自引用）
from __future__ import annotations

import re  # 正则表达式引擎，用于 L4/L5 模式匹配与中文连续字符检测
import threading  # 线程锁，保护共享缓存（sanitized 缓存 / NER 缓存）的并发安全
import time  # 性能计时（perf_counter），用于统计 Pipeline 整体耗时
from dataclasses import asdict, dataclass, field  # dataclass 定义与报告转 dict（asdict）
from typing import Any, Optional  # 泛型类型标注

# 动态分类服务：3 层漏斗（Rule -> NER -> LLM）的统一分类引擎入口
from privacy_local_agent.dynclassification import DynClassificationService
# 图像病例打码能力：识别图像输入（文件路径 / Base64 Data URI）并执行图像级打码
from privacy_local_agent.dynclassification.image_redaction import (
    IMAGE_REDACTION_FAILURE,  # 图像打码失败时的固定返回标记（fail-closed 安全策略）
    is_image_input,           # 判断输入是否为图像（路径或以 data:image 开头的 Base64）
)
# 通用字段名感知脱敏原语：按字段名（id_card_no/name/address 等）执行强掩码
from privacy_local_agent.privacy.masking import mask_value

# 医疗领域规则集：
# - PII_FIELD_RULES: PII 身份字段名 -> 脱敏规则名 的映射表
# - L4_PATTERNS / L5_PATTERNS: 高风险病史词汇正则（L5 为极高敏，L4 为高敏）
# - redact_medical_text: 规则引擎文本抹平（纯正则，零依赖快速路径）
# - redact_medical_text_with_ner: Small-NER 引擎文本抹平（深度学习推理路径）
from .rules import (
    PII_FIELD_RULES,
    canonicalize_pii_field,
    ICD10_FIELD_NAMES,
    DATE_GENERALIZATION_FIELDS,
    L4_PATTERNS,
    L5_PATTERNS,
    RedactionStrategyConfig,
    classify_icd10_code,
    compile_l4_l5_patterns,
    contains_high_risk_text,
    load_redaction_strategy,
    normalize_fullwidth_alphanumeric,
    redact_icd10_code,
    redact_medical_text,
    redact_medical_text_with_ner,
    truncate_date_to_month,
)

# 图像打码失败标记的模块级别名：
# 全流程中统一以 IMAGE_FAILURE 判断图像字段是否脱敏失败（fail-closed：失败即标记，绝不返回原图）
IMAGE_FAILURE = IMAGE_REDACTION_FAILURE

# NER 抹平缓存大小上限：防止长文本高并发下无界内存增长。
# dict 保持插入序，超出上限时按 FIFO 淘汰最旧条目（见 _cache_ner_result）
_NER_CACHE_MAX_SIZE = 2048

# sanitized 缓存大小上限：与 NER 缓存相同的 FIFO 有界策略，
# 防止长驻实例在图像字段/sanitize=False 等路径下无界增长（内存耗尽防护）
_SANITIZED_CACHE_MAX_SIZE = 2048

_SUPPORTED_REDACT_MODES = frozenset({"redact", "mask"})
_SUPPORTED_REDACT_ENGINES = frozenset({"ner", "rule"})

# 枚举型分类字段集合：取值来自封闭枚举（如科室、医疗类别），不是自由文本。
# 对这类字段做 L4/L5 词库子串扫描会产生误伤（如科室"皮肤性病科"含"性病"被定级 L4
# 并篡改为"皮肤科"），因此豁免自由文本高敏词扫描与抹平触发。
_CATEGORICAL_FIELDS = frozenset({
    "department", "dept", "dept_name", "admission_dept", "discharge_dept",
})


def _mask_string(field_name: str, value: str) -> str:
    """调用 masking 并稳定返回字符串，屏蔽其可选详情返回类型。

    内部工具函数：mask_value 在 return_details=False 时可能返回纯字符串，
    也可能返回带 .value 属性的对象（取决于 masking 版本），此处统一收敛为 str。
    """
    result = mask_value(field_name, value, return_details=False)
    # 双形态兼容：str 直接返回；对象则取其 .value 字段
    if isinstance(result, str):
        return result
    return result.value


# === 数据模型区 / Data Models ===


@dataclass
class FieldClassification:
    """单字段分类分级结果模型。

    process_records 中每个字段都会生成一个该结构的实例，
    记录字段名、最终等级、命中的安全标签与规则，以及原始值/脱敏值的三套快照。
    """

    field_name: str  # 字段名（如 chief_complaint / id_card_no）
    level: str  # 风险等级：L1, L2, L3, L4, L5（L4/L5 为高敏，需强抹平）
    security_tag: str  # 安全标签（如 PII_IDENTITY / HIGH_RISK_MEDICAL_L5），用于下游统计与告警
    description: str  # 人类可读的字段语义描述
    rule_matched: str  # 命中的规则名（如 MEDICAL_L5_STRICT_RULE），用于审计追溯
    raw_value: str = ""  # 原始输入值快照（process_records 循环内回填）
    sanitized_value: str = ""  # 最终对外脱敏值（与 sanitized_data 中该字段一致）
    sanitized_value_rule: str = ""  # 纯规则引擎（正则）抹平结果，供对比分析
    sanitized_value_ner: str = ""  # Small-NER 引擎抹平结果，供对比分析（无推理时等于规则结果）


@dataclass
class RecordClassificationReport:
    """单条记录的分级报告模型（asdict 后进入 classification_report 列表）。"""

    record_index: int  # 记录序号（从 1 开始，对应 enumerate(start=1)）
    max_level: str  # 该记录所有字段中的最高等级（记录级风险取 max）
    pii_fields_detected: list[str]  # 检测到的 PII 身份字段名列表
    high_sensitivity_detected: list[str]  # 检测到的高敏字段及其等级（L4/L5 级风险）
    field_details: list[FieldClassification]  # 字段级明细（完整分级/脱敏过程记录）
    raw_record: Optional[dict[str, str]] = field(default=None)  # 原始记录快照（可关闭以省内存）


@dataclass
class MedicalPipelineResult:
    """Pipeline 最终双结构输出模型。

    - classification_report: 分级报告（每条记录一份，含字段明细）
    - sanitized_data: 合规脱敏数据（与输入记录一一对应、字段结构一致）
    - raw_data: 原始数据透传（与 sanitized_data 配套对照）
    - summary: 统计汇总（各等级记录数、失败数、耗时等）
    """

    classification_report: list[dict[str, Any]]  # 分级报告列表（RecordClassificationReport 的 dict 形态）
    sanitized_data: list[dict[str, str]]  # 脱敏后的合规数据集
    raw_data: list[dict[str, str]]  # 原始数据集（仅透传引用，不复制）
    summary: dict[str, Any]  # 汇总统计信息（等级计数/失败数/耗时/合规保证标记）


# === 可选依赖惰性加载 / Optional Lazy Import ===
# Small-NER 适配器（ONNXRuntime/ModelScope/TensorRT 后端）属于重依赖，
# 采用 try/except 惰性导入：环境中缺失时 NerAdapter 置为 None，
# Pipeline 仍可降级为纯规则引擎工作，保证核心镜像（--target core）可正常启动。
try:
    from ..dynclassification.ner_adapter import NerAdapter
except ImportError:  # NER 依赖缺失 → 记录并降级（后续 redact_medical_text_with_ner 内部同样容错）
    NerAdapter = None


class MedicalPrivacyPipeline:
    """医疗敏感数据全流程治理 Pipeline。
    
    1. 动态分类分级：集成 dynclassification 3层漏斗 (Rule -> NER -> LLM) 识别 27 个字段及临床文本中的 L1~L5 风险标识；
    2. 身份与特高风险脱敏：抹平 PII 信息，强剥离 L4/L5 高风险病史词汇；支持 "ner" (Layer-2 Small-NER) 与 "rule" 双引擎抹平模式；
    3. 输出双结构数据：(1) 分级报告 (2) 合规脱敏数据。

    执行逻辑总览（数据流）：

    process_records() ──逐记录、逐字段──> _classify_field()   [分级：PII 拦截 → L5/L4 正则扫描 → 医疗规则映射 → 兜底 L1]
                                              │
                                              ├──> dyn_service.classify_field()  [3 层漏斗动态分类，结果缓存进 _sanitized_cache]
                                              │
                                              ▼
                                         _sanitize_field()   [脱敏：图像打码 → 复用缓存 → PII 强掩码 → 文本抹平 → 最终门禁]
                                              │
                                              ▼
                                RecordClassificationReport + sanitized_rec  [双输出]

    线程安全：
    - _sanitized_cache / _ner_cache 均为进程内共享状态，所有读写都经过 self._lock 保护，
      且两个缓存均有 FIFO 容量上限（防无界内存增长）；
    - NER 推理采用双检模式：锁内查/写缓存、锁外执行推理，长耗时推理不阻塞并发线程，
      相同文本重复出现时 0ms 响应。
    """

    def __init__(
        self,
        dyn_service: DynClassificationService | None = None,
        redact_engine: str = "ner",
        redaction_strategy: RedactionStrategyConfig | None = None,
    ):
        """初始化 Pipeline 引擎，挂载 DynClassificationService 统一分类能力与 Small-NER 抹平引擎。

        Args:
            dyn_service: 动态分类服务实例；为 None 时自动创建，并注入医疗文本脱敏回调。
            redact_engine: 文本抹平引擎选择："ner"（Small-NER 推理）或 "rule"（纯正则快速路径）。
            redaction_strategy: 脱敏治理策略配置；为 None 时自动从 YAML 加载（回退到代码默认值）。
        """
        if redact_engine not in _SUPPORTED_REDACT_ENGINES:
            supported = ", ".join(sorted(_SUPPORTED_REDACT_ENGINES))
            raise ValueError(
                f"Unsupported redact_engine {redact_engine!r}; expected one of: {supported}"
            )
        # 加载脱敏治理策略（YAML 可配置 → 代码默认值回退）
        self.redaction_strategy = redaction_strategy or load_redaction_strategy()
        # 根据策略编译 L4/L5 正则模式（替换标签可由 YAML 自定义）
        self._l5_patterns, self._l4_patterns = compile_l4_l5_patterns(
            l5_replacement_map=self.redaction_strategy.l5_replacement_map or None,
            l4_replacement_map=self.redaction_strategy.l4_replacement_map or None,
        )
        if dyn_service is None:
            # 创建 DynClassificationService 并注入医疗领域文本脱敏回调
            # 解耦 dynclassification 对 medical_pipeline 的反向依赖
            # （回调引用 pipeline 的方法，而 pipeline 持有 dyn_service，形成闭环但不产生包级循环 import）
            dyn_service = DynClassificationService(text_sanitizer=self._medical_text_sanitizer)
        self.dyn_service = dyn_service  # 挂载分类引擎（调用方也可注入自定义实例以替换默认漏斗）
        self.redact_engine = redact_engine  # 抹平引擎模式："ner" / "rule"

        # 将医疗领域的脱敏回调注册到通用策略注册表 (DomainStrategyRegistry) 中
        from ..dynclassification import default_domain_registry
        default_domain_registry.register_sanitizer("medical", self._medical_text_sanitizer)
        # 惰性初始化 Small-NER 适配器；依赖缺失时为 None（纯规则降级运行）
        self.ner_adapter = NerAdapter() if NerAdapter is not None else None
        self._lock = threading.Lock()  # 全局互斥锁：保护下方两个共享缓存的并发读写
        # 缓存 _classify_field 中 dyn_service 计算的 sanitized_value，供 _sanitize_field 复用，
        # 避免同一字段被三层漏斗分类两次（性能优化）。
        # key = (字段名, 原始字符串值)，value = dyn_service 智能抹平结果
        self._sanitized_cache: dict[tuple[str, str], str] = {}
        # NER 抹平结果缓存（受 _NER_CACHE_MAX_SIZE 上限约束，防止无界内存增长）
        # key = 原始文本，value = NER 抹平后的文本；相同文本复用推理结果
        self._ner_cache: dict[str, str] = {}

    def _cache_ner_result(self, text: str, sanitized: str) -> None:
        """写入 NER 抹平缓存；超过上限时淘汰最旧条目（dict 保持插入序）。

        实现细节：Python dict 保持键的插入顺序，next(iter(dict)) 取到的是最早插入的键，
        因此超出 _NER_CACHE_MAX_SIZE 时弹出最旧条目即等价于 FIFO 淘汰。
        """
        if len(self._ner_cache) >= _NER_CACHE_MAX_SIZE:
            # 淘汰最旧条目：先取首个键再弹出；
            # StopIteration/KeyError 双保护应对极端并发下的竞态（理论上锁内不会发生，双保险）
            try:
                self._ner_cache.pop(next(iter(self._ner_cache)))
            except (StopIteration, KeyError):
                pass
        # 写入新条目（若键已存在则覆盖，同时刷新为"最新"插入位置）
        self._ner_cache[text] = sanitized

    def _cache_sanitized_result(self, cache_key: tuple[str, str], sanitized: str) -> None:
        """写入 sanitized 缓存；超过 _SANITIZED_CACHE_MAX_SIZE 时按 FIFO 淘汰最旧条目。

        与 _cache_ner_result 相同的有界策略：调用方必须持有 self._lock。
        """
        if len(self._sanitized_cache) >= _SANITIZED_CACHE_MAX_SIZE:
            try:
                self._sanitized_cache.pop(next(iter(self._sanitized_cache)))
            except (StopIteration, KeyError):
                pass
        self._sanitized_cache[cache_key] = sanitized

    def _medical_text_sanitizer(self, field_name: str, text: str, final_level: str, mode: str = "redact") -> str:
        """医疗领域文本脱敏回调（注入到 DynClassificationService）。

        支持双引擎抹平模式：
        - redact_engine=="ner": 启用 Layer-2 Small-NER (ONNXRuntime/ModelScope/TensorRT) 实体识别无痕抹平
        - redact_engine=="rule": 采用高级规则引擎抹平模式

        默认彻底擦除 L4/L5 敏感病史与关联句法介词，不留任何形如 [L4-xxx] 的提示性标志。
        若 mode=="mask" 则回退为显式标签掩码模式。
        """
        if mode not in _SUPPORTED_REDACT_MODES:
            supported = ", ".join(sorted(_SUPPORTED_REDACT_MODES))
            raise ValueError(f"Unsupported sanitization mode {mode!r}; expected one of: {supported}")

        # 枚举型分类字段（科室等）不做自由文本抹平：封闭枚举值的子串命中属于误伤
        # （如"皮肤性病科"含"性病"被改成"皮肤科"），原样返回
        if field_name.strip().lower() in _CATEGORICAL_FIELDS:
            return text

        # ── 分支一：mask 模式（显式标签掩码）──
        # 场景：上层请求保留可读性的"打标签式"脱敏（如 [L4-TUMOR-MASKED]），
        # 而非默认的无痕抹平（不留任何提示性标志）。
        if mode == "mask":
            sanitized_text: str = text
            # 依次应用 L5（极高敏）→ L4（高敏）正则替换，将命中词汇替换为带等级标签的占位符
            for pat, replacement in self._l5_patterns:
                sanitized_text = pat.sub(replacement, sanitized_text)
            for pat, replacement in self._l4_patterns:
                sanitized_text = pat.sub(replacement, sanitized_text)
        # ── 分支二：redact 模式（无痕抹平，默认）──
        else:
            # 仅当显式选择 "ner" 引擎时才尝试深度 NER 推理；否则统一走纯正则快速路径
            if self.redact_engine == "ner":
                # 临床自由文本字段白名单：这些字段几乎必然包含病史实体，值得做深度推理
                clinical_keys = {
                    "chief_complaint", "present_illness", "past_history",
                    "personal_history", "family_history", "allergic_history",
                    "progress_note", "diagnosis_name",
                }
                # 双重筛选才触发 NER 推理（成本控制）：
                # 1) 字段属于临床文本 或 文本中仍有未抹平的高风险词汇；
                # 2) 文本长度/含中文字符数满足阈值（_could_benefit_from_ner）。
                if (field_name in clinical_keys or self._contains_high_risk_text(text)) and self._could_benefit_from_ner(text):
                    # 双检模式：锁内仅做缓存查/写，NER 推理（百毫秒~秒级）在锁外执行，
                    # 避免长耗时推理阻塞其他线程的缓存访问（并发吞吐）。
                    # 极端情况下两个线程可能对同一文本重复推理一次，结果等价，无害。
                    with self._lock:
                        cached_ner = self._ner_cache.get(text)
                    if cached_ner is not None:
                        # 缓存命中：相同文本直接复用上次 NER 推理结果（0ms 响应）
                        sanitized_text = cached_ner
                    else:
                        # 缓存未命中 → 锁外执行 Small-NER 推理抹平，再持锁写回缓存
                        sanitized_text = redact_medical_text_with_ner(text, ner_adapter=self.ner_adapter, strategy=self.redaction_strategy)
                        with self._lock:
                            self._cache_ner_result(text, sanitized_text)
                else:
                    # 不满足深度推理条件 → 降级为纯规则引擎（正则）抹平
                    sanitized_text = redact_medical_text(text, strategy=self.redaction_strategy)
            else:
                # "rule" 引擎：始终使用纯正则快速路径
                sanitized_text = redact_medical_text(text, strategy=self.redaction_strategy)

            # 语义清洗：诊断字段若被抹平成残缺的修饰词（如"慢性"），
            # 说明其主体（病名）已被抹除，保留修饰词无意义且易泄露上下文 → 整体置空
            if field_name in ["diagnosis_name", "diagnosis"]:
                sanitized_text = self._purge_diagnosis_residual(field_name, text, sanitized_text)

        # 收尾：仅对明确的个人信息字段应用 PII 掩码（字段名命中 PII_FIELD_RULES），
        # 确保身份类信息走统一的强掩码规则而非文本抹平逻辑
        canonical_field = canonicalize_pii_field(field_name)
        if canonical_field in PII_FIELD_RULES:
            return _mask_string(canonical_field, sanitized_text)

        return sanitized_text

    def _classify_field(self, key: str, val: str) -> FieldClassification:
        """单字段分类分级评估算法（优先调度 dynclassification 动态分类引擎）。
        
        算法流程：
        1. 类型安全转换：在 None 时置为空串，保留 0 和 False 等合法数据；
        2. 调度 dynclassification 引擎评估该字段；
        3. PII 身份规则拦截：若列名命中 PII 词库，根据 GB 11643 标准设定 ID Card 为 L4，其余为 L3；
        4. 病史文本深度匹配：扫描 L5 (极高敏: HIV/重度精神障碍/遗传缺陷) 与 L4 (高敏: 肿瘤/性病/乙肝/衰竭) 词库；
        5. 普通临床与评估字段映射：根据医疗标准规范赋予 L3 (主诉/病史) 与 L2 (健康评估/个人史)；
        6. 综合 dynclassification 与医疗规则取最高敏等级。
        """
        # 前置：类型安全转换。None → 空串（避免后续正则/NER 崩溃），
        # 但 0/False 等合法数据仍转为 "0"/"False" 保留（不丢失数据语义）
        val_str = "" if val is None else str(val)
        
        # ── 第 0 步：调度 dynclassification 动态引擎（3 层漏斗）获取通用/领域分类结果 ──
        # 优化: 同时请求 sanitize=True，将计算出的 sanitized_value 缓存进 _sanitized_cache，
        # 供稍后 _sanitize_field 直接复用——同一字段只跑一次漏斗、出两份结果（分级 + 脱敏）。
        dyn_level: str | None = None
        try:
            dyn_resp = self.dyn_service.classify_field(key, val_str, sanitize=True)
            if dyn_resp and dyn_resp.field_result:
                dyn_level = dyn_resp.field_result.final_level
                # 缓存 dyn_service 智能抹平结果（仅缓存确实发生变换的值，避免无意义写入）
                sanitized_value = dyn_resp.field_result.sanitized_value
                if isinstance(sanitized_value, str) and sanitized_value != val_str:
                    with self._lock:
                        self._cache_sanitized_result((key, val_str), sanitized_value)
        except Exception:
            # 漏斗异常（如 LLM 未加载/超时）时静默降级：dyn_level 保持 None，由本地规则接管
            dyn_level = None

        # ── 步骤 1: PII 身份字段检测与分级（最高优先级，最先拦截）──
        # 依据：字段名命中 PII_FIELD_RULES 即按身份信息定级；
        # 身份证号（唯一国家法定证件号）按 GB 11643 相关要求定为 L4，其余（姓名/地址等）为 L3。
        canonical_key = canonicalize_pii_field(key)
        if canonical_key in PII_FIELD_RULES:
            level = "L4" if canonical_key == "id_card_no" else "L3"
            return FieldClassification(
                field_name=key,
                level=level,
                security_tag="PII_IDENTITY",
                description=f"个人身份标识信息 ({key})",
                rule_matched=f"PII_RULE_{PII_FIELD_RULES[canonical_key]}",
            )

        # ── 步骤 1.5: ICD-10 诊断编码字段定级（§9 规约：高危编码段 L4/L5）──
        # 诊断名称抹平后编码本身仍泄露病种（如 B20.900=HIV、C34.900=肺恶性肿瘤），
        # 因此编码字段按 ICD-10 章节码段独立定级，先于自由文本词库扫描。
        if key.strip().lower() in ICD10_FIELD_NAMES:
            icd_result = classify_icd10_code(val_str)
            if icd_result is not None:
                icd_level, icd_category = icd_result
                return FieldClassification(
                    field_name=key,
                    level=icd_level,
                    security_tag=f"HIGH_RISK_MEDICAL_{icd_level}",
                    description=f"ICD-10 {icd_level} 高危诊断编码 ({icd_category})",
                    rule_matched=f"ICD10_{icd_level}_RULE",
                )

        # ── 步骤 2: 病史文本 L5/L4 术语扫描（正则词库匹配，最高等级优先）──
        # 设计要点：先扫 L5、再扫 L4。L5 已是全表最高级，命中即提前中断（短路优化，避免多余正则开销）；
        # 未命中 L5 才继续扫 L4（L4 命中同样中断）。
        # 枚举型分类字段（科室等）豁免扫描：封闭枚举值的子串命中属于误伤（如"皮肤性病科"含"性病"）。
        detected_level: str | None = None

        if key.strip().lower() not in _CATEGORICAL_FIELDS:
            for pat, _replacement in self._l5_patterns:
                if pat.search(val_str):
                    detected_level = "L5"
                    break  # L5 已是最高级，中断循环

            if detected_level is None:
                for pat, _replacement in self._l4_patterns:
                    if pat.search(val_str):
                        detected_level = "L4"
                        break  # 已找到 L4

        # ── 步骤 2.5: 融合 dynclassification 动态分类引擎的定级结果 ──
        # 本地正则未命中，但 3 层漏斗（Rule->NER->LLM）识别出 L4/L5 时同样采纳，
        # 覆盖词库外的同义/变体表达（如 LLM 理解出的"转移癌"等复杂语义）。
        # 枚举型分类字段同样豁免（避免漏斗对封闭枚举值的误判抬高记录等级）。
        if detected_level is None and key.strip().lower() not in _CATEGORICAL_FIELDS and dyn_level in ["L4", "L5"]:
            detected_level = dyn_level

        # 依据最终等级构造对应 FieldClassification（L5 > L4 优先返回）
        if detected_level == "L5":
            return FieldClassification(
                field_name=key,
                level="L5",
                security_tag="HIGH_RISK_MEDICAL_L5",
                description="极高风险病史/诊断信息 (L5: 重度精神障碍/HIV/重大遗传缺陷)",
                rule_matched="MEDICAL_L5_STRICT_RULE",
            )
        if detected_level == "L4":
            return FieldClassification(
                field_name=key,
                level="L4",
                security_tag="HIGH_RISK_MEDICAL_L4",
                description="高风险病史/诊断信息 (L4: 恶性肿瘤/性病传染病/重度衰竭)",
                rule_matched="MEDICAL_L4_STRICT_RULE",
            )

        # ── 步骤 3: 普通临床与评估字段按医疗标准规范映射 ──
        # 未命中高敏词库时，按字段语义定级：
        # - L3: 临床病史、问诊主诉与诊断名称（主诉/既往史/家族史/过敏史/诊断名称）——敏感度较高；
        # - L2: 健康与残疾评估信息（残疾类别/等级/评估结果/个人史）、入院病情、
        #       非高危诊断编码与完整精度日期准标识符——敏感度中等。
        if key in ["chief_complaint", "past_history", "family_history", "allergic_history", "diagnosis_name"]:
            return FieldClassification(
                field_name=key,
                level="L3",
                security_tag="CLINICAL_HISTORY",
                description="临床病史、问诊主诉与诊断信息",
                rule_matched="CLINICAL_TEXT_RULE",
            )

        if key in ["disability_category", "disability_level", "assess_result_name", "personal_history", "admission_condition"]:
            return FieldClassification(
                field_name=key,
                level="L2",
                security_tag="HEALTH_ASSESSMENT",
                description="健康与残疾评估信息",
                rule_matched="ASSESSMENT_RULE",
            )

        # 良性 ICD-10 编码（高危编码已在步骤 1.5 拦截）与完整精度日期准标识符
        if key.strip().lower() in ICD10_FIELD_NAMES or key.strip() in DATE_GENERALIZATION_FIELDS or key.strip().lower() in DATE_GENERALIZATION_FIELDS:
            return FieldClassification(
                field_name=key,
                level="L2",
                security_tag="QUASI_IDENTIFIER",
                description="诊断编码/日期准标识符信息",
                rule_matched="QUASI_IDENTIFIER_RULE",
            )

        # ── 步骤 4: 通用 L1 级兜底 ──
        # 以上均未命中：视为普通健康/人口学统计信息（如年龄、性别、地区编码等），定级 L1
        return FieldClassification(
            field_name=key,
            level="L1",
            security_tag="GENERAL_INFO",
            description="普通健康/人口学统计信息",
            rule_matched="DEFAULT_L1_RULE",
        )

    def sanitize_text(self, text: str) -> str:
        """剥离与替换文本中的所有 L4/L5 敏感病史术语，保障无 L4/L5 原始词汇泄露。

        纯正则快速路径（零依赖、无推理开销），是 _sanitize_field 的备用降级方案，
        也是公开 API 中无缓存逻辑时的兜底抹平手段。
        """
        if not text:
            return text  # 空文本直接返回，避免空值进入正则管道
        sanitized = text
        # 先 L5（极高敏）后 L4（高敏）逐模式替换；
        # 顺序不可交换：L5 模式更具体，先替换可避免 L4 模式提前吞掉上下文
        for pat, replacement in self._l5_patterns:
            sanitized = pat.sub(replacement, sanitized)
        for pat, replacement in self._l4_patterns:
            sanitized = pat.sub(replacement, sanitized)
        return sanitized

    def _purge_diagnosis_residual(self, field_name: str, original: str, sanitized: str) -> str:
        """诊断名称字段的残余整值抹平（§9 规约：L4/L5 诊断彻底抹平）。

        诊断名称是结构化诊断标签而非叙事文本：高敏词抹平后残留的修饰碎片
        （如 "确诊"、"伴滴度阳性"、"升结肠"）既无医学意义，又会泄露上下文
        （部位/检查手段可反推病种），因此原文含高敏词且值已被改写时整值置空。
        良性诊断（未含高敏词、未被改写）原样保留，不影响数据可用性。
        """
        if field_name not in ("diagnosis_name", "diagnosis"):
            return sanitized
        residual = sanitized.strip()
        if residual in ("慢性", "既往", "既往慢性"):
            return ""
        if residual and residual != original.strip() and self._contains_high_risk_text(original):
            return ""
        return sanitized

    def _contains_high_risk_text(self, text: str) -> bool:
        """判断文本是否仍包含未抹平的 L4/L5 术语。

        双重用途：
        1. 作为 NER 深度推理的触发条件之一（文本中确实有高敏词才值得推理）；
        2. 作为最终门禁——process_records 中脱敏后仍命中则整值删除（见最终门禁逻辑）。

        使用实例级 L4/L5 模式（含自定义替换标签），委托模块级函数执行三级检测。
        前置词库首字符预筛：不含任何词库首字符的文本直接判否（毫秒级短路）。
        """
        return contains_high_risk_text(
            text,
            patterns=self._l5_patterns + self._l4_patterns,
        )

    @staticmethod
    def _could_benefit_from_ner(text: str) -> bool:
        """快速筛选过滤：仅当文本包含至少 2 个连续汉字且长度 >= 2 时才触发深度 NER 推理。

        成本控制前提：NER 是深度学习前向推理（毫秒~秒级），而纯正则只需微秒级。
        对短文本/纯数字/英文短串直接跳过推理，防止高并发下推理吞吐被打满。
        """
        if not text or len(text.strip()) < 2:
            return False  # 空文本或去除空白后不足 4 字符 → 无推理价值
        # 至少包含 2 个连续汉字才可能含中文病史实体（英文/纯符号文本交给规则引擎）
        return bool(re.search(r"[\u4e00-\u9fa5]{2,}", text))

    def _mask_pii_value(self, key: str, val_str: str) -> str:
        """PII 身份字段统一脱敏（提取公共逻辑，避免重复代码）。"""
        canonical_key = canonicalize_pii_field(key)
        if canonical_key == "id_card_no":
            return _mask_string("id_card_no", val_str)
        if canonical_key == "name":
            return _mask_string("name", val_str)
        if canonical_key == "registered_address":
            return _mask_string("address", val_str)
        if canonical_key == "person_id":
            # 人员唯一标识：保留标识前缀与末 4 位（如 PID****1234），维持记录关联能力
            if len(val_str) > 7:
                return val_str[:3] + "****" + val_str[-4:]
            return "****"
        if canonical_key == "hospital_code":
            # 定点医疗机构编码：保留前 5 位机构区划前缀，尾部掩码（如 H1101****）
            if len(val_str) > 5:
                return val_str[:5] + "****"
            return "****"
        if canonical_key in ["disability_cert_no", "medical_insurance_no"]:
            # 证件号/医保号：本地首尾保留格式掩码——
            # 长度 >6 时保留前 4 后 2、中间星号填充；短串直接全掩码
            if len(val_str) > 6:
                return val_str[:4] + "*" * (len(val_str) - 6) + val_str[-2:]
            return "****"
        return val_str  # 未列出的 PII 字段原样返回（保持扩展性，后续规则可在此追加）

    def sanitize_field(self, key: str, val: str) -> str:
        """字段智能抹平脱敏（公开 API，向后兼容）。

        注意：在 process_records 循环中使用 _sanitize_field 代替，
        因为 _sanitize_field 可复用 _classify_field 的缓存，避免重复调用 dyn_service。
        """
        # 公开 API 需要先执行分类以填充缓存（单次调用同时产出分级结果与脱敏值）
        fc = self._classify_field(key, val)
        return self._sanitize_field(key, val, level_hint=fc.level)

    def _sanitize_field(self, key: str, val: str, level_hint: str | None = None) -> str:
        """字段智能抹平脱敏（供 process_records 内部使用）。

        执行优先级（自上而下短路返回）：
        0. 图像输入 → 图像打码（fail-closed，失败返回 IMAGE_FAILURE 标记）；
        1. 复用 _classify_field 的 dyn_service 脱敏缓存（避免二次漏斗调用）；
        2. PII 字段 → 强掩码（不信任漏斗的弱脱敏）；
        3. 临床文本/高敏词 → 纯正则强剥离 L4/L5；
        4. 其余低敏字段 → 原样返回。

        Args:
            key: 字段名。
            val: 字段原始值。
            level_hint: 调用方已计算的分级结果（如 process_records 传入 fc.level），
                传入后条件 (c) 直接复用该等级，避免对同一字段重入 _classify_field
                造成二次完整漏斗推理（NER/LLM 成本翻倍）。
        """
        val_str = "" if val is None else str(val)  # 类型安全转换（与 _classify_field 保持一致）

        # 0. 图像病例检测：文件路径或 Base64 Data URI → 调用图像打码（fail-closed）
        # fail-closed 策略：打码失败绝不返回原图，而是返回固定失败标记供上层计数/告警
        if is_image_input(val_str):
            # 图像分支提前返回：先消费 _classify_field 阶段可能写入的 sanitized 缓存项，
            # 否则该条目永久残留（缓存膨胀）
            with self._lock:
                self._sanitized_cache.pop((key, val_str), None)
            try:
                from privacy_local_agent.dynclassification.image_redaction import sanitize_image_input
                return sanitize_image_input(val_str)  # 执行图像级打码（区域遮盖/人脸模糊）
            except Exception:
                return IMAGE_FAILURE  # 任何异常 → 固定失败标记（绝不泄露原图）

        # 1. 优先复用 _classify_field 中 dyn_service 已计算的 sanitized_value
        # 注意 pop 而非 peek：缓存是"一次性"的——每个字段只消费一次，防脏读也防缓存膨胀
        cache_key = (key, val_str)
        cached: str | None = None
        with self._lock:
            if cache_key in self._sanitized_cache:
                cached = self._sanitized_cache.pop(cache_key)

        if cached is not None:
            # PII 字段保持强掩码规则（dyn_service 的 sanitize 可能不够强，
            # 例如姓名只做了部分抹平而未按身份证规则掩码）
            if canonicalize_pii_field(key) in PII_FIELD_RULES:
                return self._mask_pii_value(key, val_str)
            # 枚举型分类字段不信任漏斗的文本改写（封闭枚举值抹平属误伤），原样返回
            if key.strip().lower() in _CATEGORICAL_FIELDS:
                return val_str
            return cached  # 非 PII 字段直接信任漏斗结果（已含 NER/LLM 智能抹平）

        # 2. PII 字段始终使用强掩码（缓存未命中时兜底，确保身份信息绝不裸奔）
        if canonicalize_pii_field(key) in PII_FIELD_RULES:
            return self._mask_pii_value(key, val_str)

        # 2.5 ICD-10 诊断编码字段：按章节码段脱敏（L5 整值抹平、L4 替换范畴码），
        # 防止诊断名称抹平后编码本身泄露病种（如 B20.900=HIV）
        if key.strip().lower() in ICD10_FIELD_NAMES:
            redacted_code = redact_icd10_code(val_str)
            if redacted_code != val_str:
                return redacted_code

        # 2.6 日期准标识符字段：完整精度日期截断为年月（§9 规约 L2 泛化）
        if key.strip() in DATE_GENERALIZATION_FIELDS or key.strip().lower() in DATE_GENERALIZATION_FIELDS:
            return truncate_date_to_month(val_str)

        # 3. 备用降级：文本强剥离 L4/L5 术语（纯正则快速路径）
        clinical_keys = {
            "diagnosis_name", "chief_complaint", "present_illness",
            "past_history", "personal_history", "family_history",
            "allergic_history", "progress_note", "icd10_code", "admission_condition",
        }
        # 三个触发条件满足其一即抹平：
        # (a) 字段属临床自由文本（病史/诊断类）；
        # (b) 文本中检测到高敏词（不依赖字段名——未知字段里的敏感词同样必须抹平；
        #     枚举型分类字段如科室豁免，避免"皮肤性病科"子串误伤）；
        # (c) 分级为 L4/L5（优先复用调用方传入的 level_hint，避免重入分类造成二次漏斗推理）。
        if (
            key in clinical_keys
            or canonicalize_pii_field(key) in clinical_keys
            or (key.strip().lower() not in _CATEGORICAL_FIELDS and self._contains_high_risk_text(val_str))
            or (level_hint if level_hint is not None else self._classify_field(key, val_str).level) in ["L4", "L5"]
        ):
            return self._purge_diagnosis_residual(key, val_str, self.sanitize_text(val_str))

        # 4. 低敏/普通字段：不满足任何抹平条件 → 原样返回（避免过度脱敏破坏数据可用性）
        return val_str

    def process_records(
        self, records: list[dict[str, str]], sanitize: bool = True
    ) -> MedicalPipelineResult:
        """处理医疗数据集记录并生成双输出。
        
        Args:
            records: 输入医疗记录列表。
            sanitize: 是否进行高敏与 PII 脱敏（默认 True）。若为 True，在单次推断/循环中同时完成分级与脱敏。
        """
        start_time = time.perf_counter()  # 高精度计时起点（统计整体耗时）
        
        # 输出容器：报告列表（每条记录一份）与脱敏记录列表（与输入一一对应）
        reports: list[dict[str, Any]] = []
        sanitized_records: list[dict[str, str]] = []
        
        # 各等级记录计数器（summary 汇总用）
        l5_count = 0
        l4_count = 0
        l3_count = 0
        redaction_failures = 0  # 图像打码失败计数（影响合规保证标记）
        fail_safe_triggered = 0  # 最终门禁触发计数（规则+NER 未抹净、被门禁整值删除的字段数）
        pii_fields_total = 0  # 实际检出并掩码的 PII 字段总数（summary 实测统计用）

        # ── 外层循环：逐条记录处理 ──
        for idx, rec in enumerate(records, start=1):
            # 每条记录独立的中间状态：字段明细列表、脱敏记录、PII/高敏字段名、记录级最高等级
            field_classifications: list[FieldClassification] = []
            sanitized_rec: dict[str, str] = {}
            
            rec_pii: list[str] = []
            rec_high_risk: list[str] = []
            max_level = "L1"  # 记录级最高等级，初始为最低级 L1
            
            # 兼容 L1~L5 与 C1~C5 双重等级体系防 crash
            # （旧版接口/下游可能返回 C 前缀等级，统一映射为数字序数参与比较）
            level_rank = {
                "L1": 1, "L2": 2, "L3": 3, "L4": 4, "L5": 5,
                "C1": 1, "C2": 2, "C3": 3, "C4": 4, "C5": 5,
            }
            
            # ── 内层循环：逐字段分类 + 脱敏（单次遍历同时完成两件事）──
            for key, val in rec.items():
                val_str = "" if val is None else str(val)  # 类型安全转换
                # ① 分级：调用 _classify_field（内部已调用 dyn 漏斗并填充 sanitized 缓存）
                fc = self._classify_field(key, val_str)
                field_classifications.append(fc)
                
                # 收集记录级风险信息：
                # - PII_IDENTITY 标签 → 记入 pii_fields_detected
                # - L4/L5（含 C4/C5 旧体系）→ 记入 high_sensitivity_detected
                if fc.security_tag == "PII_IDENTITY":
                    rec_pii.append(key)
                if fc.level in ["L4", "L5", "C4", "C5"]:
                    rec_high_risk.append(f"{key}:{fc.level}")
                    
                # 更新记录级最高等级（用数字序数比较，L 与 C 体系可混用）
                fc_rank = level_rank.get(fc.level, 1)
                max_rank = level_rank.get(max_level, 1)
                if fc_rank > max_rank:
                    max_level = fc.level
                    
                # ② 脱敏：使用 _sanitize_field 复用 _classify_field 的 dyn_service 结果
                #（单次调用优化——_classify_field 已填充缓存并产出 fc.level，
                #  _sanitize_field 直接消费缓存与等级提示，不重复跑漏斗）
                if sanitize:
                    sanitized_rec[key] = self._sanitize_field(key, val_str, level_hint=fc.level)
                    if sanitized_rec[key] == IMAGE_FAILURE:
                        redaction_failures += 1  # 图像打码失败计数（不中断流程，仅记录）
                    elif key.strip().lower() not in _CATEGORICAL_FIELDS and self._contains_high_risk_text(sanitized_rec[key]):
                        # 最终门禁：任何漏网的高敏文本整体删除，不能返回部分原文。
                        # （覆盖规则引擎与 NER 都未抹净的极端场景，保证"零 L4/L5 原文泄露"；
                        #  枚举型分类字段豁免——"皮肤性病科"等封闭枚举值的子串命中属误伤）
                        sanitized_rec[key] = "[L4-L5-DATA-REMOVED]"
                        fail_safe_triggered += 1  # 门禁触发计数（反映规则+NER 双引擎未抹净的字段数）
                else:
                    sanitized_rec[key] = val_str  # sanitize=False：原样透传（仅做分级报告）

                # 回填字段级明细快照（原始值 + 最终对外脱敏值）
                fc.raw_value = val_str
                fc.sanitized_value = sanitized_rec[key]

                # ③ 生成双引擎对比快照（供审计/调优分析，不影响对外脱敏值）：
                if canonicalize_pii_field(key) in PII_FIELD_RULES:
                    # PII 字段：rule 引擎与 ner 引擎均记录强掩码结果（PII 不走文本抹平）
                    fc.sanitized_value_rule = self._mask_pii_value(key, val_str)
                    fc.sanitized_value_ner = fc.sanitized_value_rule
                else:
                    # 非 PII 字段：
                    # 性能超级优化：仅对临床长文本字段/高危敏感字段触发深度 Small-NER 前向推理，
                    # 结构化/短文本字段直接复用超快规则管道，结合 LRU 缓存秒级响应
                    fc.sanitized_value_rule = redact_medical_text(val_str, strategy=self.redaction_strategy)
                    clinical_keys = {
                        "chief_complaint", "present_illness", "past_history",
                        "personal_history", "family_history", "allergic_history",
                        "progress_note", "diagnosis_name",
                    }
                    # 触发条件与 _medical_text_sanitizer 一致：临床字段或含高敏词，且文本值得推理
                    if (key in clinical_keys or self._contains_high_risk_text(val_str)) and self._could_benefit_from_ner(val_str):
                        # 带内存缓存的 NER 抹平（相同文本 0ms 闪电响应）；
                        # 双检模式：锁内仅查/写缓存，推理在锁外执行（不阻塞并发线程的缓存访问）
                        with self._lock:
                            ner_cached = self._ner_cache.get(val_str)
                        if ner_cached is not None:
                            fc.sanitized_value_ner = ner_cached  # 缓存命中：直接复用
                        else:
                            # 缓存未命中：锁外执行 NER 推理，再持锁写回缓存
                            ner_res = redact_medical_text_with_ner(val_str, ner_adapter=self.ner_adapter, strategy=self.redaction_strategy)
                            with self._lock:
                                self._cache_ner_result(val_str, ner_res)
                            fc.sanitized_value_ner = ner_res
                    else:
                        # 不值得推理：ner 快照直接等于规则结果（保证字段快照非空）
                        fc.sanitized_value_ner = fc.sanitized_value_rule

            # ── 记录级统计：按最高等级计数 ──
            pii_fields_total += len(rec_pii)  # 累加实际检出/掩码的 PII 字段数
            if max_level == "L5":
                l5_count += 1
            elif max_level == "L4":
                l4_count += 1
            elif max_level == "L3":
                l3_count += 1
            # L1/L2 不单独计数，由 summary 中总数差值计算

            # 组装并输出该记录的双结构结果（报告转 dict 存入 reports，脱敏记录存入 sanitized_records）
            rep = RecordClassificationReport(
                record_index=idx,
                max_level=max_level,
                pii_fields_detected=rec_pii,
                high_sensitivity_detected=rec_high_risk,
                field_details=field_classifications,
                raw_record=rec,
            )
            reports.append(asdict(rep))
            sanitized_records.append(sanitized_rec)

        # ── 汇总统计：耗时 + 各等级计数 + 合规保证标记 ──
        # sanitize=False 时 _sanitize_field 未被调用，分类阶段写入的 sanitized 缓存
        # 无人消费——统一清空，防止长驻实例缓存只写不读导致的膨胀（另有 FIFO 上限兜底）
        if not sanitize:
            with self._lock:
                self._sanitized_cache.clear()

        # 输出回扫验证（实测而非自报）：对全部脱敏字段执行三级高敏词检测
        # （含全角/插字符变体），任何字段仍命中则合规保证标记为 False
        leaked_fields = 0
        if sanitize:
            for rec_out in sanitized_records:
                for out_key, out_val in rec_out.items():
                    if out_key.strip().lower() in _CATEGORICAL_FIELDS:
                        continue  # 枚举型分类字段不参与高敏词回扫（子串误伤豁免）
                    if out_val in (IMAGE_FAILURE, "[L4-L5-DATA-REMOVED]"):
                        continue
                    if self._contains_high_risk_text(out_val):
                        leaked_fields += 1

        elapsed_ms = (time.perf_counter() - start_time) * 1000.0

        summary = {
            "total_records": len(records),
            "l5_records_count": l5_count,
            "l4_records_count": l4_count,
            "l3_records_count": l3_count,
            "l1_l2_records_count": len(records) - l5_count - l4_count - l3_count,
            # 实测统计：实际检出并掩码的 PII 字段总数与记录均值（不再硬编码词表大小）
            "sanitized_pii_fields_total": pii_fields_total if sanitize else 0,
            "sanitized_pii_fields_per_record": round(pii_fields_total / len(records), 2) if (sanitize and records) else 0,
            "redaction_failures": redaction_failures,
            "fail_safe_triggered_fields": fail_safe_triggered,
            # 合规保证：开启脱敏 + 零打码失败 + 输出全量回扫零泄露（实测验证而非自报）
            "guarantee_no_l4_l5_raw_data": bool(sanitize and redaction_failures == 0 and leaked_fields == 0),
            "duration_ms": round(elapsed_ms, 2),
        }

        return MedicalPipelineResult(
            classification_report=reports,  # 输出 1：分级报告
            sanitized_data=sanitized_records,  # 输出 2：合规脱敏数据
            raw_data=records,  # 原始数据透传（便于对照校验）
            summary=summary,
        )


def process_medical_dataset(
    records: list[dict[str, str]], sanitize: bool = True
) -> MedicalPipelineResult:
    """高层入口：处理医疗数据集并返回分类分级报告与脱敏清洗数据。

    便捷函数：内部自动创建默认配置的 MedicalPrivacyPipeline 实例（
    默认挂载 DynClassificationService + Small-NER 抹平引擎），
    然后委托 process_records 执行完整的「逐字段分级 → 脱敏 → 双输出」闭环。

    Args:
        records: 输入医疗记录列表（每条记录为 字段名 -> 值 的字典）。
        sanitize: 是否执行高敏与 PII 脱敏（默认 True）；False 时仅输出分级报告。

    Returns:
        MedicalPipelineResult：包含分级报告、脱敏数据、原始数据与汇总统计。
    """
    # 创建默认引擎实例（分类 + NER 抹平）并执行主流程
    return MedicalPrivacyPipeline().process_records(records, sanitize=sanitize)
