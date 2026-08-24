# 数据源管理 (Datasource Manager)

数据源管理模块负责统一管理 PrivShield 控制台中的所有数据源连接，提供数据源注册、连通性测试、元数据浏览、安全等级标记与访问审计等功能。

## 功能特性

- **数据源注册**：支持数据库、API、文件等多种类型数据源的注册管理
- **连通性测试**：一键测试数据源连接可用性
- **元数据浏览**：查看数据表结构、字段类型、安全等级
- **安全等级标记**：按高密/中密/低密标记数据源安全级别
- **访问审计**：记录所有数据源访问操作（查询/导出/脱敏）
- **分类分级联动**：自动调用 Agent 对字段进行分类分级

## 快速开始

### 开发模式

```bash
cd console/datasource-mgr
bash run.sh
```

默认监听 `http://127.0.0.1:8083`。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `DATASOURCE_MGR_PORT` | `8083` | HTTP 监听端口 |
| `PRIVACY_AGENT_REST_HOST` | `127.0.0.1` | 上游 Agent REST 地址 |
| `PRIVACY_REST_PORT` | `8079` | 上游 Agent REST 端口 |
| `PRIVACY_AGENT_API_KEY` | (空) | 可选认证密钥 |

### 构建

```bash
make build
```

### 测试

```bash
make test
```

### Docker

```bash
docker build -t privshield-datasource-mgr .
docker run -p 8083:8083 -e PRIVACY_AGENT_REST_HOST=host.docker.internal privshield-datasource-mgr
```

## API 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/datasources` | 数据源列表 |
| POST | `/api/datasources` | 注册新数据源 |
| GET | `/api/datasources/:id` | 获取单个数据源 |
| DELETE | `/api/datasources/:id` | 删除数据源 |
| POST | `/api/datasources/:id/test` | 测试连接 |
| GET | `/api/datasources/:id/metadata` | 获取元数据 |
| GET | `/api/datasources/:id/audit` | 访问审计日志 |

## 与分级脱敏模块的集成

数据源管理通过元数据浏览接口实现与分级脱敏模块的集成：

1. 查询元数据时自动调用 Agent 的 `/v1/dynclassification/classify` 对字段进行分类
2. 根据分类结果自动标记字段的安全等级（L1~L5）
3. 敏感字段（PII、医疗数据等）自动打标，便于后续脱敏策略选择
4. 访问审计记录所有对数据源的操作，包括脱敏操作

## 文档

- [设计文档](docs/design.md)
- [API 参考](docs/api.md)
- [运维手册](docs/ops.md)
