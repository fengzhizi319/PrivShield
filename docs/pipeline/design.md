# 医疗数据分类分级与脱敏流水线设计方案

> Medical Data Classification & Masking Pipeline Design

## 1. 需求概述

构建一条端到端的 **医疗数据分类分级 + 脱敏流水线**，覆盖数据生成、分类分级、脱敏处理、全栈测试控制台集成：

| 阶段 | 目标 | 关键产出 |
|---|---|---|
| ① 数据生成 | 生成 20 条逼真医疗记录 CSV | `data/data1.csv` |
| ② 分类分级 | 调用 `dynclassification` 对每行/每字段分级 | 分级结果（含 L1-L5 等级） |
| ③ 脱敏处理 | 调用 `privacy/masking` 消除 L4/L5 数据 | 脱敏后 CSV + 分级报告 |
| ④ 全栈集成 | Python 后端 + Go 后端 + React 前端联调 | 控制台可视化测试面板 |

---

## 2. 数据模型设计

### 2.1 CSV 字段定义（data1.csv）

共 28 个字段，分为 5 个语义组：

| 语义组 | 字段（中/英） | 预期敏感度 |
|---|---|---|
| **基本信息** | 姓名/name, 性别/gender, 年龄/age | L3-L4 |
| **医疗信息** | 诊断名称/diagnosis_name, 主诉/chief_complaint, 现病史/present_illness, 既往史/past_history, 个人史/personal_history, 是否吸烟/is_smoking, 吸烟时长/smoking_duration, 家族史/family_history, 过敏史/allergic_history, 科室/department | L3-L5 |
| **体征信息** | 身高/height, 体重/weight | L2-L3 |
| **残疾评估** | 残疾类别/disability_category, 残疾等级/disability_level, 评估类型/assess_type_name, 评估结果/assess_result_name, 评估分数/assess_score, 评估时间/assess_time | L3-L4 |
| **病程与身份** | 病程记录/progress_note, 病程记录时间/progress_note_time, 户口地址/registered_address, 身份证号/id_card_no, 残疾证号/disability_cert_no, 医保证号/medical_insurance_no | L4-L5 |

### 2.2 数据生成规则

| 规则 | 说明 |
|---|---|
| 身份证号 | 符合 GB 11643-1999 标准，18 位，末位校验码正确 |
| 残疾证号 | 18-20 位，含地区码 + 残疾类别码 + 序号 |
| 医保证号 | 模拟社保号格式，地区前缀 + 序号 |
| 姓名 | 从常见姓氏 + 名字库随机组合 |
| 地址 | 省/市/区/街道 四级真实地名组合 |
| 病史 | 包含 L4（如详细手术记录）和 L5（如基因检测结果、精神疾病史）级内容 |
| 病例类型 | 20 条中约 3-4 条为图片病例（生成 base64 占位或引用路径），其余为文字病例 |
| 时间字段 | 近 2 年内合理随机日期 |

---

## 3. 架构设计

### 3.1 整体流水线

```
┌──────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│  data1.csv   │────▶│  Classification       │────▶│  Masking Pipeline   │
│  (原始数据)   │     │  (DynClassification)  │     │  (privacy/masking)  │
└──────────────┘     └──────────┬───────────┘     └──────────┬──────────┘
                                │                             │
                    ┌───────────▼───────────┐    ┌───────────▼───────────┐
                    │  分级结果              │    │  脱敏后数据            │
                    │  classification_      │    │  masked_data1.csv     │
                    │  report.json          │    │                       │
                    └───────────────────────┘    └───────────────────────┘
```

### 3.2 模块划分

```
privacy_local_agent/
├── pipeline/                      # 新增：医疗数据流水线模块
│   ├── __init__.py                # 公开 API 导出
│   ├── models.py                  # Pydantic 请求/响应模型
│   ├── classifier.py              # 分类分级封装（调用 dynclassification）
│   ├── masker.py                  # 脱敏处理封装（调用 privacy/masking）
│   ├── service.py                 # PipelineService 编排器
│   └── router.py                  # FastAPI REST 路由
```

### 3.3 核心类设计

#### 3.3.1 PipelineService

```python
class PipelineService:
    """医疗数据分类分级 + 脱敏流水线编排器。"""

    def __init__(
        self,
        rules_dir: str | Path | None = None,
        standard: str = "jrt0197",
        profile_path: str | Path | None = None,
    ): ...

    async def process_csv(
        self,
        csv_path: Path,
        *,
        standard: str | None = None,
        mask_l4: bool = True,
        mask_l5: bool = True,
    ) -> PipelineResult: ...

    async def process_records(
        self,
        records: list[dict],
        *,
        standard: str | None = None,
    ) -> PipelineResult: ...
```

#### 3.3.2 PipelineResult

```python
class FieldClassificationDetail(BaseModel):
    """单字段分级明细。"""
    field_name: str
    field_value: str
    sensitivity_level: str          # L1-L5
    category: str | None = None
    confidence: float = 1.0
    engine_layer: str = "L1_RULE"
    reasoning: str | None = None

class RecordClassificationDetail(BaseModel):
    """单记录分级明细。"""
    record_index: int
    final_level: str
    field_details: list[FieldClassificationDetail]

class PipelineResult(BaseModel):
    """流水线统一输出。"""
    # 分级数据
    classification_summary: ClassificationSummary
    record_details: list[RecordClassificationDetail]

    # 脱敏数据
    masked_records: list[dict]
    masking_details: list[MaskingDetail]

class ClassificationSummary(BaseModel):
    """分级汇总统计。"""
    total_records: int
    level_distribution: dict[str, int]     # {"L1": 5, "L2": 8, "L3": 3, "L4": 3, "L5": 1}
    high_risk_fields: list[str]             # L4/L5 字段名列表
    standard_id: str
    duration_ms: float

class MaskingDetail(BaseModel):
    """脱敏操作明细。"""
    record_index: int
    field_name: str
    original_level: str
    masking_type: str               # FieldType: ID_CARD, NAME, etc.
    original_value: str
    masked_value: str
```

### 3.4 处理流程

```
process_csv(csv_path)
    │
    ├─ 1. 读取 CSV → list[dict]
    │
    ├─ 2. 逐条调用 DynClassificationService.classify_record()
    │     └─ 对每个字段获取 FieldClassificationResult
    │
    ├─ 3. 汇总分级结果，统计 level_distribution
    │
    ├─ 4. 识别 L4/L5 字段，构建脱敏上下文
    │     └─ 根据字段名自动推断 FieldType（id_card→ID_CARD, name→NAME, ...）
    │
    ├─ 5. 调用 masking.mask_record() / mask_value() 对 L4/L5 字段脱敏
    │
    └─ 6. 组装 PipelineResult 返回
```

---

## 4. 数据生成脚本设计

### 4.1 脚本位置与接口

- **路径**: `scripts/generate_medical_data.py`
- **输出**: `data/data1.csv`
- **运行**: `python scripts/generate_medical_data.py`

### 4.2 数据生成策略

| 字段 | 生成策略 |
|---|---|
| 身份证号 | 地区码(6位) + 生日(8位) + 顺序码(3位) + 校验码(1位)，校验算法符合 MOD 11-2 |
| 姓名 | 常见姓氏(李/王/张/刘/陈...) + 双字名库随机组合 |
| 地址 | 省市区三级真实行政区划 + 随机街道/路名/门牌号 |
| 残疾证号 | 地区码(6位) + 残疾类别(2位) + 等级(1位) + 序号(4位) + 校验(1位) |
| 医保证号 | 地区码(6位) + "01" + 序号(8位) |
| 现病史/既往史 | 从模板库随机组合，部分包含 L4(手术细节)/L5(基因检测/精神疾病) 内容 |
| 图片病例 | 3-4 条记录标记 `has_image=true`，生成占位图片描述字段 |
| 诊断名称 | ICD-10 常见诊断编码映射 |
| 科室 | 与诊断名称关联的科室列表 |
| 评估分数 | 根据残疾等级生成合理分数区间 |

---

## 5. REST API 设计

### 5.1 新增路由

前缀: `/v1/pipeline`

| 端点 | 方法 | 说明 |
|---|---|---|
| `/v1/pipeline/process_csv` | POST | 上传 CSV 文件，执行分类分级 + 脱敏 |
| `/v1/pipeline/process_records` | POST | 传入 JSON 记录数组，执行流水线 |

### 5.2 请求/响应示例

**process_csv 请求**: `multipart/form-data`
- `file`: CSV 文件
- `standard`: 分类标准（可选，默认 jrt0197）

**process_csv 响应**:
```json
{
  "classification_summary": {
    "total_records": 20,
    "level_distribution": {"L1": 2, "L2": 5, "L3": 6, "L4": 5, "L5": 2},
    "high_risk_fields": ["id_card_no", "name", "registered_address", "medical_insurance_no"],
    "standard_id": "jrt0197",
    "duration_ms": 156.3
  },
  "record_details": [...],
  "masked_records": [...],
  "masking_details": [...]
}
```

---

## 6. 全栈集成设计

### 6.1 文件部署

```
data/
└── data1.csv                          # 预生成的医疗数据

console/
├── backend/
│   └── app/
│       └── data/                      # Python 后端数据目录
│           └── data1.csv              # 从项目 data/ 复制或符号链接
├── backend-go/
│   └── data/                          # Go 后端数据目录
│       └── data1.csv                  # 从项目 data/ 复制或符号链接
```

### 6.2 Python 后端新增端点

路径: `console/backend/app/main.py`

| 端点 | 方法 | 说明 |
|---|---|---|
| `POST /api/pipeline/process` | POST | 读取后端 data1.csv，调用 agent `/v1/pipeline/process_records`，返回分级 + 脱敏结果 |
| `POST /api/pipeline/upload` | POST | 上传自定义 CSV，执行流水线 |

实现方式：复用现有 `ProxyRequest` 代理模式，转发到 agent 的 `/v1/pipeline/process_records`。

### 6.3 Go 后端新增端点

路径: `console/backend-go/internal/handlers/handlers.go`

| 端点 | 方法 | 说明 |
|---|---|---|
| `POST /api/pipeline/process` | POST | 读取 data1.csv，通过 gRPC/REST 调用 agent 流水线 |
| `POST /api/pipeline/upload` | POST | 上传 CSV 并处理 |

实现方式：
1. 读取 `data/data1.csv` 解析为 JSON 记录数组
2. 通过 `agent.Client` 调用 agent 的 `/v1/pipeline/process_records`
3. 返回统一格式响应

### 6.4 前端新增面板

新增组件: `console/web/src/components/MedicalPipelinePanel.tsx`

功能区域：
1. **操作区**: 「执行 data1.csv 分级脱敏」按钮 + 「上传自定义 CSV」按钮
2. **分级结果区**:
   - 汇总卡片：总记录数、各级别分布饼图、高风险字段列表
   - 明细表格：每条记录每字段的分级结果（颜色编码 L1-L5）
3. **脱敏结果区**:
   - 脱敏后数据表格（可切换原始值/脱敏值对比视图）
   - 脱敏操作日志（哪些字段、从什么级别、做了什么脱敏）
4. **导出区**: 导出分级报告 JSON + 脱敏后 CSV

国际化: 新增 `zh` / `en` 翻译条目于 `console/web/src/i18n/`

---

## 7. 测试设计

### 7.1 单元测试 (`tests/test_pipeline.py`)

| 测试用例 | 验证内容 |
|---|---|
| `test_generate_medical_data` | CSV 生成：行数=20、字段完整、身份证校验正确 |
| `test_pipeline_process_records` | 流水线处理 20 条记录，输出包含分级 + 脱敏两部分 |
| `test_classification_levels` | 分级结果包含 L1-L5 各级别，L4/L5 字段被正确识别 |
| `test_masking_removes_l4_l5` | 脱敏后数据不含原始 L4/L5 值（姓名、身份证、地址等） |
| `test_id_card_masking` | 身份证号脱敏格式正确（保留前3后4） |
| `test_name_masking` | 姓名脱敏格式正确 |
| `test_address_masking` | 地址脱敏格式正确 |
| `test_pipeline_result_model` | PipelineResult 序列化/反序列化正确 |
| `test_empty_records` | 空记录输入不报错，返回空结果 |
| `test_single_record` | 单条记录正确处理 |

### 7.2 数据生成测试 (`tests/test_generate_medical_data.py`)

| 测试用例 | 验证内容 |
|---|---|
| `test_csv_generation` | 脚本可执行，输出文件存在 |
| `test_row_count` | 生成 20 行数据 |
| `test_id_card_checksum` | 所有身份证号校验码正确 |
| `test_required_fields` | 所有必填字段非空 |
| `test_medical_content` | 病史中包含 L4/L5 级内容 |

### 7.3 集成测试

| 测试用例 | 验证内容 |
|---|---|
| `test_rest_pipeline_endpoint` | REST `/v1/pipeline/process_records` 端到端 |
| `test_python_backend_pipeline` | Python 后端 `/api/pipeline/process` 联调 |
| `test_go_backend_pipeline` | Go 后端 `/api/pipeline/process` 联调 |

---

## 8. 文件清单

| 操作 | 路径 | 说明 |
|---|---|---|
| 新增 | `docs/pipeline/design.md` | 本文档 |
| 新增 | `docs/pipeline/prd.md` | 需求文档（引用本文） |
| 新增 | `scripts/generate_medical_data.py` | 医疗数据生成脚本 |
| 新增 | `data/data1.csv` | 生成的 CSV 数据 |
| 新增 | `privacy_local_agent/pipeline/__init__.py` | 模块入口 |
| 新增 | `privacy_local_agent/pipeline/models.py` | 数据模型 |
| 新增 | `privacy_local_agent/pipeline/classifier.py` | 分类分级封装 |
| 新增 | `privacy_local_agent/pipeline/masker.py` | 脱敏处理封装 |
| 新增 | `privacy_local_agent/pipeline/service.py` | PipelineService 编排 |
| 新增 | `privacy_local_agent/pipeline/router.py` | REST 路由 |
| 修改 | `privacy_local_agent/main.py` | 挂载 pipeline 路由 |
| 新增 | `tests/test_pipeline.py` | 流水线单元测试 |
| 新增 | `tests/test_generate_medical_data.py` | 数据生成测试 |
| 修改 | `console/backend/app/main.py` | 新增 pipeline 代理端点 |
| 修改 | `console/backend-go/internal/handlers/handlers.go` | 新增 pipeline 端点 |
| 修改 | `console/backend-go/internal/models/models.go` | 新增 pipeline 数据模型 |
| 新增 | `console/web/src/components/MedicalPipelinePanel.tsx` | 前端面板 |
| 修改 | `console/web/src/App.tsx` | 注册新面板 |
| 修改 | `console/web/src/api/client.ts` | 新增 API 调用 |
| 修改 | `console/web/src/types/api.ts` | 新增类型定义 |
| 修改 | `console/web/src/i18n/zh.ts` | 中文翻译 |
| 修改 | `console/web/src/i18n/en.ts` | 英文翻译 |

---

## 9. 实施顺序

```
Phase 1: 数据层
  ├─ 1a. scripts/generate_medical_data.py
  ├─ 1b. 生成 data/data1.csv
  └─ 1c. tests/test_generate_medical_data.py

Phase 2: 核心流水线
  ├─ 2a. privacy_local_agent/pipeline/ 全部模块
  ├─ 2b. 挂载路由到 main.py
  └─ 2c. tests/test_pipeline.py

Phase 3: 后端集成
  ├─ 3a. console/backend/ Python 端点
  ├─ 3b. console/backend-go/ Go 端点
  └─ 3c. 复制 data1.csv 到后端目录

Phase 4: 前端集成
  ├─ 4a. MedicalPipelinePanel.tsx
  ├─ 4b. API client + 类型定义
  ├─ 4c. 国际化
  └─ 4d. App.tsx 注册
```

---

## 10. 关键设计决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 新模块位置 | `privacy_local_agent/pipeline/` | 独立于现有 `privacy/` 和 `dynclassification/`，避免循环依赖 |
| 分类调用方式 | 封装 `DynClassificationService` | 复用已有三层漏斗能力，不重复实现 |
| 脱敏调用方式 | 封装 `masking.mask_record()` | 利用字段名推断 + 已有脱敏策略 |
| CSV 存放位置 | `data/` 项目根目录 | 独立数据目录，后端通过复制或符号链接获取 |
| 前端面板 | 独立组件 `MedicalPipelinePanel` | 不影响现有面板，可独立演进 |
| 图片病例处理 | 文字描述 + base64 占位 | 避免依赖真实图片资源，测试可控 |
