# 脱敏审计日志 (Audit Log)

脱敏审计日志模块负责记录和管理所有脱敏操作的审计日志，提供日志查询、统计聚合、存证快照、完整性校验与合规报告生成等功能。

## 功能特性

- **审计日志记录**：记录所有脱敏操作的详细信息（操作类型、数据源、算法、参数、耗时等）
- **日志查询与过滤**：支持按时间、操作类型、数据源、用户、状态、安全等级等多维度查询
- **统计聚合**：按操作类型、状态、安全等级等维度统计审计数据
- **存证快照**：自动为每次脱敏操作生成存证快照，包含完整性哈希
- **完整性校验**：验证存证快照的完整性，防止篡改
- **合规报告**：生成符合数据安全法/个保法要求的合规审计报告

## 快速开始

### 开发模式

```bash
cd services/audit-log
bash run.sh
```

默认监听 `http://127.0.0.1:8084`。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `AUDIT_LOG_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `AUDIT_LOG_PORT` | `8084` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 可选认证密钥 |
| `AUDIT_LOG_MAX_ENTRIES` | `10000` | 内存中保留的最大日志条数 |

### 构建

```bash
make build
```

### 测试

```bash
make test
```

### Docker

构建上下文为仓库根目录（需复制共享库 `pkg/`）：

```bash
# 在仓库根目录执行
docker build -f services/audit-log/Dockerfile -t privshield-audit-log .
docker run -p 8084:8084 -e PRIVACY_AGENT_REST_HOST=host.docker.internal privshield-audit-log
```

## API 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/audit/logs` | 审计日志列表（支持多维度过滤） |
| POST | `/api/audit/logs` | 创建审计日志 |
| GET | `/api/audit/logs/:id` | 获取单条日志 |
| GET | `/api/audit/stats` | 统计聚合 |
| GET | `/api/audit/snapshots` | 存证快照列表 |
| POST | `/api/audit/snapshots/verify` | 完整性校验 |
| POST | `/api/audit/report` | 生成合规报告 |

## 与分级脱敏模块的集成

审计日志模块通过以下方式与分级脱敏模块集成：

1. **自动记录**：每次脱敏操作（mask/k_anon/dp/classify/qol）都会自动记录审计日志
2. **存证快照**：为每次脱敏生成存证快照，包含输入/输出哈希、算法参数、时间戳
3. **完整性校验**：使用 SHA256 哈希链确保存证不可篡改
4. **合规报告**：根据审计数据生成符合数据安全法/个保法的合规报告
5. **安全等级追踪**：记录每次操作涉及的数据安全等级（L1~L5）

## 文档

- [设计文档](docs/design.md)
- [API 参考](docs/api.md)
- [运维手册](docs/ops.md)
