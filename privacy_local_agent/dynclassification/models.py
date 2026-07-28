"""动态分类分级数据模型 / Dynamic Classification Data Models.

定义分类体系元数据（Taxonomy）、敏感度等级、分类类别、安全标签等核心模型。
所有模型均为 Pydantic v2 BaseModel，支持 YAML/JSON 序列化与校验。

本模块完全独立于旧分类引擎（privacy/classification/），无交叉依赖。
"""

from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Optional

from pydantic import BaseModel, ConfigDict, Field


# ===========================================================================
# 分类体系元数据模型 / Taxonomy Metadata Models
# ===========================================================================


class SensitivityLevelDef(BaseModel):
    """动态敏感度等级定义。

    替代硬编码的 SensitivityLevel 枚举，支持任意等级体系。
    不同行业可使用不同等级标识（L1~L5 / C1~C4 / 1~4级）。
    """

    model_config = ConfigDict(populate_by_name=True)

    id: str = Field(description="级别唯一标识，如 'L1', 'C4', 'LEVEL_3'")
    name: str = Field(description="显示名称，如 '高敏感数据'")
    rank: int = Field(description="排序权重，数值越大越敏感（用于 max_level 比较）")
    description: Optional[str] = Field(default=None, description="等级说明")


class CategoryDef(BaseModel):
    """动态分类类别定义。

    替代硬编码的 BusinessCategory 枚举，支持多级分类树结构。
    通过 parent_id 建立父子关系。
    """

    model_config = ConfigDict(populate_by_name=True)

    id: str = Field(description="分类 ID，如 'PERSONAL_BASIC', 'FINANCIAL_ACCOUNT'")
    name: str = Field(description="分类名称，如 '个人基本信息'")
    parent_id: Optional[str] = Field(default=None, description="父分类 ID，支持多级树结构")
    description: Optional[str] = Field(default=None, description="分类说明")


class DomainTaxonomy(BaseModel):
    """领域分类体系完整定义。

    一个 Taxonomy 对应一个行业标准的分类分级元数据，
    包含该标准下的所有等级定义和分类目录树。
    """

    model_config = ConfigDict(populate_by_name=True)

    domain: str = Field(description="领域标识，如 'healthcare', 'finance', 'gov'")
    standard_id: str = Field(description="标准编号，如 'DB51_T_2989', 'JR_T_0197'")
    version: str = Field(default="1.0.0", description="体系版本号")
    description: Optional[str] = Field(default=None, description="体系说明")
    levels: dict[str, SensitivityLevelDef] = Field(
        default_factory=dict, description="等级 ID → 等级定义的映射"
    )
    categories: dict[str, CategoryDef] = Field(
        default_factory=dict, description="分类 ID → 分类定义的映射"
    )
    default_level: str = Field(default="L3", description="无规则命中时的默认等级 ID")

    def max_level(self, *level_ids: str) -> str:
        """返回等级集合中 rank 最高的等级 ID。

        Args:
            *level_ids: 一个或多个等级 ID。

        Returns:
            rank 最大的等级 ID；无有效输入时返回 default_level。
        """
        if not level_ids:
            return self.default_level
        valid = [lid for lid in level_ids if lid in self.levels]
        if not valid:
            return self.default_level
        return max(valid, key=lambda lid: self.levels[lid].rank)

    def get_level_rank(self, level_id: str) -> int:
        """获取等级的排序权重。

        Args:
            level_id: 等级 ID。

        Returns:
            rank 值；未找到时返回 0。
        """
        if level_id in self.levels:
            return self.levels[level_id].rank
        return 0


    def get_category_path(self, category_id: str) -> list[str]:
        """获取分类的完整路径（从根到叶）。

        Args:
            category_id: 分类 ID。

        Returns:
            从根分类到目标分类的 ID 列表。
        """
        path: list[str] = []
        current: Optional[str] = category_id
        visited: set[str] = set()
        while current and current in self.categories and current not in visited:
            visited.add(current)
            path.append(current)
            current = self.categories[current].parent_id
        return list(reversed(path))


# ===========================================================================
# 安全标签与分类结果 / Security Tag & Classification Results
# ===========================================================================


class SecurityTag(BaseModel):
    """安全标签，描述单次规则命中的分类结果。

    每次规则/算子命中都会产出一个 SecurityTag，记录命中的等级、类别、
    来源引擎、规则 ID 等审计信息。
    """

    model_config = ConfigDict(populate_by_name=True)

    level: str = Field(description="敏感度等级 ID（如 'L3', 'C4'）")
    category: str = Field(description="分类类别 ID（如 'PII_ID_CARD', 'GENOMIC'）")
    confidence: float = Field(default=1.0, ge=0.0, le=1.0, description="置信度 [0,1]")
    source_engine: str = Field(default="RULE", alias="sourceEngine", description="来源引擎标识")
    rule_id: str = Field(default="", alias="ruleId", description="触发的规则 ID")
    domain: str = Field(default="", description="所属领域")
    standard_id: str = Field(default="", alias="standardId", description="所属标准")
    version: str = Field(default="1.0.0", description="标签版本")
    needs_human_review: bool = Field(default=False, alias="needsHumanReview", description="是否需人工复核")

    def __str__(self) -> str:
        return f"{self.level}_{self.category}"


class FieldClassificationResult(BaseModel):
    """单个字段的分类结果。"""

    model_config = ConfigDict(populate_by_name=True)

    field_name: str = Field(alias="fieldName", description="字段名称")
    field_value: Optional[str] = Field(default=None, alias="fieldValue", description="字段示例值")
    tags: list[SecurityTag] = Field(default_factory=list, description="命中的安全标签列表")
    final_level: str = Field(alias="finalLevel", description="最终裁定的敏感度等级")
    confidence: float = Field(default=0.0, ge=0.0, le=1.0, description="综合置信度")
    needs_human_review: bool = Field(default=False, alias="needsHumanReview")


class RecordClassificationResult(BaseModel):
    """单条记录（多字段）的分类结果。"""

    model_config = ConfigDict(populate_by_name=True)

    record_index: int = Field(default=0, alias="recordIndex")
    field_results: dict[str, FieldClassificationResult] = Field(
        default_factory=dict, alias="fieldResults"
    )
    aggregated_tags: list[SecurityTag] = Field(default_factory=list, alias="aggregatedTags")
    final_level: str = Field(alias="finalLevel", description="记录级最终等级")
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    needs_human_review: bool = Field(default=False, alias="needsHumanReview")


class TableClassificationResult(BaseModel):
    """整张表/批次的分类结果。"""

    model_config = ConfigDict(populate_by_name=True)

    schema_: list[str] = Field(default_factory=list, alias="schema")
    record_results: list[RecordClassificationResult] = Field(
        default_factory=list, alias="recordResults"
    )
    aggregated_tags: list[SecurityTag] = Field(default_factory=list, alias="aggregatedTags")
    final_level: str = Field(alias="finalLevel")
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    needs_human_review: bool = Field(default=False, alias="needsHumanReview")


class AuditInfo(BaseModel):
    """审计信息，记录分类请求的执行元数据。"""

    model_config = ConfigDict(populate_by_name=True)

    version: str = "1.0.0"
    domain: str = ""
    standard_id: str = Field(default="", alias="standardId")
    timestamp: str = Field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    rule_set_version: str = Field(default="1.0.0", alias="ruleSetVersion")
    rules_evaluated: int = Field(default=0, alias="rulesEvaluated")
    rules_hit: int = Field(default=0, alias="rulesHit")
    duration_ms: float = Field(default=0.0, alias="durationMs")


class ClassificationResponse(BaseModel):
    """分类响应包装器。"""

    model_config = ConfigDict(populate_by_name=True)

    field_result: Optional[FieldClassificationResult] = Field(default=None, alias="fieldResult")
    record_result: Optional[RecordClassificationResult] = Field(default=None, alias="recordResult")
    table_result: Optional[TableClassificationResult] = Field(default=None, alias="tableResult")
    audit_info: AuditInfo = Field(default_factory=AuditInfo, alias="auditInfo")
