# 医疗敏感数据全流程治理流水线 — 测试指南 (Testing)

> **文档版本**: 1.0  
> **面向对象**: QA 工程师、测试开发工程师、安全合规审计人员

---

## 1. 测试策略与用例设计

测试矩阵覆盖 4 个核心维度：

| 测试维度 | 验证内容 | 测试用例文件 | 状态 |
|---|---|---|---|
| **1. 校验码合法性** | 身份证号符合 GB 11643-1999 (ISO 7064:1983.MOD 11-2) 模 11-2 校验 | `tests/test_medical_pipeline.py::test_generate_valid_id_card_checksum` | ✅ PASS |
| **2. 字段规范与数量** | 模拟数据完整包含 27 个标准医疗与 PII 字段（仓库预置 `kangyang.csv` 为 100 条） | `tests/test_medical_pipeline.py::test_generated_dataset_fields_count` | ✅ PASS |
| **3. 零泄露与强剥离** | 经治理后的 `sanitized_data` 绝对不含 HIV、恶性肿瘤、重度精神障碍等原始词 | `tests/test_medical_pipeline.py::test_medical_privacy_pipeline_no_raw_l4_l5_leak` | ✅ PASS |
| **4. 双重结构契约** | `classification_report` 与 `sanitized_data` 格式与结构完全符合定义 | `tests/test_medical_pipeline.py::test_medical_privacy_pipeline_dual_output` | ✅ PASS |
| **5. 通用 Pipeline** | `PipelineService` 的 `process_records`、`process_csv` 及 `/v1/pipeline/*` 端点 | `tests/test_pipeline.py` | ✅ PASS |
| **6. ReDoS 防护** | 含敏感词触发完整句法管线后，长空白/干扰串必须在线性时间内完成 | `tests/test_medical_pipeline.py::test_redos_catastrophic_backtracking_prevention` | ✅ PASS |
| **7. 变体绕过防护** | 全角/字符打散/英文病名/同义词变体全数捕获脱敏 | `tests/test_medical_pipeline.py::test_all_33_sanitization_variant_bypasses` | ✅ PASS |
| **8. 四柱强剥离覆盖** | 单药/抗精神病药/肝硬化体征群/CD4 计数等强关联特征探针 | `tests/test_medical_pipeline.py::test_four_pillar_16_probes_coverage` | ✅ PASS |
| **9. 规范案例对账** | 标准规范 8 个临床案例的精确输出锚定 | `tests/test_medical_pipeline.py::test_all_8_specification_cases_exact_match` | ✅ PASS |

---

## 2. 自动化测试命令

### 2.1 运行医疗 Pipeline 测试集

```bash
cd /path/to/PrivShield
PYTHONPATH=. pytest tests/test_medical_pipeline.py -v
```

### 2.2 运行通用 Pipeline 测试集

```bash
PYTHONPATH=. pytest tests/test_pipeline.py -v
```

### 2.3 运行控制台 Python 后端测试

```bash
pytest console/backend/tests -v
```

### 2.4 运行控制台 Go 后端测试

```bash
cd console/backend-go
go test -v ./...
```

---

## 3. 关键断言代码范例

### 3.1 身份证 ISO 7064 校验码断言
```python
def test_generate_valid_id_card_checksum() -> None:
    id_card = gen_id_card()
    assert len(id_card) == 18
    weights = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
    checksum_map = ['1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2']
    total = sum(int(id_card[i]) * weights[i] for i in range(17))
    expected_check = checksum_map[total % 11]
    assert id_card[-1].upper() == expected_check
```

### 3.2 L4/L5 敏感字符串零泄露断言
```python
def test_medical_privacy_pipeline_no_raw_l4_l5_leak() -> None:
    records = generate_dataset(20)
    res = process_medical_dataset(records)
    forbidden_terms = ["HIV", "艾滋", "获得性免疫缺陷", "恶性肿瘤", "精神分裂症"]

    # 注意：process_medical_dataset 返回 dataclass（MedicalPipelineResult），
    # 使用属性访问而非下标访问
    for row in res.sanitized_data:
        for k, v in row.items():
            for term in forbidden_terms:
                assert term not in str(v), f"泄露高危词汇 '{term}' 于字段 {k}: {v}"
```
