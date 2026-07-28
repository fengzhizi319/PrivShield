# 动态分类分级使用示例与最佳实践

本文档提供 `privacy-local-agent` 动态分类分级模块的自动配置生成、Python 代码示例、自定义算子扩展及 REST API 调用用例。

---

## 1. 分级标准文档一键自动生成 YAML 配置

无需手动编写复杂的 YAML 语法，输入一份标准 Markdown 规范文档（如《四川省健康医疗大数据应用指南.md》），即可自动抽取并生成全套 Taxonomy、Domain Profile 与 Standard 定义文件。

### 1.1 CLI 命令行方式一键生成

```bash
cd /home/charles/code/sfwork/privacy-local-agent
PYTHONPATH=. python -m privacy_local_agent.dynclassification.generator \
  --doc docs/standard/四川省健康医疗大数据应用指南.md \
  --output rules/
```

运行后自动输出三个标准 YAML 配置文件：
- `rules/taxonomies/sc_health_db51.yaml`
- `rules/domains/sc_health_db51.yaml`
- `rules/standards/sc_health_db51.yaml`

---

### 1.2 Python SDK 自动化生成与热重载

```python
from privacy_local_agent.dynclassification import DynClassificationService

service = DynClassificationService(rules_dir="rules")

# 从标准文档一键生成配置并自动载入引擎缓存
generated_files = service.generate_profile_from_doc("docs/standard/四川省健康医疗大数据应用指南.md")
print("生成并热重载配置文件:", generated_files)

# 立即使用自动生成的标准进行求值
resp = service.classify_field("patient_brca1_gene", "rs80357906", standard="sc_health_db51")
print("最终定级:", resp.field_result.final_level)  # 输出: L5
```

---

## 2. Python SDK 基础用例


### 1.1 加载 Profile 并评估单个字段

```python
from privacy_local_agent.privacy.classification.profile_loader import ProfileLoader

# 初始化加载器（指向规则 YAML 目录）
loader = ProfileLoader(rules_dir="rules")

# 1. 加载金融标准规则引擎 (JR/T 0197)
finance_engine = loader.get_engine(standard="jrt0197")

# 2. 评估字段
tags = finance_engine.evaluate(
    field_name="bank_card_number",
    value="6222021001123456789"
)

for tag in tags:
    print(f"命中规则: {tag.rule_id}, 类别: {tag.category}, 级别: {tag.level}")
# 输出: 命中规则: RULE_FIN_CARD_VALUE, 类别: FINANCIAL_ACCOUNT, 级别: C4
```

---

### 1.2 医疗行业场景评估 (DB51/T 2989)

```python
# 加载医疗标准规则引擎
medical_engine = loader.get_engine(standard="sc_health_db51")

# 评估基因组变异指标字段
tags = medical_engine.evaluate(
    field_name="brca1_mutation",
    value="rs80357906"
)

print(tags)
# [SecurityTag(level='L5', category='GENOMIC', rule_id='RULE_MED_G_001', ...)]
```

---

## 2. 自定义匹配算子扩展示例

当现有的内置算子无法满足特定行业或复杂逻辑校验需求时，可以自定义注册无状态匹配算子：

```python
from typing import Any
from privacy_local_agent.privacy.classification.operator_registry import OperatorRegistry

# 1. 使用装饰器注册自定义算子：中国新能源车牌号校验算子
@OperatorRegistry.register("nev_plate_number")
def nev_plate_matcher(value: Any, params: dict[str, Any]) -> bool:
    import re
    if not isinstance(value, str):
        return False
    # 新能源车牌号规则正则 (D/F)
    pattern = r"^[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤川青藏琼宁][A-Z][DF][A-HJ-NP-Z0-9]\d{4}$"
    return bool(re.match(pattern, value))

# 2. 验证算子已注册
print("nev_plate_number" in OperatorRegistry.list_operators())  # True

# 3. 在 YAML 规则中即可直接指定:
# matchers:
#   - target: "field_value"
#     operator: "nev_plate_number"
```

---

## 3. REST API 调用示例

### 3.1 HTTP curl 请求：评估字段

```bash
curl -X POST http://127.0.0.1:8079/v1/dynclassification/eval \
  -H "Content-Type: application/json" \
  -d '{
    "fieldName": "patient_icd10",
    "value": "B20.0",
    "standard": "sc_health_db51"
  }'
```

#### 预期响应：
```json
{
  "tags": [
    {
      "level": "L4",
      "category": "MEDICAL_ICD10_HIV",
      "ruleId": "RULE_MED_ICD10",
      "sourceEngine": "RULE",
      "domain": "medical",
      "standardId": "sc_health_db51"
    }
  ],
  "maxLevel": "L4"
}
```

---

### 3.2 触发规则配置热加载

在更新了 `rules/` 目录下的 YAML 配置文件后，发起重载 HTTP 请求：

```bash
curl -X POST http://127.0.0.1:8079/v1/dynclassification/profiles/reload
```

#### 预期响应：
```json
{
  "status": "ok",
  "message": "Classification profiles and engines reloaded successfully"
}
```

---

## 4. Python 影子模式（Shadow Mode）对比测试示例

在线上升级或迁移规则包时，开启影子模式对比新旧引擎产出的标签是否一致：

```python
from privacy_local_agent.privacy.classification.profile_loader import ProfileLoader
from privacy_local_agent.privacy.classification.classification_rule_engine import DefaultRuleEngine

loader = ProfileLoader("rules")
new_engine = loader.get_engine(standard="sc_health_db51")
old_engine = DefaultRuleEngine()

field = "brca1_status"
value = "rs123456"

# 新引擎计算
new_tags = new_engine.evaluate(field, value)
# 旧引擎计算
old_tags = old_engine.evaluate(field, value)

new_keys = {(t.level, t.category) for t in new_tags}
old_keys = {(t.level, t.category) for t in old_tags}

if new_keys != old_keys:
    print(f"[警告] 新旧引擎输出结果存在差异！")
    print(f"新引擎结果: {new_keys}")
    print(f"旧引擎结果: {old_keys}")
else:
    print("[成功] 新旧引擎匹配结果完全一致。")
```
