# 数据源管理与特征探查 (datasource-mgr) — API 规范

`datasource-mgr` 采用 **REST (HTTP/JSON) + gRPC (mTLS)** 双协议架构，为前端控制台与分布式微服务集群提供高性能、安全的数据源资产管理与敏感特征自动探查服务。

---

## 1. 通信协议与端口规划

| 协议 | 默认地址 | 认证方式 | 说明 |
|---|---|---|---|
| **HTTP REST** | `http://127.0.0.1:8083` | Bearer Token / API Key | 供 React 前端与 BFF 交互 |
| **gRPC (mTLS)** | `127.0.0.1:50053` | 双向 TLS (mTLS) + 公钥固定 | 供跨节点微服务高性能 RPC 调用 |
| **Prometheus** | `http://127.0.0.1:8083/metrics` | 无（可配置内网隔离） | 指标抓取与监控告警 |

---

## 2. gRPC API 规范 (`datasourcemgr.proto`)

`package datasourcemgr;`

### 2.1 服务接口定义 (`DataSourceManagerService`)

```protobuf
service DataSourceManagerService {
  // Health 健康检查（自检 + 上游 Agent 连通性）
  rpc Health(HealthRequest) returns (HealthResponse);

  // ListDataSources 获取已注册的数据源列表（支持分页）
  rpc ListDataSources(ListDataSourcesRequest) returns (ListDataSourcesResponse);

  // GetDataSource 查询单个数据源详情
  rpc GetDataSource(GetDataSourceRequest) returns (DataSourceProto);

  // CreateDataSource 注册新的数据源
  rpc CreateDataSource(CreateDataSourceRequest) returns (DataSourceProto);

  // UpdateDataSource 更新数据源配置
  rpc UpdateDataSource(UpdateDataSourceRequest) returns (DataSourceProto);

  // DeleteDataSource 删除数据源
  rpc DeleteDataSource(DeleteDataSourceRequest) returns (DeleteDataSourceResponse);

  // TestConnection 测试数据源连通性
  rpc TestConnection(TestConnectionRequest) returns (TestConnectionResponse);

  // GetMetadata 获取数据源元数据与敏感字段自动探查分类结果
  rpc GetMetadata(GetMetadataRequest) returns (MetadataResponse);

  // GetDataSourceRecords 读取数据源明细记录或采样数据
  rpc GetDataSourceRecords(GetRecordsRequest) returns (GetRecordsResponse);

  // GetAccessAudit 查询数据源访问审计日志
  rpc GetAccessAudit(GetAccessAuditRequest) returns (AccessAuditResponse);

  // SeedDataSources 初始化/预置默认样例数据源
  rpc SeedDataSources(SeedDataSourcesRequest) returns (SeedDataSourcesResponse);
}
```

### 2.2 核心 Proto 消息定义

```protobuf
message DataSourceProto {
  string id = 1;              // 唯一 ID (如 "ds_yibao")
  string name = 2;            // 显示名称
  string type = 3;            // "database" | "api" | "file"
  string host = 4;            // 连接主机
  int32  port = 5;            // 连接端口
  string database = 6;        // 数据库名称 / 文件名
  string security_level = 7;  // "high" | "medium" | "low"
  string status = 8;          // "connected" | "disconnected" | "error"
  string created_at = 9;      // 创建时间 (RFC3339)
  string last_check_at = 10;  // 最后健康检查时间 (RFC3339)
  repeated string tags = 11;  // 业务标签 ("医保", "门诊住院", etc.)
}

message MetadataFieldProto {
  string name = 1;           // 字段名 (如 "id_card", "diagnosis_name")
  string type = 2;           // 数据类型 ("string", "integer", "float")
  string security_level = 3; // 敏感等级 ("L1" - "L5")
  string classification = 4; // 敏感分类标签 ("PII_IDCard", "Medical_Diagnosis")
  bool   sensitive = 5;      // 是否敏感特征
}

message TableMetadataProto {
  string name = 1;           // 表名 / 实体名
  repeated MetadataFieldProto fields = 2;
  int32 row_count = 3;       // 记录总行数
}

message MetadataResponse {
  string datasource_id = 1;
  repeated TableMetadataProto tables = 2;
  string via = 3;
}
```

---

## 3. HTTP REST API 规范

### 3.1 健康检查与运维

#### `GET /api/health` / `GET /health`
- **说明**：检查本服务状态与上游 `PrivShield Agent` 连通性及网络耗时。
- **响应示例** (200 OK)：
```json
{
  "backend": "ok",
  "agent": {"status": "ok", "version": "0.1.0"},
  "agent_url": "http://127.0.0.1:8079",
  "latency_ms": 3,
  "via": "datasource-mgr"
}
```

#### `GET /metrics`
- **说明**：Prometheus 指标端点，暴露请求量、延迟分布与连接池健康度。

---

### 3.2 数据源资产管理 (CRUD)

#### `GET /api/datasources`
- **参数**：`limit` (默认 100), `offset` (默认 0)
- **响应**：
```json
{
  "total": 2,
  "limit": 100,
  "offset": 0,
  "datasources": [
    {
      "id": "ds_yibao",
      "name": "医保就医与结算模拟数据库 (yibao.csv)",
      "type": "file",
      "host": "127.0.0.1",
      "port": 8083,
      "database": "yibao.csv",
      "security_level": "high",
      "status": "connected",
      "created_at": "2026-08-24T14:00:00Z",
      "tags": ["医保", "门诊住院", "结算流水", "敏感数据"]
    }
  ],
  "via": "datasource-mgr"
}
```

#### `POST /api/datasources`
- **请求体**：
```json
{
  "name": "市妇幼电子病历库",
  "type": "database",
  "host": "192.168.1.120",
  "port": 5432,
  "database": "emr_db",
  "security_level": "high",
  "tags": ["妇幼", "电子病历", "L4/L5敏感"]
}
```
- **校验规则**：`type` 限制为 `database|api|file`；`port` 限制为 1-65535；`security_level` 限制为 `high|medium|low`。

#### `GET /api/datasources/:id`
- **响应**：单个数据源详情。若不存在返回 404。

#### `PUT /api/datasources/:id`
- **请求体**：更新字段（支持增量修改名称、端口、标签、安全等级）。

#### `DELETE /api/datasources/:id`
- **响应**：`{"message": "datasource deleted", "via": "datasource-mgr"}`。

---

### 3.3 敏感特征自动探查与采样读取

#### `GET /api/datasources/:id/metadata`
- **说明**：对目标数据源执行自动化探查，结合 `PrivShield Agent` 3层分类漏斗输出字段级敏感分类与等级（L1-L5）。
- **响应示例**：
```json
{
  "datasource_id": "ds_yibao",
  "tables": [
    {
      "name": "yibao.csv",
      "row_count": 50,
      "fields": [
        {
          "name": "id_card",
          "type": "string",
          "security_level": "L4",
          "classification": "PII_IDCard",
          "sensitive": true
        },
        {
          "name": "diagnosis_name",
          "type": "string",
          "security_level": "L4",
          "classification": "Medical_Diagnosis",
          "sensitive": true
        }
      ]
    }
  ],
  "via": "datasource-mgr"
}
```

#### `GET /api/datasources/:id/records` / `GET /api/datasources/:id/sample`
- **说明**：安全采样读取数据源明细记录。内置防路径穿越（LFI）与 50,000 行内存上限防护（DoS 防护）。
- **参数**：`limit` (默认 20), `offset` (默认 0)

#### `POST /api/datasources/:id/test`
- **说明**：测试网络连通性与文件可读性。返回网络往返延迟 `latency_ms`。

#### `GET /api/datasources/:id/audit`
- **说明**：查询数据源访问与导出审计记录。

#### `POST /api/datasources/seed`
- **说明**：一键预置 `yibao.csv` 与 `kangyang.csv` 演示数据源。
