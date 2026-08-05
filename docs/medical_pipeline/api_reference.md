# 医疗敏感数据全流程治理流水线 — API 参考指南

> **文档版本**: 1.0  
> **面向对象**: 接入开发者、后端工程师、前端开发者

---

## 1. Agent REST API

### 1.1 `POST /v1/medical/process`

处理医疗数据集，执行 3-Layer 分类分级与 L4/L5 剥离治理，返回双重输出结构。

- **URL Path**: `/v1/medical/process`
- **Method**: `POST`
- **Headers**: `Content-Type: application/json`

#### Request Body
```json
{
  "records": [
    {
      "name": "张伟",
      "id_card_no": "110101199003072381",
      "gender": "男",
      "age": "34",
      "diagnosis_name": "获得性免疫缺陷综合征(HIV)",
      "present_illness": "患者因反复发热就诊，检出HIV抗体阳性",
      "registered_address": "北京市东城区天安门广场1号"
    }
  ]
}
```

#### Response (200 OK)
```json
{
  "classification_report": [
    {
      "record_index": 0,
      "max_level": "L5",
      "pii_fields_detected": ["id_card_no", "name", "registered_address"],
      "high_sensitivity_detected": ["diagnosis_name:L5", "present_illness:L5"],
      "field_details": [
        {
          "field_name": "id_card_no",
          "level": "L4",
          "security_tag": "PII_ID_CARD",
          "description": "公民身份证号码",
          "rule_matched": "RULE_PII_IDCARD"
        },
        {
          "field_name": "diagnosis_name",
          "level": "L5",
          "security_tag": "CRITICAL_DIAGNOSIS",
          "description": "临床诊断名称",
          "rule_matched": "RULE_L5_HIV"
        }
      ]
    }
  ],
  "sanitized_data": [
    {
      "name": "张*",
      "id_card_no": "110101********2381",
      "gender": "男",
      "age": "34",
      "diagnosis_name": "[L5-IMMUNODEFICIENCY-SENSITIVE-MASKED]",
      "present_illness": "患者因反复发热就诊，检出[L5-IMMUNODEFICIENCY-SENSITIVE-MASKED]",
      "registered_address": "北京市***"
    }
  ],
  "summary": {
    "total_records": 1,
    "l5_records_count": 1,
    "l4_records_count": 0,
    "l3_records_count": 0,
    "l1_l2_records_count": 0,
    "sanitized_pii_fields_per_record": 3,
    "guarantee_no_l4_l5_raw_data": true,
    "duration_ms": 12.5
  }
}
```

---

### 1.2 `POST /v1/pipeline/process_records`

通用分类分级与脱敏流水线端点。

- **URL Path**: `/v1/pipeline/process_records`
- **Method**: `POST`

#### Request Body
```json
{
  "records": [ ... ],
  "standard": "jrt0197",
  "mask_l4": true,
  "mask_l5": true
}
```

#### Response (200 OK)
```json
{
  "classification_summary": {
    "total_records": 1,
    "level_distribution": {"L1": 2, "L2": 0, "L3": 1, "L4": 1, "L5": 1},
    "high_risk_fields": ["diagnosis_name", "id_card_no"],
    "standard_id": "jrt0197",
    "duration_ms": 15.2
  },
  "record_details": [ ... ],
  "masked_records": [ ... ],
  "masking_details": [ ... ]
}
```

---

### 1.3 `POST /v1/pipeline/process_csv`

接受 CSV 文件上传，执行流水线处理。

- **URL Path**: `/v1/pipeline/process_csv`
- **Method**: `POST`
- **Content-Type**: `multipart/form-data`
- **Query Parameters**: `standard=jrt0197&mask_l4=true&mask_l5=true`
- **Form Data**: `file=@data1.csv`

---

## 2. Python SDK (`privacy_local_agent.pipeline`)

```python
from privacy_local_agent.pipeline import PipelineService, PipelineResult

service = PipelineService(standard="jrt0197")

# 1. 字典数组处理
result: PipelineResult = service.process_records(records, mask_l4=True, mask_l5=True)

# 2. CSV 文件处理
result: PipelineResult = service.process_csv("data/data1.csv")
```

---

## 3. 控制台代理 API

### 3.1 Python 后端代理 (`console/backend`)
- `POST /api/medical_pipeline`: 未传 `records` 时自动读取 `console/backend/samples/data1.csv`。
- `POST /api/pipeline/process`: 通用代理端点。

### 3.2 Go 后端代理 (`console/backend-go`)
- `POST /api/medical_pipeline`: 未传 `records` 时自动读取 `console/backend-go/internal/samples/data1.csv`。
- `POST /api/pipeline/process`: 通用代理端点。
