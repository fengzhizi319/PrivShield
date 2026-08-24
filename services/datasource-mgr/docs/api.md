# 数据源管理 — API 参考

## 基础信息

- 默认地址：`http://127.0.0.1:8083`
- 数据格式：JSON
- 认证：可选 Bearer Token

---

## GET /api/datasources/:id/records

获取指定数据源的实际样本数据记录（如 `yibao.csv` 或 `kangyang.csv`）。

**查询参数：**
- `limit`：返回行数（默认 20，上限 500）
- `offset`：偏移量（默认 0）

**响应示例：**

```json
{
  "datasource_id": "ds_yibao",
  "database": "yibao.csv",
  "total": 50,
  "limit": 20,
  "offset": 0,
  "records": [
    {
      "insurance_settlement_id": "YB202511040001",
      "person_id": "PID66453983",
      "gender": "男",
      "birth_date": "1968-09-17",
      "diagnosis_name": "硬下疳伴TPPA滴度1:64阳性(早期梅毒)"
    }
  ],
  "via": "datasource-mgr"
}
```

---

## POST /api/datasources/seed

一键初始化或重置预置的模拟数据源（`yibao.csv` 与 `kangyang.csv`）。

**响应示例：**

```json
{
  "message": "successfully seeded mock data sources: yibao.csv, kangyang.csv",
  "via": "datasource-mgr"
}
```

---

## GET /metrics

Prometheus 指标抓取端点。

---

## GET /api/health

健康检查。

**响应示例：**

```json
{
  "backend": "ok",
  "agent": {"status": "ok"},
  "agent_url": "http://127.0.0.1:8079",
  "latency_ms": 3,
  "via": "datasource-mgr"
}
```

---

## GET /api/datasources

列出所有已注册的数据源。

**响应示例：**

```json
{
  "total": 2,
  "datasources": [
    {
      "id": "ds-1724300000-1",
      "name": "卫健数据库",
      "type": "database",
      "host": "192.168.1.100",
      "port": 5432,
      "database": "health_db",
      "security_level": "high",
      "status": "connected",
      "created_at": "2026-08-22T10:00:00Z",
      "tags": ["卫健", "高密"]
    }
  ],
  "via": "datasource-mgr"
}
```

---

## POST /api/datasources

注册新数据源。

**请求体：**

```json
{
  "name": "医保数据库",
  "type": "database",
  "host": "192.168.1.101",
  "port": 5432,
  "database": "insurance_db",
  "security_level": "high",
  "tags": ["医保", "高密"]
}
```

**响应：**

```json
{
  "id": "ds-1724300000-2",
  "via": "datasource-mgr"
}
```

---

## GET /api/datasources/:id

获取单个数据源详情。

---

## DELETE /api/datasources/:id

删除数据源。

---

## POST /api/datasources/:id/test

测试数据源连接。

**响应：**

```json
{
  "datasource_id": "ds-1724300000-1",
  "success": true,
  "latency_ms": 45,
  "via": "datasource-mgr"
}
```

---

## GET /api/datasources/:id/metadata

获取数据源元数据（自动分类分级）。

**响应示例：**

```json
{
  "datasource_id": "ds-1724300000-1",
  "tables": [
    {
      "name": "patients",
      "row_count": 10000,
      "fields": [
        {
          "name": "id",
          "type": "integer",
          "security_level": "L1",
          "sensitive": false
        },
        {
          "name": "name",
          "type": "string",
          "security_level": "L3",
          "classification": "PII",
          "sensitive": true
        },
        {
          "name": "id_card",
          "type": "string",
          "security_level": "L4",
          "classification": "PII",
          "sensitive": true
        }
      ]
    }
  ],
  "via": "datasource-mgr"
}
```

---

## GET /api/datasources/:id/audit

获取数据源访问审计日志。

**响应示例：**

```json
{
  "total": 5,
  "records": [
    {
      "id": "audit-1",
      "datasource_id": "ds-1724300000-1",
      "datasource_name": "卫健数据库",
      "operation": "query_metadata",
      "user": "system",
      "timestamp": "2026-08-22T10:05:00Z",
      "records_count": 0,
      "status": "success"
    }
  ],
  "via": "datasource-mgr"
}
```
