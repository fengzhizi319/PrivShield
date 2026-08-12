# 医疗敏感数据全流程治理流水线 (Medical Privacy Pipeline) 文档索引

本目录包含 `privacy-local-agent` 医疗敏感数据分类分级与脱敏流水线模块的全套 SDLC 文档。

---

## 目录 (Table of Contents)

- [文档清单](#文档清单)
- [模块概览](#模块概览)
- [快速开始](#快速开始)

---

## 文档清单

| 文档 | 说明 | 目标读者 |
|---|---|---|
| [prd.md](./prd.md) | 医疗敏感数据治理流水线产品需求文档 (PRD) | 产品经理、数据合规人员、架构师 |
| [design.md](./design.md) | 3-Layer 分类分级、L4/L5 脱敏剥离与双输出架构设计方案 | 算法工程师、Agent 开发者 |
| [api_reference.md](./api_reference.md) | Agent REST / Python SDK / Dual-Backend API 接口参考 | 接入开发者、前端开发 |
| [ops.md](./ops.md) | 数据生成、脚本使用、运维配置与排障指南 | SRE、运维工程师、测试人员 |
| [testing.md](./testing.md) | 校验码测试、L4/L5 零泄露测试与测试用例指南 | QA、测试工程师 |
| [examples.md](./examples.md) | Python 端到端示例、cURL / HTTP REST 示例与前端调用代码 | 接入开发者 |

---

## 模块概览

`medical_pipeline` 是 `privacy-local-agent` 的端到端数据合规治理能力示范模块，主要解决医疗健康场景下涉及 27 个高敏感与 PII 字段的强合规共享问题：

1. **高仿真数据生成 (`scripts/data/generate_medical_data.py`)**: 支持 GB 11643-1999 校验码算法的 18 位身份证、图文病历引用及 L4/L5 级重症/传染病/精神障碍病史数据生成。
2. **3-Layer 分类分级 (L1~L5)**: 结合 `dynclassification` 规则引擎与语义 Funnel，精准识别个人身份标识 (PII) 与特高敏感病史。
3. **L4/L5 强剥离与 PII 掩码**: 对身份证、姓名、医保卡执行格式保频掩码，对恶性肿瘤、HIV、重度精神障碍等 L4/L5 诊断自动转换为合规范畴标签，**100% 承诺输出清洗数据不含原始高危词汇**。
4. **双重输出结构**: 同步输出 (1) 分类分级元数据报告 (`classification_report`) 和 (2) 安全脱敏清洗数据 (`sanitized_data`)。
5. **全栈控制台打通**: 接入 Agent REST 端点，打通 Python & Go 双代理后端以及 Web React 控制台 UI (`MedicalPipelinePanel.tsx`)。

---

## 快速开始

1. **阅读需求文档**: 查看 [prd.md](./prd.md) 了解业务场景与敏感等级分级定义。
2. **理解架构设计**: 阅读 [design.md](./design.md) 掌握算法编排与双重输出逻辑。
3. **运行数据生成**: 执行 `python scripts/data/generate_medical_data.py --output data/kangyang.csv --count 100` 生成 100 条模拟数据（与仓库预置样例一致；脚本默认 20 条）。
4. **代码示例参考**: 查看 [examples.md](./examples.md) 运行 Python SDK 示例脚本或发送 cURL HTTP 请求。
5. **API 详细契约**: 查阅 [api_reference.md](./api_reference.md) 获取接口模型。
6. **运行单元测试**: 参阅 [testing.md](./testing.md) 执行 `PYTHONPATH=. pytest tests/test_medical_pipeline.py -v`。
