# 脱敏审计日志 — 产品需求文档 (PRD)

## 1. 产品概述

**脱敏审计日志**（Audit Log）是 PrivShield 控制台的审计合规模块，记录所有脱敏操作的完整审计轨迹，提供 SHA256 完整性校验和合规报告生成能力。

| 属性 | 值 |
|---|---|
| 模块名称 | audit-log |
| 默认端口 | 8084 |
| 开发语言 | Go + Gin |
| 上游依赖 | PrivShield Agent REST API (:8079) |

## 2. 核心需求

### 2.1 审计记录生命周期

```
操作发生 → 创建审计记录 → SHA256 完整性哈希 → 生成快照 → 定期合规报告
```

### 2.2 审计记录内容

每条审计记录必须包含：

| 字段 | 说明 |
|---|---|
| operation | 操作类型（mask/k_anon/dp/qol/classify） |
| datasource | 数据源名称 |
| algorithm | 使用的算法 |
| parameters | 算法参数 |
| input_rows | 输入行数 |
| output_rows | 输出行数 |
| duration_ms | 处理耗时（毫秒） |
| user | 操作用户 |
| status | 操作状态（success/failed） |
| security_level | 安全等级（L1-L5） |
| integrity_hash | SHA256 完整性哈希 |

### 2.3 完整性校验

- 每条记录自动生成 `SHA256(id + timestamp + algorithm)` 完整性哈希
- 快照（Snapshot）定期保存审计日志摘要
- 支持按快照 ID 验证完整性

### 2.4 合规报告

- 支持按时段生成报告（1h/24h/7d/30d）
- 统计：总操作数、按操作类型分布、成功率、平均耗时
- 合规建议：根据统计结果自动生成改进建议

## 3. 功能需求

### 3.1 API 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/audit/logs` | 审计日志列表（支持过滤） |
| POST | `/api/audit/logs` | 创建审计记录 |
| GET | `/api/audit/logs/:id` | 审计记录详情 |
| GET | `/api/audit/stats` | 审计统计概览 |
| GET | `/api/audit/snapshots` | 快照列表 |
| POST | `/api/audit/snapshots/verify` | 验证快照完整性 |
| POST | `/api/audit/report` | 生成合规报告 |

### 3.2 过滤与查询

审计日志列表支持以下过滤参数：

| 参数 | 类型 | 说明 |
|---|---|---|
| operation | string | 按操作类型过滤 |
| datasource | string | 按数据源过滤 |
| status | string | 按状态过滤 |
| security_level | string | 按安全等级过滤 |
| user | string | 按用户过滤 |
| limit | int | 返回条数限制（默认 50） |
| offset | int | 偏移量（默认 0） |

### 3.3 配置项

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `AUDIT_LOG_HOST` | `0.0.0.0` | 监听地址 |
| `AUDIT_LOG_PORT` | `8084` | 监听端口 |
| `AUDIT_LOG_AGENT_REST_HOST` | `127.0.0.1` | Agent REST 主机 |
| `AUDIT_LOG_AGENT_REST_PORT` | `8079` | Agent REST 端口 |
| `AUDIT_LOG_AGENT_API_KEY` | — | Agent API Key |
| `AUDIT_LOG_MAX_LOG_ENTRIES` | `10000` | 最大内存审计记录数 |

## 4. 非功能需求

- **不可篡改**: 审计记录创建后不可修改，仅追加
- **完整性**: SHA256 哈希确保记录未被篡改
- **性能**: 写入延迟 < 5ms，查询延迟 < 50ms
- **容量**: 内存存储支持 10000 条记录，超出后 FIFO 淘汰
- **合规**: 满足等保 2.0 / GDPR 审计要求

## 5. 集成关系

```
service-hub → audit-log (脱敏任务审计)
datasource-mgr → audit-log (数据源操作审计)
PrivShield Agent → audit-log (可选，直接调用审计 API)
```

- **上游**: service-hub / datasource-mgr / 直接 API 调用
- **下游**: 合规报告输出（文件/API）
- **存储**: 内存环形缓冲 + 快照持久化
