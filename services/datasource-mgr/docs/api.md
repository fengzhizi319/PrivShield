# 模拟数据源服务 (datasource-mgr) — API 规范

`datasource-mgr` 是 PrivShield 开发与调试专用的模拟数据源服务。生产环境中下游服务将直接对接真实外部数据源，本项目仅用于开发、测试与联调。模块支持 **REST (HTTP/JSON :8083) + gRPC (mTLS :50053)** 双协议接入。

---

## 1. 通信协议与端口规划

| 协议 | 默认地址 | 认证方式 | 说明 |
|---|---|---|---|
| **HTTP REST** | `http://127.0.0.1:8083` | Bearer Token / API Key | 供 React 前端与 BFF 开发调试 |
| **gRPC (mTLS)** | `127.0.0.1:50053` | 双向 TLS (mTLS) + 公钥固定 | 供调度中枢与微服务集群模拟数据采样 |

---

## 2. 模拟数据接口规划 (API 1 ~ 4)

| 接口编号 | 对应数据源 | REST 路径 | gRPC 方法 | 说明 |
|---|---|---|---|---|
| **API 1** | 医保数据源 | `GET /api/v1/yibao` | `GetYibaoData` | 模拟医保就医、诊断与结算流水 (`yibao.csv`) |
| **API 2** | 康养数据源 | `GET /api/v1/kangyang` | `GetKangyangData` | 模拟康养中心体检、慢病随访与健康档案 (`kangyang.csv`) |
| **API 3** | 预留接口 3 | `GET /api/v1/mock3` | `GetMockData3` | 预留政务数据源 3 模拟数据 |
| **API 4** | 预留接口 4 | `GET /api/v1/mock4` | `GetMockData4` | 预留政务数据源 4 模拟数据 |

---

## 3. gRPC API 规范 (`datasourcemgr.proto`)

`package datasourcemgr;`

### 3.1 服务接口定义 (`DataSourceManagerService`)

```protobuf
service DataSourceManagerService {
  // Health 健康检查
  rpc Health(HealthRequest) returns (HealthResponse);

  // API 1: 获取医保就医与结算模拟数据 (yibao.csv)
  rpc GetYibaoData(DataQueryRequest) returns (DataQueryResponse);

  // API 2: 获取康养体检与慢病模拟数据 (kangyang.csv)
  rpc GetKangyangData(DataQueryRequest) returns (DataQueryResponse);

  // API 3: 预留模拟数据源扩展接口 3
  rpc GetMockData3(DataQueryRequest) returns (DataQueryResponse);

  // API 4: 预留模拟数据源扩展接口 4
  rpc GetMockData4(DataQueryRequest) returns (DataQueryResponse);

  // 通用按数据源 ID 获取模拟数据
  rpc GetDataBySource(SourceDataQueryRequest) returns (DataQueryResponse);

  // 列出所有内置模拟数据源
  rpc ListMockSources(ListMockSourcesRequest) returns (ListMockSourcesResponse);

  // 获取单个模拟数据源基本信息
  rpc GetDataSource(GetDataSourceRequest) returns (DataSourceProto);

  // 模拟数据源连通性测试
  rpc TestConnection(TestConnectionRequest) returns (TestConnectionResponse);
}
```

### 3.2 核心 Proto 消息定义

```protobuf
message DataQueryRequest {
  int32 limit = 1;         // 返回条数（默认 20）
  int32 offset = 2;        // 偏移量（默认 0）
}

message SourceDataQueryRequest {
  string source_id = 1;    // "ds_yibao" | "ds_kangyang" | "ds_mock3" | "ds_mock4"
  int32 limit = 2;
  int32 offset = 3;
}

message DataRowProto {
  map<string, string> fields = 1;
}

message DataQueryResponse {
  string source_id = 1;
  string source_name = 2;
  int32 total = 3;
  int32 limit = 4;
  int32 offset = 5;
  repeated DataRowProto records = 6;
  string via = 7;
}
```

---

## 4. HTTP REST API 规范

### 4.1 获取模拟数据集

#### `GET /api/v1/yibao` (API 1: 医保数据)
- **参数**：`limit` (默认 20), `offset` (默认 0)
- **响应示例**：
```json
{
  "source_id": "ds_yibao",
  "source_name": "医保就医与结算模拟数据库 (yibao.csv)",
  "total": 50,
  "limit": 20,
  "offset": 0,
  "records": [
    {
      "person_id": "510101198503151234",
      "name": "李明",
      "gender": "男",
      "diagnosis_name": "原发性高血压",
      "settlement_amount": 158.5
    }
  ],
  "via": "datasource-mgr"
}
```

#### `GET /api/v1/kangyang` (API 2: 康养数据)
- **参数**：`limit` (默认 20), `offset` (默认 0)

#### `GET /api/v1/mock3` (API 3: 预留数据 3) / `GET /api/v1/mock4` (API 4: 预留数据 4)
- **参数**：`limit`, `offset`

---

### 4.2 模拟数据源元数据与连通性

#### `GET /api/datasources`
- **说明**：列出所有内置模拟数据源列表。

#### `GET /api/datasources/:id`
- **说明**：获取指定模拟数据源的基本信息。

#### `GET /api/datasources/:id/records`
- **说明**：通用数据读取端点（支持 `ds_yibao`、`ds_kangyang`、`ds_mock3`、`ds_mock4`）。

#### `POST /api/datasources/:id/test`
- **说明**：模拟连通性测试。
- **响应**：`{"datasource_id": "ds_yibao", "success": true, "latency_ms": 1, "via": "datasource-mgr"}`
