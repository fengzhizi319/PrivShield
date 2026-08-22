# 数据源管理 — 产品需求文档 (PRD)

## 1. 产品概述

**数据源管理**（Datasource Manager）是 PrivShield 控制台的数据源管理模块，提供数据源的注册、连接测试、元数据自动采集与分类分级能力。

| 属性 | 值 |
|---|---|
| 模块名称 | datasource-mgr |
| 默认端口 | 8083 |
| 开发语言 | Go + Gin |
| 上游依赖 | PrivShield Agent REST API (:8079) |

## 2. 核心需求

### 2.1 数据源生命周期

```
注册 → 连接测试 → 元数据采集 → 自动分类分级 → 访问审计
```

### 2.2 数据源类型支持

| 类型 | 说明 | 连接参数 |
|---|---|---|
| database | 关系型数据库 | host/port/database/username/password |
| api | REST API 数据源 | base_url/api_key |
| file | 文件数据源 | file_path/format |
| kafka | 消息队列 | brokers/topic/group |

### 2.3 元数据自动分类分级

- 采集表结构（字段名、类型、注释）
- 调用 Agent `/v1/dynclassification/classify` 对字段名进行自动分类
- 根据分类结果自动标注安全等级（L1-L5）

## 3. 功能需求

### 3.1 API 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/datasources` | 数据源列表 |
| POST | `/api/datasources` | 注册数据源 |
| GET | `/api/datasources/:id` | 数据源详情 |
| PUT | `/api/datasources/:id` | 更新数据源 |
| DELETE | `/api/datasources/:id` | 删除数据源 |
| POST | `/api/datasources/:id/test` | 连接测试 |
| GET | `/api/datasources/:id/metadata` | 获取元数据（含自动分类） |
| GET | `/api/datasources/:id/audit` | 访问审计日志 |

### 3.2 数据源模型

```json
{
  "id": "uuid",
  "name": "卫健数据库",
  "type": "database",
  "host": "192.168.1.100",
  "port": 5432,
  "database": "health_db",
  "security_level": "high",
  "tags": ["卫健", "高密"],
  "created_at": "2026-08-23T10:00:00Z",
  "updated_at": "2026-08-23T10:00:00Z"
}
```

### 3.3 配置项

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `DATASOURCE_MGR_HOST` | `0.0.0.0` | 监听地址 |
| `DATASOURCE_MGR_PORT` | `8083` | 监听端口 |
| `DATASOURCE_MGR_AGENT_REST_HOST` | `127.0.0.1` | Agent REST 主机 |
| `DATASOURCE_MGR_AGENT_REST_PORT` | `8079` | Agent REST 端口 |
| `DATASOURCE_MGR_AGENT_API_KEY` | — | Agent API Key |

## 4. 非功能需求

- **安全性**: 连接凭证内存存储，不落盘明文密码
- **审计**: 所有 CRUD 操作自动记录审计日志
- **可靠性**: 连接测试超时 10s，不影响主流程
- **可扩展**: 数据源类型通过接口扩展

## 5. 集成关系

```
service-hub → datasource-mgr (获取数据源信息)
            → PrivShield Agent (元数据分类分级)
```

- **上游**: 用户 / service-hub
- **下游**: PrivShield Agent（分类分级 API）
- **协同**: audit-log 记录数据源操作审计
