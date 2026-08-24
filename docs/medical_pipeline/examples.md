# 医疗敏感数据全流程治理流水线 — 代码示例指南 (Examples)

> **文档版本**: 1.0  
> **面向对象**: 接入开发者、系统集成工程师

---

## 1. Python SDK 使用示例

可以直接在 Python 代码中导入并使用 `engine.medical_pipeline` 或 `engine.pipeline`：

```python
from engine.medical_pipeline import process_medical_dataset
from engine.pipeline import PipelineService

# 示例 1: 使用 process_medical_dataset 对医疗记录进行双重治理
sample_records = [
    {
        "name": "张伟",
        "id_card_no": "110101199003072381",
        "gender": "男",
        "age": "34",
        "diagnosis_name": "获得性免疫缺陷综合征(HIV)",
        "present_illness": "患者因反复发热就诊，检出HIV抗体阳性",
        "registered_address": "北京市东城区天安门广场1号",
    }
]

output = process_medical_dataset(sample_records)
print("=== 分类分级报告 ===")
# 注意：process_medical_dataset 返回 dataclass（MedicalPipelineResult），
# 使用属性访问而非下标访问
print(output.classification_report)

print("\n=== 脱敏清洗数据 (零泄露) ===")
print(output.sanitized_data)


# 示例 2: 使用 PipelineService 处理 CSV 文件
service = PipelineService(standard="jrt0197")
res = service.process_csv("data/kangyang.csv", mask_l4=True, mask_l5=True)

print(f"\n处理完成，总记录数: {res.classification_summary.total_records}")
print(f"各风险等级分布: {res.classification_summary.level_distribution}")
print(f"L4/L5 高风险字段: {res.classification_summary.high_risk_fields}")
```

---

## 2. cURL / HTTP REST 示例

### 2.1 请求 Agent `/v1/medical/process` 接口

```bash
curl -X POST "http://127.0.0.1:8079/v1/medical/process" \
  -H "Content-Type: application/json" \
  -d '{
    "records": [
      {
        "name": "李娜",
        "id_card_no": "310101199508151247",
        "gender": "女",
        "age": "29",
        "diagnosis_name": "恶性肿瘤(肺腺癌)",
        "present_illness": "咳嗽2月，CT示右肺上叶结节",
        "registered_address": "上海市黄浦区南京东路100号"
      }
    ]
  }'
```

### 2.2 请求 Python / Go 控制台代理端点

```bash
# 请求 Python 控制台后端 (端口 8000)
curl -X POST "http://127.0.0.1:8000/api/medical_pipeline" \
  -H "Content-Type: application/json" \
  -d '{"records": []}'

# 请求 Go 控制台后端 (端口 8080)
curl -X POST "http://127.0.0.1:8080/api/medical_pipeline" \
  -H "Content-Type: application/json" \
  -d '{"records": []}'
```

---

## 3. React 前端集成代码片段

```typescript
import { runMedicalPipeline } from '@/api/client';
import type { MedicalPipelineResponse } from '@/types/api';

async function handleGovernance() {
  try {
    const response: MedicalPipelineResponse = await runMedicalPipeline();
    console.log('分类分级报告:', response.classification_report);
    console.log('脱敏清洗数据:', response.sanitized_data);
    console.log('统计元数据:', response.summary);
  } catch (error) {
    console.error('治理失败:', error);
  }
}
```
