# 脱敏审计日志与存证 (audit-log) — API 规范

`audit-log` 采用 **REST (HTTP/JSON :8084) + gRPC (mTLS :50054)** 双协议架构，为 PrivShield 平台提供全量脱敏合规审计、不可篡改 SHA-256 存证、快照对账与统计合规报告服务。

---

## 1. 通信协议与端口规划

| 协议 | 默认地址 | 认证方式 | 说明 |
|---|---|---|---|
| **HTTP REST** | `http://127.0.0.1:8084` | Bearer Token / API Key | 供 React 前端与 BFF 交互 |
| **gRPC (mTLS)** | `127.0.0.1:50054` | 双向 TLS (mTLS) + 公钥固定 | 供调度流水线与服务集群高性能审计入库 |
| **Prometheus** | `http://127.0.0.1:8084/metrics` | 无（可配置内网隔离） | 指标抓取与监控告警 |

---

## 2. gRPC API 规范 (`auditlog.proto`)

`package auditlog;`

### 2.1 服务接口定义 (`AuditLogService`)

```protobuf
service AuditLogService {
  // Health 健康检查（自检 + 上游 Agent 连通性）
  rpc Health(HealthRequest) returns (HealthResponse);

  // RecordAudit 写入单条审计存证日志
  rpc RecordAudit(RecordAuditRequest) returns (RecordAuditResponse);

  // GetAuditLog 查询单条审计日志
  rpc GetAuditLog(GetAuditLogRequest) returns (AuditLogProto);

  // ListAuditLogs 分页与多维度条件检索审计日志
  rpc ListAuditLogs(ListAuditLogsRequest) returns (ListAuditLogsResponse);

  // GetAuditStats 获取审计与脱敏统计分析指标
  rpc GetAuditStats(GetAuditStatsRequest) returns (AuditStatsResponse);

  // ListSnapshots 查询脱敏快照数据存证
  rpc ListSnapshots(ListSnapshotsRequest) returns (ListSnapshotsResponse);

  // VerifyIntegrity 校验审计快照的 SHA-256 完整性与防篡改存证
  rpc VerifyIntegrity(VerifyIntegrityRequest) returns (VerifyIntegrityResponse);

  // GenerateReport 生成合规审计与治理效能报告
  rpc GenerateReport(GenerateReportRequest) returns (ComplianceReportResponse);
}
```

### 2.2 核心 Proto 消息定义

```protobuf
message AuditLogProto {
  string id = 1;              // 唯一日志 ID
  string timestamp = 2;       // 操作发生时间 (RFC3339)
  string operation = 3;       // "mask" | "classify" | "k_anon" | "dp" | "qol"
  string datasource = 4;      // 数据源标识
  string input_hash = 5;      // 输入数据 SHA256 哈希
  string output_hash = 6;     // 输出数据 SHA256 哈希
  string algorithm = 7;       // 所用脱敏或隐私算法
  string parameters_json = 8; // 算法参数（JSON 字符串）
  int32  input_rows = 9;      // 输入数据行数
  int32  output_rows = 10;    // 输出数据行数
  int64  duration_ms = 11;    // 耗时（毫秒）
  string user = 12;           // 操作人/调用服务
  string status = 13;         // "success" | "failed"
  string error_message = 14;  // 错误信息（失败时）
  string security_level = 15; // 敏感等级 "L1" - "L5"
}

message VerifyIntegrityRequest {
  string snapshot_id = 1;     // 快照 ID
  string expected_hash = 2;   // 期望哈希（空表示与存证哈希比对）
}

message VerifyIntegrityResponse {
  string snapshot_id = 1;
  bool   valid = 2;           // 是否防篡改校验通过
  string computed_hash = 3;   // 本地重计算哈希
  string expected_hash = 4;   // 存证哈希
  string message = 5;         // 校验结论说明
  string via = 6;
}
```

---

## 3. HTTP REST API 规范

### 3.1 审计日志与存证检索

#### `GET /api/audit/logs`
- **参数**：
  - `operation`：操作类型 (`mask` / `classify` / `k_anon` / `dp` / `qol`)
  - `datasource`：数据源名称
  - `user`：操作人员/系统
  - `status`：状态 (`success` / `failed`)
  - `security_level`：敏感等级 (`L1`~`L5`)
  - `limit` (默认 100), `offset` (默认 0)
- **响应示例**：
```json
{
  "total": 150,
  "limit": 100,
  "offset": 0,
  "logs": [
    {
      "id": "audit_1787552256274976692",
      "timestamp": "2026-08-24T14:15:00Z",
      "operation": "mask",
      "datasource": "yibao.csv",
      "input_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "output_hash": "ca978112ca1bbdcafac231b39a23dc4da786081998d6365faf57629009733549",
      "algorithm": "field_mask",
      "parameters": {"fields": ["id_card", "diagnosis_name"]},
      "input_rows": 50,
      "output_rows": 50,
      "duration_ms": 15,
      "user": "service_hub",
      "status": "success",
      "security_level": "L4"
    }
  ],
  "via": "audit-log"
}
```

#### `POST /api/audit/logs`
- **请求体**：
```json
{
  "operation": "k_anon",
  "datasource": "kangyang.csv",
  "algorithm": "k_anonymity",
  "parameters": {"k": 5, "qi_cols": ["age", "gender"]},
  "input_rows": 50,
  "output_rows": 50,
  "duration_ms": 25,
  "user": "sec_officer",
  "status": "success",
  "security_level": "L4"
}
```

#### `GET /api/audit/logs/:id`
- **响应**：指定 ID 的单条审计日志。

---

### 3.2 不可篡改快照与防篡改对账

#### `GET /api/audit/snapshots`
- **说明**：获取脱敏前后样本快照与对应 SHA-256 存证指纹。
- **参数**：`limit` (默认 20), `offset` (默认 0)

#### `POST /api/audit/snapshots/verify`
- **说明**：对指定的快照重新计算 SHA-256 哈希并与存证哈希比对，验证数据是否遭受篡改。
- **请求体**：`{"snapshot_id": "snap-1"}`
- **响应**：
```json
{
  "snapshot_id": "snap-1",
  "valid": true,
  "computed_hash": "ca978112...",
  "expected_hash": "ca978112...",
  "message": "integrity verified: SHA-256 matches non-repudiation proof",
  "via": "audit-log"
}
```

---

### 3.3 统计分析与合规报告

#### `GET /api/audit/stats`
- **说明**：聚合脱敏与治理指标，包含各操作频次、成功率分布、等级构成比及平均处理延迟。
- **参数**：`period` (`1h` | `24h` | `7d` | `30d`，默认 `24h`)

#### `POST /api/audit/report`
- **说明**：生成权威合规评估报告，包含合规建议。
