# 动态分类分级（多标准适配）API 参考手册

本文档提供 `privacy-local-agent` 动态分类分级模块的 Python SDK、REST API 以及 gRPC 接口的完整参考指南。

---

## 1. Python SDK 参考

### 1.1 `models.py` - 元数据模型

#### `SensitivityLevelDef`
动态敏感度等级定义模型。

```python
class SensitivityLevelDef(BaseModel):
    id: str                    # 级别标识（如 "L1", "C4", "LEVEL_3"）
    name: str                  # 显示名称（如 "高敏感数据"）
    rank: int                  # 排序权重（用于 max_level 比较）
    description: Optional[str] = None # 等级描述说明
```

#### `CategoryDef`
动态分类类别定义模型。

```python
class CategoryDef(BaseModel):
    id: str                    # 分类 ID（如 "PERSONAL_BASIC"）
    name: str                  # 分类名称（如 "个人基本信息"）
    parent_id: Optional[str] = None # 父分类 ID（支持层级树形结构）
    description: Optional[str] = None
```

#### `DomainTaxonomy`
完整领域分类体系定义模型。

```python
class DomainTaxonomy(BaseModel):
    domain: str                # 领域标识（如 "healthcare", "finance"）
    standard_id: str           # 标准编号（如 "DB51_T_2989", "JR_T_0197"）
    version: str = "1.0.0"     # 版本号
    levels: dict[str, SensitivityLevelDef]
    categories: dict[str, CategoryDef]
    default_level: str = "L3"
    confidence_policy: Optional[ConfidencePolicy] = None  # 置信度策略配置
    ner_entity_mapping: Optional[dict[str, str]] = None   # NER 实体类型→等级 ID 映射
    ner_sensitive_keywords: Optional[list[str]] = None    # NER 敏感关键词列表
    ner_label_mapping: Optional[dict[str, str]] = None    # NER 原始标签→标准标签映射
    ner_model_path: Optional[str] = None                  # NER 模型文件路径
    ner_vocab_path: Optional[str] = None                  # NER 词表文件路径
    llm_model_path: Optional[str] = None                  # LLM 模型目录路径
    llm_arbitration_prompt_template: Optional[str] = None # LLM 仲裁 prompt 模板
    llm_classify_prompt_template: Optional[str] = None    # LLM 分类 system prompt 模板（支持 {domain}/{standard_id}/{levels_desc}）

    def max_level(self, *level_ids: str) -> str:
        """返回给定等级列表中 rank 最高等级的 ID。"""

    def get_category_path(self, category_id: str) -> list[str]:
        """获取指定分类从根到节点的完整路径。"""
```

---

### 1.2 `operator_registry.py` - 算子注册表

#### `OperatorRegistry`
匹配算子单例注册表。

```python
class OperatorRegistry:
    @classmethod
    def register(cls, name: str):
        """算子注册装饰器。"""

    @classmethod
    def register_func(cls, name: str, func: MatcherOperator) -> None:
        """运行时动态注册算子。"""

    @classmethod
    def get(cls, name: str) -> MatcherOperator:
        """获取已注册算子函数，若不存在则抛出 KeyError。"""

    @classmethod
    def list_operators(cls) -> list[str]:
        """获取所有已注册的算子名称列表。"""
```

#### 内置标准算子表 (`operators.py`)

| 算子名称 (`operator`) | 描述 | 参数支持 (`params`) |
|---|---|---|
| `regex` | 正则表达式匹配算子 | `pattern` (str): 正则匹配表达式 |
| `keyword_contains` | 归一化子串包含匹配 | `keywords` (list[str]): 关键词列表；`use_word_boundaries` (bool): 是否使用单词边界匹配（默认 False） |
| `prefix_match` | 前缀匹配 | `prefixes` (list[str]): 前缀字符串列表 |
| `suffix_match` | 后缀匹配 | `suffixes` (list[str]): 后缀字符串列表 |
| `id_card_checksum` | GB 11643 身份证校验码算子 | 无 |
| `medical_card_checksum` | 医保卡号算法算子 | 无 |
| `luhn_checksum` | 银行卡 Luhn 算法校验算子 | `min_length`, `max_length` |
| `icd10_range` | ICD-10 编码区间及级别提升判定 | `default_level`, `upgrade_level`, `intervals` |
| `length_range` | 字符串长度区间匹配算子 | `min_length`, `max_length` |
| `exact_match` | 精确匹配算子 | `values` (list[str]) |
| `ip_address` | IP 地址正则匹配算子 | 无 |
| `mac_address` | MAC 地址匹配算子 | 无 |
| `chinese_name` | 中文姓名校验匹配算子 | 无 |

---

### 1.3 `engine.py` - 通用规则引擎

#### `ConfigurableRuleEngine`

```python
class ConfigurableRuleEngine:
    def __init__(self, taxonomy: DomainTaxonomy, profiles: list[RuleProfile], domain: str = "", standard_id: str = ""):
        """根据给定的元数据体系和规则包列表初始化引擎。"""

    def evaluate(
        self, field_name: str, value: Any, context: dict[str, Any] | None = None
    ) -> tuple[list[SecurityTag], list[SecurityTag]]:
        """评估单个字段，返回 (final_tags, suppressed_tags) 元组。
        
        final_tags: 最终生效的安全标签列表
        suppressed_tags: 被降级规则压制的标签列表（用于审计）
        """
```

---

### 1.4 `profile_loader.py` - 配置加载与管理

#### `ProfileLoader`

```python
class ProfileLoader:
    def __init__(self, rules_dir: str | Path = "rules"):
        """初始化 ProfileLoader。"""

    def get_engine(
        self, domain: Optional[str] = None, standard: Optional[str] = None
    ) -> ConfigurableRuleEngine:
        """根据 domain 或 standard 获取或构建配置化引擎实例。"""

    def invalidate_cache(self) -> None:
        """清除缓存，实现配置热重载。"""
```

---

## 2. REST API 接口定义

### 2.1 动态分类求值接口
- **Endpoint**: `POST /v1/dynclassification/eval`
- **Content-Type**: `application/json`

#### 请求体格式
```json
{
  "fieldName": "user_id_card",
  "value": "510104199003072345",
  "domain": "general-pii",
  "standard": "gbt35273"
}
```

#### 响应体格式
```json
{
  "fieldResult": {
    "fieldName": "user_id_card",
    "fieldValue": "510104199003072345",
    "tags": [
      {
        "level": "L3",
        "category": "PERSONAL_BASIC",
        "confidence": 1.0,
        "sourceEngine": "RULE",
        "ruleId": "RULE_PII_IDCARD",
        "domain": "general-pii",
        "standardId": "gbt35273",
        "matchTarget": "field_value"
      }
    ],
    "finalLevel": "L3",
    "confidence": 1.0,
    "needsHumanReview": false,
    "engineLayer": "L1_RULE",
    "reasoning": "命中规则: RULE_PII_IDCARD"
  },
  "auditInfo": {
    "domain": "general-pii",
    "standardId": "gbt35273",
    "ruleSetVersion": "1.0.0",
    "rulesEvaluated": 12,
    "rulesHit": 1,
    "durationMs": 0.235
  }
}
```

---

### 2.2 规则配置热加载接口
- **Endpoint**: `POST /v1/dynclassification/profiles/reload`

#### 响应体格式
```json
{
  "status": "ok",
  "message": "Classification profiles and engines reloaded successfully"
}
```

---

### 2.3 获取可用的标准列表
- **Endpoint**: `GET /v1/dynclassification/standards`

#### 响应体格式
```json
{
  "standards": [
    {
      "standard_id": "sc_health_db51",
      "description": "DB51/T 2989—2023 四川省健康医疗大数据应用指南",
      "taxonomy": "default",
      "domains": ["general-pii", "medical"]
    },
    {
      "standard_id": "jrt0197",
      "description": "JR/T 0197-2020 金融数据安全分级指南",
      "taxonomy": "finance_jrt0197",
      "domains": ["general-pii", "finance"]
    }
  ]
}
```

---

### 2.4 获取可用匹配算子列表
- **Endpoint**: `GET /v1/dynclassification/operators`

#### 响应体格式
```json
{
  "operators": [
    "regex",
    "keyword_contains",
    "prefix_match",
    "suffix_match",
    "id_card_checksum",
    "medical_card_checksum",
    "luhn_checksum",
    "icd10_range",
    "length_range",
    "exact_match",
    "ip_address",
    "mac_address",
    "chinese_name"
  ]
}
```

---

## 3. gRPC 协议声明

在 `proto/privacy.proto` 中定义动态分类请求与响应结构：

```protobuf
message DynClassificationRequest {
  string field_name = 1;
  string field_value = 2;
  string domain = 3;
  string standard = 4;
}

message DynSecurityTagProto {
  string level = 1;
  string category = 2;
  string rule_id = 3;
  string source_engine = 4;
  string domain = 5;
  string standard_id = 6;
}

message DynClassificationResponse {
  repeated DynSecurityTagProto tags = 1;
  string max_level = 2;
  string audit_timestamp = 3;
  string engine_layer = 4;
  double confidence = 5;
  bool needs_human_review = 6;
}
```

gRPC RPC 方法：
```protobuf
rpc DynClassify (DynClassificationRequest) returns (DynClassificationResponse);
```

