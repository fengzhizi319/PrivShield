# 动态分类分级（多标准适配）文档索引

本目录包含 `privacy-local-agent` 动态分类分级（多标准适配）模块的全套 SDLC 文档。

## 文档清单

| 文档 | 说明 | 目标读者 |
|---|---|---|
| [prd.md](./prd.md) | 产品需求文档（PRD） | 产品经理、项目经理 |
| [design.md](./design.md) | 架构设计、通用引擎实现与配置解耦方案 | 架构师、后端开发 |
| [api_reference.md](./api_reference.md) | Python SDK / REST / gRPC API 参考手册 | 接入开发者、系统集成商 |
| [examples.md](./examples.md) | Python SDK 与 REST API 代码使用示例 | 接入开发者 |
| [ops.md](./ops.md) | 运维手册、热加载管理、监控与故障排查 | SRE、运维工程师 |
| [testing.md](./testing.md) | 测试策略、测试代码示例与 Schema 校验指南 | QA、测试开发工程师 |
| [three_layer_funnel_design.md](./three_layer_funnel_design.md) | 三层漏斗模型 + 置信度策略设计 | 架构师、后端开发 |
| [downgrade_override_design.md](./downgrade_override_design.md) | 敏感度降级与 Override 压制规则设计 | 架构师、后端开发 |
| [rule_parsing_guide.md](./rule_parsing_guide.md) | 规则 YAML 解析与编写指南 | 接入开发者、运维 |

## 核心设计理念

动态分类分级架构旨在解决数据分类分级逻辑与代码深度绑定（硬编码）的问题。核心思想为：

1. **标准配置化**：解耦硬编码 Enum，允许通过 YAML 动态定义分类树（Categories）与分级矩阵（Levels: L1~L5 / C1~C4 / 1~4级）。
2. **标准文档自动生成**：支持输入 Markdown 格式的行业/地方分类分级标准文档（如《四川省健康医疗大数据应用指南.md》），自动抽取并解析生成全套规则 YAML 配置文件，降低配置门槛。
3. **规则声明化**：匹配条件（字段名模式、值正则、匹配算子）完全配置化，支持按领域包（Domain Packs）与标准组合（Standards）组织。
4. **算子插件化**：内置通用匹配算子（`regex`、`id_card_checksum`、`luhn_checksum` 等），并提供单例算子注册表（`OperatorRegistry`）支持业务自定义算子扩展。
5. **执行上下文动态化**：请求时传入 `domain` 或 `standard` 上下文，引擎自动从 `ProfileLoader` 按需加载对应规则包，并支持规则热重载。


## 快速开始

1. 阅读 [prd.md](./prd.md) 了解业务需求与功能定义。
2. 阅读 [design.md](./design.md) 掌握通用引擎架构、元数据模型与匹配算子设计。
3. 查看 [examples.md](./examples.md) 学习如何在代码中注册自定义算子与调用分类 API。
4. 开发接入时参考 [api_reference.md](./api_reference.md)。
5. 生产部署、规则更新与热重载参考 [ops.md](./ops.md)。
6. 编写测试用例或验证规则 YAML 时参考 [testing.md](./testing.md)。
