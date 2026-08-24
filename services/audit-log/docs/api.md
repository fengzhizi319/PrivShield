# 脱敏审计日志 — API 参考

## 基础信息

- 默认地址：`http://127.0.0.1:8084`
- 数据格式：JSON
- 认证：可选 Bearer Token

---

## GET /api/health

健康检查。

**响应示例：**

```json
{
  "backend": "ok",
  "agent": {"status": "ok"},
  "agent_url": "http://127.0.0.1:8079",
  "latency_ms": 2,
  "via": "audit-log"
}
```

---

## GET /api/audit/logs

列出审计日志，支持多维度过滤。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `operation` | string | 否 | 操作类型：mask / classify / k_anon / dp / qol |
| `datasource` | string | 否 | 数据源标识 |
| `user` | string | 否 | 操作用户 |
| `status` | string | 否 | 状态：success / failed |
| `security_level` | string | 否 | 安全等级：L1 / L2 / L3 / L4 / L5 |
| `limit` | int | 否 | 返回条数限制（默认 100） |

**响应示例：**

```json
{
  "total": 150,
  "logs": [
    {
      "id": "audit-1724300000-1",
      "timestamp": "2026-08-22T10:00:00Z",
      "operation": "mask",
      "datasource": "卫健数据库",
      "input_hash": "a1b2c3d4...",
      "output_hash": "e5f6g7h8...",
      "algorithm": "field_mask",
      "parameters": {"fields": ["name", "id_card"]},
      "input_rows": 1000,
      "output_rows": 1000,
      "duration_ms": 45,
      "user": "admin",
      "status": "success",
      "security_level": "L3"
    }
  ],
  "via": "audit-log"
}
```

---

## POST /api/audit/logs

创建审计日志（自动生成功证快照）。

**请求体：**

```json
{
  "operation": "k_anon",
  "datasource": "医保数据库",
  "algorithm": "k_anonymity",
  "parameters": {"k": 5, "qi_cols": ["age", "gender"]},
  "input_rows": 5000,
  "output_rows": 5000,
  "duration_ms": 120,
  "user": "data_scientist",
  "status": "success",
  "security_level": "L4"
}
```

**响应：**

```json
{
  "id": "audit-1724300000-2",
  "via": "audit-log"
}
```

---

## GET /api/audit/logs/:id

获取单条审计日志详情。

---

## GET /api/audit/stats

获取审计统计聚合数据。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `period` | string | 否 | 统计周期：1h / 24h / 7d / 30d（默认 24h） |

**响应示例：**

```json
{
  "total_operations": 1500,
  "by_operation": {
    "mask": 800,
    "k_anon": 400,
    "dp": 200,
    "classify": 100
  },
  "by_status": {
    "success": 1450,
    "failed": 50
  },
  "by_security_level": {
    "L2": 300,
    "L3": 600,
    "L4": 400,
    "L5": 200
  },
  "avg_duration_ms": 67.5,
  "period": "24h"
}
```

---

## GET /api/audit/snapshots

列出存证快照。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `limit` | int | 否 | 返回条数限制（默认 50） |

**响应示例：**

```json
{
  "total": 100,
  "snapshots": [
    {
      "id": "snap-1",
      "audit_log_id": "audit-1724300000-1",
      "timestamp": "2026-08-22T10:00:00Z",
      "algorithm": "field_mask",
      "parameters": {"fields": ["name", "id_card"]},
      "integrity_hash": "sha256:a1b2c3d4e5f6..."
    }
  ],
  "via": "audit-log"
}
```

---

## POST /api/audit/snapshots/verify

验证存证快照完整性。

**请求体：**

```json
{
  "snapshot_id": "snap-1"
}
```

**响应：**

```json
{
  "snapshot_id": "snap-1",
  "valid": true,
  "expected": "sha256:a1b2c3d4e5f6...",
  "actual": "sha256:a1b2c3d4e5f6...",
  "via": "audit-log"
}
```

---

## POST /api/audit/report

生成合规审计报告。

**请求体：**

```json
{
  "period": "7d"
}
```

**响应示例：**

```json
{
  "id": "report-1724300000",
  "generated_at": "2026-08-22T12:00:00Z",
  "period": "7d",
  "total_operations": 10500,
  "success_rate": 97.5,
  "by_security_level": {
    "L2": 2100,
    "L3": 4200,
    "L4": 2800,
    "L5": 1400
  },
  "top_operations": [
    "mask (5600)",
    "k_anon (2800)",
    "dp (1400)",
    "classify (700)"
  ],
  "recommendations": [
    "L4 级别操作频繁，建议审查差分隐私预算消耗",
    "审计指标正常，无需特别关注"
  ]
}
```
