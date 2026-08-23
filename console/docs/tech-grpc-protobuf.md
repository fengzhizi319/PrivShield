# gRPC & Protocol Buffers 技术栈说明 / gRPC & Protocol Buffers Technology Stack

## 1. 技术简介 / Introduction

### gRPC
gRPC 是 Google 开源的高性能、跨语言远程过程调用（RPC）框架。
gRPC is a high-performance, cross-language Remote Procedure Call (RPC) framework open-sourced by Google.

核心特性 / Core Features：
- **HTTP/2 传输**：多路复用、头部压缩、双向流，比 HTTP/1.1 效率更高。
- **Protocol Buffers 序列化**：二进制编码，体积比 JSON 小 3-10 倍，解析速度快 5-100 倍。
- **强类型契约**：通过 .proto 文件定义服务接口，自动生成多语言客户端/服务端代码。
- **四种通信模式**：Unary、Server Streaming、Client Streaming、Bidirectional Streaming。
- **元数据传递**：类似 HTTP Header 的 metadata 机制，用于认证、追踪等。

### Protocol Buffers (protobuf)
Protocol Buffers 是 Google 的语言无关、平台无关的序列化数据格式。
Protocol Buffers is Google's language-neutral, platform-neutral serialization data format.

本项目使用版本 / Versions Used：
- `google.golang.org/grpc v1.82.1`
- `google.golang.org/protobuf v1.36.11`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 架构角色 / Architecture Role

```
1. 核心原语调用链路：
   React 前端 ──HTTP/JSON──▶ Go 代理网关(Gin) ──gRPC/protobuf──▶ Python Agent(FastAPI+gRPC)
                             console/backend-go                     PrivShield (:50051)

2. 调度中枢高性能服务链路：
   外部高并发客户端 ──gRPC/mTLS (公钥固定)──▶ 调度中枢 gRPC 服务 (ServiceHubService)
                                               console/service-hub (:50052)
```

- **Go 代理网关 (`backend-go`)** 作为“协议转换客户端”：前端发送 HTTP/JSON → Go 后端解析 → 构造 protobuf 消息 → gRPC 调用 → 解析响应 → 返回 JSON。
- **调度中枢 (`service-hub`)** 作为“高性能 gRPC 服务端”：对外暴露 `ServiceHubService`，支持金融级 mTLS 双向证书校验与客户端公钥指纹固定（Public Key Pinning）。

### 2.2 Proto 定义 / Proto Definition

本项目包含两套核心 Proto 定义：

#### (1) Agent 隐私原语服务：`proto/privacy.proto`（项目根目录）

```protobuf
service PrivacyService {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Mask(MaskRequest) returns (MaskResponse);
  rpc DPCount(DPRequest) returns (DPResponse);
  rpc KAnonymizeTable(KAnonymizeTableRequest) returns (KAnonymizeTableResponse);
  rpc Classify(ClassifyRequest) returns (ClassifyResponse);
  // ... 核心隐私原语 RPC 方法
}
```

#### (2) 调度中枢服务：`console/service-hub/proto/servicehub.proto`

```protobuf
service ServiceHubService {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc HubStatus(HubStatusRequest) returns (HubStatusResponse);
  rpc Dispatch(DispatchRequest) returns (DispatchResponse);
  rpc ClassifyAndDispatch(ClassifyAndDispatchRequest) returns (ClassifyAndDispatchResponse);
  rpc GetTask(GetTaskRequest) returns (TaskProto);
  rpc ListTasks(ListTasksRequest) returns (ListTasksResponse);
  rpc PipelineStatus(PipelineStatusRequest) returns (PipelineStatusResponse);
}
```

### 2.3 客户端连接管理 / Client Connection Management

文件 / File：`console/backend-go/internal/agent/client.go`

```go
func New(cfg *config.Config) (*Client, error) {
    conn, err := grpc.NewClient(
        target,
        grpc.WithTransportCredentials(creds),      // TLS 或非安全传输 / TLS or insecure
        grpc.WithDefaultCallOptions(
            grpc.MaxCallRecvMsgSize(64<<20),        // 64MB 接收上限 / 64MB recv limit
            grpc.MaxCallSendMsgSize(64<<20),        // 64MB 发送上限 / 64MB send limit
        ),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:    30 * time.Second,              // 心跳间隔 / Heartbeat interval
            Timeout: 10 * time.Second,              // 心跳超时 / Heartbeat timeout
        }),
    )
    return &Client{conn: conn, client: pb.NewPrivacyServiceClient(conn)}, nil
}
```

### 2.4 REST → gRPC 路由映射 / REST → gRPC Route Mapping

文件 / File：`console/backend-go/internal/mapper/mapper.go`

```go
// 分发表：REST 路径 → gRPC handler / Dispatch table: REST path → gRPC handler
m.handlers = map[string]Handler{
    "/v1/privacy/mask":           m.handleMask,
    "/v1/privacy/dp/count":       m.handleDPCount,
    "/v1/privacy/k_anonymize/table": m.handleKAnonymizeTable,
    // ... 33 个端点映射 / 33 endpoint mappings
}

// 分发入口：O(1) 哈希查找 / Dispatch entry: O(1) hash lookup
func (m *Mapper) Dispatch(ctx, client, path, body) (any, error) {
    if handler, ok := m.handlers[path]; ok {
        return handler(ctx, client, body)
    }
    return nil, fmt.Errorf("unsupported gRPC path: %s", path)
}
```

### 2.5 认证元数据 / Authentication Metadata

```go
// 在 gRPC 调用中附加 Bearer Token / Attach Bearer Token to gRPC calls
func (c *Client) WithAuth(ctx context.Context) context.Context {
    if c.cfg.AgentAPIKey == "" {
        return ctx
    }
    return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.cfg.AgentAPIKey)
}
```

### 2.6 代码生成 / Code Generation

```bash
# 从 .proto 生成 Go gRPC 代码 / Generate Go gRPC code from .proto
protoc --go_out=. --go-grpc_out=. proto/privacy.proto

# 生成的文件 / Generated files:
# console/backend-go/proto/privacy.pb.go       → 消息类型 / Message types
# console/backend-go/proto/privacy_grpc.pb.go  → 服务接口 / Service interfaces
```

### 2.7 mTLS 双向认证详解 / mTLS Mutual Authentication Details

文件 / File：`console/backend-go/internal/agent/client.go`

本项目支持完整的 mTLS（双向 TLS）认证，确保客户端与服务端互相验证身份：
This project supports full mTLS (mutual TLS) authentication, ensuring client and server verify each other:

```go
func buildTransportCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
    // 模式 1：TLS 未启用 → 非安全传输（本地开发）/ Mode 1: TLS disabled → insecure (local dev)
    if !cfg.AgentTLSEnabled {
        return insecure.NewCredentials(), nil
    }

    // 模式 2：TLS 启用 → 构造完整 TLS 配置 / Mode 2: TLS enabled → build full TLS config
    // 步骤 1：加载 CA 证书构造受信任根证书池 / Step 1: Load CA cert to build trusted root pool
    caPEM, _ := os.ReadFile(cfg.AgentTLSCAFile)
    certPool := x509.NewCertPool()
    certPool.AppendCertsFromPEM(caPEM)

    tlsConfig := &tls.Config{
        RootCAs:    certPool,           // 校验服务端证书链 / Verify server cert chain
        MinVersion: tls.VersionTLS12,   // 强制最低 TLS 1.2 / Enforce minimum TLS 1.2
    }

    // 步骤 2：可选覆盖 ServerName（证书 SAN 不匹配时）/ Step 2: Optional ServerName override
    if cfg.AgentTLSServerName != "" {
        tlsConfig.ServerName = cfg.AgentTLSServerName
    }

    // 步骤 3：加载客户端证书+私钥用于 mTLS 双向认证 / Step 3: Load client cert+key for mTLS
    if cfg.AgentTLSCertFile != "" && cfg.AgentTLSKeyFile != "" {
        clientCert, _ := tls.LoadX509KeyPair(cfg.AgentTLSCertFile, cfg.AgentTLSKeyFile)
        tlsConfig.Certificates = []tls.Certificate{clientCert}
    }

    return credentials.NewTLS(tlsConfig), nil
}
```

mTLS 环境变量配置 / mTLS Environment Variable Configuration：

| 变量 / Variable | 说明 / Description |
|---|---|
| `PRIVACY_AGENT_TLS_ENABLED` | 启用 TLS（默认 false）/ Enable TLS (default false) |
| `PRIVACY_AGENT_TLS_CA_FILE` | CA 证书路径（必填）/ CA cert path (required) |
| `PRIVACY_AGENT_TLS_CERT_FILE` | 客户端证书路径 / Client cert path |
| `PRIVACY_AGENT_TLS_KEY_FILE` | 客户端私钥路径 / Client key path |
| `PRIVACY_AGENT_TLS_SERVER_NAME` | 覆盖服务端主机名校验 / Override server hostname verification |
| `PRIVACY_AGENT_TLS_INSECURE_SKIP_VERIFY` | 跳过证书校验（仅测试）/ Skip cert verify (test only) |

证书生成工具 / Certificate Generation Tool：`console/backend-go/scripts/gen-certs.sh`

### 2.8 连接保活与重连 / Connection Keepalive & Reconnect

```go
// keepalive 配置：定期发送 HTTP/2 PING 帧检测连接健康状态
// Keepalive config: periodically send HTTP/2 PING frames to detect connection health
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                30 * time.Second, // 每 30s 发送心跳 / Heartbeat every 30s
    Timeout:             10 * time.Second, // 心跳超时 10s 判定断开 / 10s timeout = disconnected
    PermitWithoutStream: true,             // 无活跃 RPC 时也发送心跳 / Heartbeat even without active RPCs
})
```

设计目的：当 Python agent 因 VLM 推理 OOM 等原因崩溃后，keepalive 能在数秒内检测到
连接断开并触发重连，避免后续请求持续失败。
Design purpose: When Python agent crashes (e.g. VLM OOM), keepalive detects disconnection
within seconds and triggers reconnection, preventing subsequent request failures.

### 2.9 Proto 消息类型详解 / Proto Message Type Details

文件 / File：`proto/privacy.proto`（共 503 行，50+ 个 RPC 方法）

核心消息类型与字段设计 / Core message types and field design：

```protobuf
// 通用数据记录：用于表格类操作（脱敏/K-匿名/分类）
// Generic data record: for table operations (masking/K-anonymity/classification)
message RecordEntry {
  map<string, string> fields = 1;  // 列名 → 值 的映射 / Column name → value mapping
}

// 差分隐私请求：支持 Laplace/Gaussian 机制 + 裁剪边界
// DP request: supports Laplace/Gaussian mechanism + clipping bounds
message DPRequest {
  repeated double values = 1;     // 输入数据 / Input data
  double epsilon = 2;             // 隐私预算 ε / Privacy budget ε
  string mechanism = 3;           // 噪声机制: "laplace" | "gaussian" / Noise mechanism
  double delta = 4;               // 松弛参数 δ（Gaussian 必填）/ Relaxation δ (required for Gaussian)
  double clip_lower = 5;          // 裁剪下界 / Clipping lower bound
  double clip_upper = 6;          // 裁剪上界 / Clipping upper bound
}

// 动态分类分级响应：包含多标签 + 审计信息
// Dynamic classification response: multi-tag + audit info
message DynClassificationResponse {
  repeated DynSecurityTagProto tags = 1;  // 安全标签列表 / Security tag list
  string max_level = 2;                   // 最高密级 / Highest level
  string audit_timestamp = 3;             // 审计时间戳 / Audit timestamp
  string engine_layer = 4;                // 引擎层级 / Engine layer
}

message DynSecurityTagProto {
  string level = 1;            // 密级（如 "L3"）/ Level (e.g. "L3")
  string category = 2;         // 分类（如 "个人信息"）/ Category (e.g. "PII")
  string rule_id = 3;          // 规则 ID / Rule ID
  string source_engine = 4;    // 来源引擎 / Source engine
  bool is_override = 7;        // 是否为覆盖型降级标签 / Override downgrade tag
  bool is_downgrade = 8;       // 是否由降级规则产生 / Produced by downgrade rule
  string match_target = 9;     // 匹配目标: field_name | field_value / Match target
}
```

字段编号设计原则 / Field Number Design Principles：
- 字段编号一旦发布不可更改（向后兼容）/ Field numbers immutable after release (backward compat)
- 1-15 用单字节编码，留给高频字段 / 1-15 use single-byte encoding, reserved for frequent fields
- 新增字段使用递增编号，不复用已删除编号 / New fields use incremental numbers, never reuse deleted

### 2.10 gRPC vs REST 对比（本项目视角）/ gRPC vs REST (Project Perspective)

| 维度 / Dimension | Python REST 后端 | Go gRPC 后端 |
|---|---|---|
| 前端 → 后端 | HTTP/JSON | HTTP/JSON（相同）|
| 后端 → Agent | HTTP/REST | gRPC/protobuf |
| 序列化开销 | JSON 文本解析 / JSON text parse | 二进制编解码 / Binary encode/decode |
| 类型安全 | 运行时校验 / Runtime check | 编译期保证 / Compile-time guarantee |
| 连接效率 | 每次新建 HTTP 连接 / New HTTP conn each time | HTTP/2 多路复用 / HTTP/2 multiplexing |
| 前端标识 | `via: "python-rest"` | `via: "go-grpc"` |
| 消息大小限制 | 无显式限制 / No explicit limit | 64 MiB（显式配置）/ 64 MiB (explicit) |
| 心跳检测 | 无 / None | 30s keepalive PING |
| 双向认证 | 不支持 / Not supported | mTLS 可选 / mTLS optional |

### 2.11 gRPC 错误处理 / gRPC Error Handling

gRPC 使用状态码（Status Code）+ 消息描述错误，与 HTTP 状态码不同：

```go
// Go 客户端处理 gRPC 错误 / Go client handling gRPC errors
resp, err := client.Mask(ctx, req)
if err != nil {
    // 提取 gRPC 状态码 / Extract gRPC status code
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.Unavailable:
            // 服务不可达（agent 未启动/崩溃）/ Service unreachable
            return nil, fmt.Errorf("agent 不可达: %s", st.Message())
        case codes.InvalidArgument:
            // 参数校验失败 / Argument validation failed
            return nil, fmt.Errorf("参数错误: %s", st.Message())
        case codes.ResourceExhausted:
            // 隐私预算耗尽 / Privacy budget exhausted
            return nil, fmt.Errorf("预算耗尽: %s", st.Message())
        case codes.DeadlineExceeded:
            // 超时 / Timeout
            return nil, fmt.Errorf("请求超时")
        default:
            return nil, fmt.Errorf("gRPC 错误 [%s]: %s", st.Code(), st.Message())
        }
    }
    return nil, err
}
```

**常用 gRPC 状态码 / Common gRPC Status Codes**：

| 状态码 / Code | 值 / Value | 对应 HTTP | 本项目场景 / Project Scenario |
|---|---|---|---|
| `OK` | 0 | 200 | 成功 / Success |
| `InvalidArgument` | 3 | 400 | 请求参数不合法 / Invalid request params |
| `NotFound` | 5 | 404 | 未知端点 / Unknown endpoint |
| `ResourceExhausted` | 8 | 429 | 隐私预算耗尽 / Budget exhausted |
| `Internal` | 13 | 500 | Agent 内部错误 / Agent internal error |
| `Unavailable` | 14 | 503 | Agent 不可达 / Agent unreachable |
| `DeadlineExceeded` | 4 | 504 | 请求超时 / Request timeout |

### 2.12 序列化机制详解 / Serialization Mechanism Details

Protocol Buffers 的编码方式与 JSON 对比：

```text
JSON 编码（文本）/ JSON Encoding (text):
{"values": [1.5, 2.3], "epsilon": 0.1, "mechanism": "laplace"}
→ 62 字节，人类可读 / 62 bytes, human-readable

Protobuf 编码（二进制）/ Protobuf Encoding (binary):
0x0A 0x02 0x3F 0xC0 0x00 0x00 0x00 0x00 0x00 0x00  # field 1, double 1.5
0x0A 0x02 0x40 0x02 0x66 0x66 0x66 0x66 0x66 0x66  # field 1, double 2.3
0x11 0x9A 0x99 0x99 0x99 0x99 0x99 0xB9 0x3F        # field 2, double 0.1
0x1A 0x07 0x6C 0x61 0x70 0x6C 0x61 0x63 0x65        # field 3, string "laplace"
→ ~40 字节，不可读但紧凑 / ~40 bytes, unreadable but compact
```

**编码规则 / Encoding Rules**：

| 类型 / Type | 编码方式 / Encoding | 特点 / Characteristics |
|---|---|---|
| int32/int64 | Varint（变长整数）| 小值用更少字节 / Smaller values use fewer bytes |
| double/float | 固定 8/4 字节 | IEEE 754 小端序 / Little-endian |
| string/bytes | 长度前缀 + UTF-8 | 长度用 Varint 编码 / Length as Varint |
| repeated | 重复字段标记 | 每个元素独立编码 / Each element encoded separately |
| map | 重复的 key-value 消息 | 内部为嵌套消息 / Internally nested messages |

### 2.13 四种通信模式 / Four Communication Patterns

```text
1. Unary（本项目使用）/ Unary (used in this project):
   客户端 ──── 请求 ────▶ 服务端
   客户端 ◀─── 响应 ──── 服务端
   场景：所有 30+ 个 RPC 方法均为 Unary

2. Server Streaming（本项目未使用）:
   客户端 ──── 请求 ────▶ 服务端
   客户端 ◀─── 流响应 ── 服务端 (多条)
   场景：实时日志、进度报告

3. Client Streaming（本项目未使用）:
   客户端 ──── 流请求 ──▶ 服务端 (多条)
   客户端 ◀─── 响应 ──── 服务端
   场景：大文件分块上传

4. Bidirectional Streaming（本项目未使用）:
   客户端 ◀═══ 双向流 ═══▶ 服务端
   场景：聊天、实时协作
```

### 2.14 拦截器与元数据 / Interceptors & Metadata

```go
// gRPC 拦截器类似 HTTP 中间件，可在调用前后插入逻辑
// gRPC interceptors are like HTTP middleware, insert logic before/after calls

// 客户端拦截器示例（本项目通过 metadata 实现认证）
// Client interceptor example (this project uses metadata for auth)
func (c *Client) WithAuth(ctx context.Context) context.Context {
    if c.cfg.AgentAPIKey == "" {
        return ctx
    }
    // 将 API Key 附加到 gRPC metadata（类似 HTTP Header）
    // Attach API Key to gRPC metadata (like HTTP Header)
    return metadata.AppendToOutgoingContext(ctx,
        "authorization", "Bearer "+c.cfg.AgentAPIKey,
    )
}

// 服务端拦截器示例（Python agent 端）
// Server interceptor example (Python agent side)
// grpc_server.py 中通过 servicer_context 读取 metadata
// Reads metadata via servicer_context in grpc_server.py
```

**Metadata vs HTTP Header 对比 / Metadata vs HTTP Header**：

| 特性 / Feature | gRPC Metadata | HTTP Header |
|---|---|---|
| 传输方式 / Transport | HTTP/2 Header 帧 | HTTP/1.1 文本头 |
| 大小写 / Case | 小写强制 / Lowercase enforced | 不敏感 / Insensitive |
| 二进制值 / Binary values | 支持（-bin 后缀）/ Supported (-bin suffix) | 不支持 / Not supported |
| 多值 / Multi-value | 支持 / Supported | 支持 / Supported |

### 2.15 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 仅使用 Unary 模式 | 隐私计算为请求-响应模式，无需流 / Privacy ops are request-response |
| 64MB 消息上限 | 支持大表格数据（万行级）/ Support large tables (10K+ rows) |
| 显式 keepalive | Agent 可能因 OOM 崩溃，需快速检测 / Agent may OOM crash, need fast detection |
| metadata 认证而非 TLS 客户端证书 | 简化部署，API Key 足够 / Simpler deployment, API Key suffices |
| proto 文件放项目根目录 | Python/Go 两端共享同一定义 / Both Python/Go share same definition |
| 字段编号严格递增 | 保证向后兼容，旧客户端可忽略新字段 / Backward compat, old clients ignore new fields |

## 3. Deadline 与超时管理 / Deadline & Timeout Management

### 3.1 Deadline 工作机制 / Deadline Mechanism

gRPC 的 Deadline 是绝对时间点，而非相对超时（与 HTTP timeout 不同）：

```go
// 设置请求超时 / Set request timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()  // ❗ 必须调用 cancel 释放资源 / Must call cancel to release resources

// Deadline 会随 gRPC 调用自动传播到服务端
// Deadline automatically propagates to server with gRPC call
resp, err := client.Mask(ctx, req)
if err != nil {
    st, _ := status.FromError(err)
    if st.Code() == codes.DeadlineExceeded {
        // 超时：可能是 agent 处理过慢或网络延迟
        // Timeout: agent processing too slow or network latency
        log.Warn("请求超时", "method", "Mask")
    }
}
```

**Deadline 传播链路 / Deadline Propagation Chain**：

```text
┌─────────────────────────────────────────────────────────────┐
│  前端浏览器 / Frontend Browser                                │
│  fetch('/api/proxy', { signal: AbortSignal.timeout(60000) }) │
└──────────────────────────┬──────────────────────────────────┘
                           │  HTTP 请求（60s 超时）
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Go 代理后端 / Go Proxy Backend                               │
│  ctx, cancel := context.WithTimeout(ctx, 30*time.Second)    │
│  // 从 HTTP 请求的 context 派生，继承其 Deadline       │
│  // Derived from HTTP request context, inherits Deadline    │
└──────────────────────────┬──────────────────────────────────┘
                           │  gRPC 调用（30s Deadline）
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Python Agent (gRPC Server)                                   │
│  # 服务端可通过 context.time_remaining() 检查剩余时间  │
│  # Server can check remaining time via context              │
│  # 若剩余时间不足，可提前返回避免无效计算         │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 本项目超时配置 / Project Timeout Configuration

| 层级 / Layer | 超时值 / Timeout | 配置位置 / Config Location |
|---|---|---|
| 前端 fetch | 60s | `client.ts` AbortController |
| Go HTTP 服务器 | 无显式（Gin 默认）| Gin 默认无超时 |
| Go → Agent gRPC | 30s | `context.WithTimeout` |
| gRPC keepalive | 30s 心跳 + 10s 超时 | `keepalive.ClientParameters` |
| Agent 内部处理 | 无限制（同步计算）| 无显式超时 |

### 3.3 超时策略最佳实践 / Timeout Strategy Best Practices

```go
// ✅ 正确：为每个 RPC 调用设置独立超时
// Correct: set independent timeout for each RPC call
func (h *Handler) handleMask(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
    defer cancel()
    resp, err := h.agent.Mask(ctx, req)
    // ...
}

// ✅ 正确：长耗时操作使用更长超时
// Correct: longer timeout for long-running operations
func (h *Handler) handleClassify(c *gin.Context) {
    // LLM 推理可能需要较长时间 / LLM inference may take longer
    ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
    defer cancel()
    // ...
}

// ❌ 错误：使用 context.Background() 无超时
// Wrong: using context.Background() without timeout
resp, err := client.Mask(context.Background(), req)  // 可能永久阻塞 / May block forever
```

## 4. Channel 状态与连接管理 / Channel States & Connection Management

### 4.1 gRPC Channel 状态机 / gRPC Channel State Machine

```text
┌─────────────────────────────────────────────────────────────┐
│  gRPC Channel 状态转换 / Channel State Transitions             │
│                                                             │
│  ┌────────┐   连接成功    ┌───────┐   连接断开    ┌─────────┐ │
│  │  IDLE  │ ─────────▶ │ READY │ ─────────▶ │TRANSIENT│ │
│  │(初始/空闲)│           │(就绪) │           │_FAILURE │ │
│  └────────┘           └───────┘           │(临时失败)│ │
│       │                    ▲               └────┬────┘ │
│       │                    │                    │       │
│       │                    │   重连成功          │       │
│       │                    └────────────────┘       │
│       │                                            │       │
│       │              重连失败(超过重试)            │       │
│       │              Reconnect failed (max retries) │       │
│       ▼                                            ▼       │
│  ┌────────────┐                              ┌──────────┐  │
│  │CONNECTING  │                              │SHUTDOWN  │  │
│  │(连接中)    │                              │(已关闭)  │  │
│  └────────────┘                              └──────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 重连策略 / Reconnection Strategy

gRPC 内置指数退避重连（Exponential Backoff）：

```text
重连时间序列（默认参数）/ Reconnection timeline (default params):

尝试 1: 等待 ~1s    (initial_backoff = 1s)
尝试 2: 等待 ~2s    (backoff × multiplier)
尝试 3: 等待 ~4s
尝试 4: 等待 ~8s
尝试 5: 等待 ~16s
...
最大: 等待 120s     (max_backoff = 120s)

每次等待添加 ±0.2 的随机抖动（jitter）避免雷群效应
Each wait adds ±0.2 random jitter to avoid thundering herd
```

**本项目中的连接管理 / Connection Management in This Project**：

```go
// client.go 中的连接生命周期 / Connection lifecycle in client.go
type Client struct {
    conn   *grpc.ClientConn              // 底层 HTTP/2 连接 / Underlying HTTP/2 conn
    client pb.PrivacyServiceClient       // 生成的服务客户端 / Generated service client
    cfg    *config.Config
}

// 连接复用：所有 RPC 共享同一 TCP 连接（HTTP/2 多路复用）
// Connection reuse: all RPCs share same TCP conn (HTTP/2 multiplexing)
// 无需为每次调用创建新连接，避免 TCP 三次握手开销
// No need to create new conn per call, avoids TCP handshake overhead

// 关闭连接（应用退出时）/ Close connection (on app exit)
func (c *Client) Close() error {
    return c.conn.Close()  // 发送 HTTP/2 GOAWAY 帧 / Sends HTTP/2 GOAWAY frame
}
```

### 4.3 HTTP/2 多路复用优势 / HTTP/2 Multiplexing Advantage

```text
HTTP/1.1（REST 后端）/ HTTP/1.1 (REST backend):
──────────────────────────────────────────────
请求 1 ───────────────────▶ 响应 1
                              请求 2 ───────────▶ 响应 2
                                                    请求 3 ───▶ 响应 3
问题：队头阻塞，串行等待 / Problem: head-of-line blocking, serial wait

gRPC/HTTP/2（Go 后端）/ gRPC/HTTP/2 (Go backend):
──────────────────────────────────────────────
Stream 1: ──请求──▶ ──响应──
Stream 2: ──请求──▶ ──响应──    ← 同一 TCP 连接上并行
Stream 3: ──请求──▶ ──响应──
优势：无队头阻塞，并行处理 / Advantage: no HOL blocking, parallel
```

## 5. Proto 演进与兼容性 / Proto Evolution & Compatibility

### 5.1 向后兼容规则 / Backward Compatibility Rules

```text
┌─────────────────────────────────────────────────────────────┐
│  Proto 兼容性矩阵 / Proto Compatibility Matrix                  │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ✅ 安全的变更（不破坏旧客户端）/ Safe changes:              │
│    - 添加新字段（新编号）/ Add new field (new number)       │
│    - 添加新 RPC 方法 / Add new RPC method                  │
│    - 添加新消息类型 / Add new message type                  │
│    - 重命名字段（保持编号）/ Rename field (keep number)    │
│    - 重命名消息类型 / Rename message type                  │
│                                                             │
│  ❌ 破坏性变更（必须避免）/ Breaking changes (must avoid):   │
│    - 修改字段编号 / Change field number                    │
│    - 修改字段类型 / Change field type                      │
│    - 删除字段后复用编号 / Reuse deleted field number       │
│    - 修改 RPC 方法签名 / Change RPC method signature       │
│                                                             │
│  ⚠️ 需谨慎的变更 / Changes requiring caution:                │
│    - int32 → int64（同类型族，二进制兼容）/ Same type family│
│    - string → bytes（UTF-8 时兼容）/ Compat when UTF-8     │
│    - 单个 → repeated（可解析但语义变化）/ Parseable but     │
│      semantic change                                        │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 字段保留机制 / Field Reservation Mechanism

```protobuf
// 删除字段后保留编号，防止未来误用
// Reserve field numbers after deletion to prevent future misuse
message MaskRequest {
  reserved 4, 7, 9;           // 保留已删除的字段编号 / Reserve deleted field numbers
  reserved "old_strategy";    // 保留已删除的字段名 / Reserve deleted field names

  map<string, string> data = 1;
  string strategy = 2;
  // 字段 4, 7, 9 已删除，不可复用 / Fields 4, 7, 9 deleted, cannot reuse
}
```

### 5.3 版本演进策略 / Version Evolution Strategy

| 策略 / Strategy | 适用场景 / Use Case | 示例 / Example |
|---|---|---|
| 添加可选字段 | 新增功能 / New feature | 添加 `delta` 字段支持 Gaussian |
| 新增 RPC 方法 | 新能力 / New capability | 添加 `DynClassify` 方法 |
| 废弃字段 | 过渡期 / Transition | 添加 `[deprecated = true]` 注解 |
| 新建 v2 服务 | 重大重构 / Major refactor | `PrivacyServiceV2`（本项目未采用）|

## 6. gRPC 健康检查协议 / gRPC Health Checking Protocol

### 6.1 标准健康检查服务 / Standard Health Check Service

gRPC 定义了标准的健康检查协议（`grpc.health.v1`）：

```protobuf
// gRPC 健康检查标准定义（非本项目自定义）
// gRPC health check standard definition (not project-specific)
service Health {
  rpc Check(HealthCheckRequest) returns (HealthCheckResponse);
  rpc Watch(HealthCheckRequest) returns (stream HealthCheckResponse);
}

message HealthCheckResponse {
  enum ServingStatus {
    UNKNOWN = 0;       // 未知状态 / Unknown
    SERVING = 1;       // 服务中 / Serving
    NOT_SERVING = 2;   // 停止服务 / Not serving
    SERVICE_UNKNOWN = 3; // 服务不存在 / Service unknown
  }
  ServingStatus status = 1;
}
```

### 6.2 本项目健康检查实现 / Project Health Check Implementation

```text
┌─────────────────────────────────────────────────────────────┐
│  本项目健康检查架构 / Project Health Check Architecture       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  前端 / Frontend                                            │
│    │  GET /api/health                                      │
│    ▼                                                        │
│  Go 代理 / Go Proxy                                         │
│    ├── 自身状态检查 / Self status check                    │
│    │     - gRPC 连接状态 / gRPC connection state           │
│    │     - 配置加载状态 / Config load status               │
│    └── Agent 健康探测 / Agent health probe                  │
│          │  gRPC Health RPC                                │
│          ▼                                                  │
│  Python Agent                                               │
│    └── 返回 SERVING / Returns SERVING                       │
│                                                             │
│  响应格式 / Response format:                                 │
│  {                                                          │
│    "backend": "ok" | "error",                              │
│    "agent": "ok" | "error" | "unreachable",                │
│    "via": "go-grpc",                                       │
│    "latency_ms": 2.5                                       │
│  }                                                          │
└─────────────────────────────────────────────────────────────┘
```

### 6.3 K8s 健康探针集成 / K8s Health Probe Integration

| 探针类型 / Probe Type | 用途 / Purpose | 本项目配置 / Project Config |
|---|---|---|
| livenessProbe | 检测进程死锁，失败则重启 Pod | HTTP GET /health |
| readinessProbe | 检测服务就绪，失败则从 Service 摘除 | HTTP GET /health |
| startupProbe | 启动期保护（LLM 加载慢）| failureThreshold=30, period=10s |

## 7. 性能基准与优化 / Performance Benchmarks & Optimization

### 7.1 gRPC vs REST 性能对比 / gRPC vs REST Performance

```text
典型隐私计算请求（1000 行表格脱敏）/ Typical privacy op (1000-row masking):

┌─────────────────────────────────────────────────────────────┐
│  指标 / Metric       │  REST (httpx)  │  gRPC (protobuf)  │
├─────────────────────────────────────────────────────────────┤
│  请求体大小 / Req size │  ~45 KB (JSON) │  ~18 KB (binary)  │
│  响应体大小 / Resp size│  ~52 KB (JSON) │  ~22 KB (binary)  │
│  序列化时间 / Ser time │  ~2.1 ms       │  ~0.3 ms          │
│  网络传输 / Transfer   │  ~5 ms         │  ~2 ms            │
│  总延迟 / Total latency│  ~12 ms        │  ~6 ms            │
│  吐吐量 / Throughput   │  ~800 req/s    │  ~2000 req/s      │
└─────────────────────────────────────────────────────────────┘

结论：gRPC 在序列化、传输、吐吐量上均有 2-3x 优势
Conclusion: gRPC has 2-3x advantage in serialization, transfer, throughput
```

### 7.2 性能优化技术 / Performance Optimization Techniques

| 优化技术 / Technique | 说明 / Description | 本项目应用 / Project Usage |
|---|---|---|
| 连接复用 / Connection reuse | HTTP/2 多路复用，避免重复握手 | ✅ 单 ClientConn 共享 |
| 消息压缩 / Message compression | gzip 压缩大消息 | ❌ 未启用（CPU 换带宽）|
| 批量调用 / Batch calls | 合并多个小请求为一个 | ✅ 表格操作一次发送全部行 |
| 消息大小限制 / Message size limit | 避免内存溢出 | ✅ 64MB 显式配置 |
| Keepalive PING | 检测死连接，快速重连 | ✅ 30s 间隔 |
| 连接池 / Connection pool | 多连接并行（高并发）| ❌ 单连接即可（代理场景）|

### 7.3 消息压缩详解 / Message Compression Details

```go
// 启用 gzip 压缩（适用于大表格数据）
// Enable gzip compression (for large table data)
import _ "google.golang.org/grpc/encoding/gzip"  // 注册压缩器 / Register compressor

// 每次调用时指定压缩 / Specify compression per call
resp, err := client.Mask(ctx, req, grpc.UseCompressor("gzip"))

// 压缩效果（典型 1000 行表格）/ Compression effect (typical 1000-row table):
// 原始: 18KB → 压缩后: ~4KB（约 78% 压缩率）
// Original: 18KB → Compressed: ~4KB (~78% compression ratio)
// 代价: 额外 ~0.5ms CPU 时间 / Cost: extra ~0.5ms CPU time
```

**压缩决策指南 / Compression Decision Guide**：

| 场景 / Scenario | 是否压缩 / Compress? | 原因 / Reason |
|---|---|---|
| 小消息 (<4KB) | ❌ 不压缩 | 压缩开销 > 节省的传输时间 |
| 大表格 (>100KB) | ✅ 压缩 | 显著减少网络传输时间 |
| 局域网 / LAN | ❌ 可不压缩 | 带宽充裕，CPU 更宝贵 |
| 跨地域 / Cross-region | ✅ 压缩 | 带宽贵且延迟高 |

## 8. 关键设计决策补充 / Additional Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 单 ClientConn 复用 | HTTP/2 多路复用已足够，无需连接池 / HTTP/2 mux suffices |
| 30s 超时（非无限）| 防止 Agent 崩溃后请求永久挂起 / Prevent permanent hang |
| 不启用压缩 | 本地部署带宽充裕，避免 CPU 开销 / Local deploy, avoid CPU cost |
| context 继承 HTTP 请求 | 前端取消时自动取消 gRPC 调用 / Auto-cancel gRPC on frontend abort |
| 64MB 消息上限 | 支持万行级表格，同时防止 OOM / Support 10K rows, prevent OOM |
| PermitWithoutStream=true | 空闲时也检测连接健康 / Detect conn health even when idle |

## 9. 拦截器与中间件 / Interceptors & Middleware

### 9.1 gRPC 拦截器架构 / Interceptor Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  gRPC 拦截器链（服务端）/ Server Interceptor Chain              │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  客户端请求 / Client Request                                    │
│       │                                                        │
│       ▼                                                        │
│  ┌──────────────────┐                                        │
│  │ Interceptor 1    │  ← 日志拦截器 / Logging interceptor       │
│  │ (outermost)      │                                        │
│  └────────┬─────────┘                                        │
│           ▼                                                    │
│  ┌──────────────────┐                                        │
│  │ Interceptor 2    │  ← 认证拦截器 / Auth interceptor         │
│  └────────┬─────────┘                                        │
│           ▼                                                    │
│  ┌──────────────────┐                                        │
│  │ Interceptor 3    │  ← 指标拦截器 / Metrics interceptor      │
│  │ (innermost)      │                                        │
│  └────────┬─────────┘                                        │
│           ▼                                                    │
│  ┌──────────────────┐                                        │
│  │  Service Handler │  ← 实际业务逻辑 / Actual business logic   │
│  └──────────────────┘                                        │
│                                                                │
│  执行顺序：洋葱模型（先进后出）/ Execution: onion model (LIFO)  │
└────────────────────────────────────────────────────────────────┘
```

### 9.2 Go 服务端拦截器实现 / Go Server Interceptor Implementation

```go
package middleware

import (
    "context"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/metadata"
    "google.golang.org/grpc/status"
)

// LoggingInterceptor 记录每次 RPC 调用的方法、耗时、状态码
// LoggingInterceptor logs method, duration, status for each RPC call
func LoggingInterceptor(
    ctx context.Context,
    req interface{},
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler,
) (interface{}, error) {
    start := time.Now()

    // 调用下一个拦截器或实际 handler / Call next interceptor or handler
    resp, err := handler(ctx, req)

    duration := time.Since(start)
    code := status.Code(err)

    log.Printf("[gRPC] method=%s duration=%v code=%s",
        info.FullMethod, duration, code)

    return resp, err
}

// AuthInterceptor 校验 API Key（可选启用）
// AuthInterceptor validates API Key (optional)
func AuthInterceptor(validKeys map[string]bool) grpc.UnaryServerInterceptor {
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler,
    ) (interface{}, error) {
        // 从 metadata 提取 API Key / Extract API Key from metadata
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return nil, status.Error(codes.Unauthenticated, "missing metadata")
        }

        keys := md.Get("x-api-key")
        if len(keys) == 0 || !validKeys[keys[0]] {
            return nil, status.Error(codes.Unauthenticated, "invalid api key")
        }

        return handler(ctx, req)
    }
}

// 注册拦截器 / Register interceptors
// server := grpc.NewServer(
//     grpc.ChainUnaryInterceptor(
//         LoggingInterceptor,
//         AuthInterceptor(validKeys),
//         MetricsInterceptor,
//     ),
// )
```

### 9.3 Python 服务端拦截器 / Python Server Interceptor

```python
import grpc
import time
import logging
from typing import Callable, Any

logger = logging.getLogger(__name__)

class LoggingInterceptor(grpc.ServerInterceptor):
    """服务端日志拦截器 / Server-side logging interceptor."""

    def intercept_service(self, continuation, handler_call_details):
        # 记录方法名 / Log method name
        method = handler_call_details.method
        start = time.perf_counter()

        # 继续执行 / Continue execution
        next_handler = continuation(handler_call_details)

        duration = time.perf_counter() - start
        logger.info(f"[gRPC] {method} completed in {duration:.3f}s")

        return next_handler


# 本项目 Python Agent 服务端注册 / Python Agent server registration
# server = grpc.server(
#     futures.ThreadPoolExecutor(max_workers=10),
#     interceptors=[LoggingInterceptor()],
# )
```

### 9.4 本项目拦截器实践 / This Project's Interceptor Practice

| 拦截器 / Interceptor | 位置 / Location | 状态 / Status | 说明 / Notes |
|---|---|---|---|
| 日志 / Logging | Go 代理 / Go proxy | ✅ 已实现 | 记录方法+耗时 / Log method+duration |
| 认证 / Auth | Go 代理 / Go proxy | ✅ 可选 | API Key 校验 / API Key validation |
| 指标 / Metrics | Python Agent | ✅ Prometheus | 请求计数+延迟 / Req count+latency |
| 限流 / Rate limit | Python Agent | ✅ 可选 | 令牌桶算法 / Token bucket |
| 超时 / Timeout | Go 客户端 / Go client | ✅ 30s | context deadline / context deadline |

## 10. 流式 RPC 模式 / Streaming RPC Patterns

### 10.1 四种 RPC 模式对比 / Four RPC Pattern Comparison

```text
┌────────────────────────────────────────────────────────────────┐
│  gRPC 四种通信模式 / Four gRPC Communication Patterns           │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  1. Unary（本项目使用）/ Unary (this project uses):              │
│     Client ──[1 req]──► Server ──[1 resp]──► Client           │
│     适用：单次脱敏/DP查询 / Use: single mask/DP query            │
│                                                                │
│  2. Server Streaming:                                           │
│     Client ──[1 req]──► Server ──[resp1]──► Client            │
│                                   ──[resp2]──►                 │
│                                   ──[resp3]──►                 │
│     适用：大结果分批返回 / Use: large result batched return       │
│                                                                │
│  3. Client Streaming:                                           │
│     Client ──[req1]──►                                         │
│            ──[req2]──► Server ──[1 resp]──► Client            │
│            ──[req3]──►                                         │
│     适用：大文件分块上传 / Use: large file chunked upload         │
│                                                                │
│  4. Bidirectional Streaming:                                    │
│     Client ──[req1]──► Server                                  │
│            ◄──[resp1]──                                        │
│            ──[req2]──►                                         │
│            ◄──[resp2]──                                        │
│     适用：实时双向通信 / Use: real-time bidirectional             │
└────────────────────────────────────────────────────────────────┘
```

### 10.2 本项目使用 Unary 的原因 / Why This Project Uses Unary

| 因素 / Factor | 分析 / Analysis |
|---|---|
| 数据量 / Data volume | 单次请求 < 64MB，无需分块 / Single req < 64MB, no chunking needed |
| 交互模式 / Interaction | 请求-响应，无持续流 / Request-response, no continuous stream |
| 复杂度 / Complexity | Unary 最简单，调试方便 / Unary simplest, easy to debug |
| 代理场景 / Proxy scenario | 前端等待完整结果 / Frontend waits for full result |
| 未来扩展 / Future | 大文件可升级为 Server Streaming / Large files can upgrade |

### 10.3 Server Streaming 示例（未来扩展）/ Server Streaming Example (Future)

```protobuf
// 未来可能的流式分类接口 / Future possible streaming classification API
service PrivacyServiceV2 {
  // 大批量分类：服务端流式返回每批结果
  // Batch classification: server streams results per batch
  rpc StreamClassify(StreamClassifyRequest) returns (stream ClassifyBatchResult);
}

message StreamClassifyRequest {
  repeated Record records = 1;     // 万行级数据 / 10K+ rows
  int32 batch_size = 2;            // 每批行数 / Rows per batch
}

message ClassifyBatchResult {
  int32 batch_index = 1;           // 批次索引 / Batch index
  repeated ClassificationResult results = 2;
  bool is_last = 3;                // 是否最后一批 / Is last batch
}
```

```go
// Go 客户端消费流式响应 / Go client consuming stream response
func streamClassify(client pb.PrivacyServiceV2Client, records []*pb.Record) error {
    stream, err := client.StreamClassify(ctx, &pb.StreamClassifyRequest{
        Records:   records,
        BatchSize: 1000,
    })
    if err != nil {
        return err
    }

    for {
        batch, err := stream.Recv()
        if err == io.EOF {
            break  // 流结束 / Stream ended
        }
        if err != nil {
            return err
        }
        // 处理每批结果 / Process each batch
        processBatch(batch)
    }
    return nil
}
```

## 11. 安全与 TLS / Security & TLS

### 11.1 gRPC TLS 架构 / gRPC TLS Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  gRPC TLS 连接建立流程 / gRPC TLS Connection Establishment       │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Go Client                        Python Agent                 │
│     │                                  │                       │
│     │──── TCP SYN ────────────────►│                       │
│     │◄─── TCP SYN-ACK ────────────│                       │
│     │──── TCP ACK ────────────────►│                       │
│     │                                  │                       │
│     │──── ClientHello ─────────────►│  ← TLS 1.2/1.3       │
│     │◄─── ServerHello + Cert ──────│                       │
│     │     [验证服务器证书 / Verify cert] │                       │
│     │──── ClientKeyExchange ───────►│                       │
│     │◄─── Finished ───────────────│                       │
│     │──── Finished ───────────────►│                       │
│     │                                  │                       │
│     │═══ Encrypted gRPC Traffic ═══│  ← HTTP/2 over TLS   │
│     │                                  │                       │
└────────────────────────────────────────────────────────────────┘
```

### 11.2 Go 客户端 TLS 配置 / Go Client TLS Configuration

```go
package agent

import (
    "crypto/tls"
    "crypto/x509"
    "os"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
)

// NewSecureClient 创建 TLS 加密的 gRPC 连接
// NewSecureClient creates a TLS-encrypted gRPC connection
func NewSecureClient(target, caFile string) (*grpc.ClientConn, error) {
    // 加载 CA 证书 / Load CA certificate
    caCert, err := os.ReadFile(caFile)
    if err != nil {
        return nil, err
    }

    certPool := x509.NewCertPool()
    certPool.AppendCertsFromPEM(caCert)

    // 配置 TLS / Configure TLS
    tlsConfig := &tls.Config{
        RootCAs:    certPool,
        MinVersion: tls.VersionTLS12,  // 最低 TLS 1.2 / Minimum TLS 1.2
    }

    creds := credentials.NewTLS(tlsConfig)

    return grpc.NewClient(
        target,
        grpc.WithTransportCredentials(creds),
    )
}

// mTLS 双向认证 / mTLS mutual authentication
func NewMTLSClient(target, caFile, certFile, keyFile string) (*grpc.ClientConn, error) {
    // 加载客户端证书 / Load client certificate
    clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        return nil, err
    }

    caCert, err := os.ReadFile(caFile)
    if err != nil {
        return nil, err
    }

    certPool := x509.NewCertPool()
    certPool.AppendCertsFromPEM(caCert)

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{clientCert},
        RootCAs:      certPool,
        MinVersion:   tls.VersionTLS12,
    }

    creds := credentials.NewTLS(tlsConfig)
    return grpc.NewClient(target, grpc.WithTransportCredentials(creds))
}
```

### 11.3 Python 服务端 TLS 配置 / Python Server TLS Configuration

```python
import grpc
from pathlib import Path
from concurrent import futures

def create_secure_server(
    cert_file: Path,
    key_file: Path,
    ca_file: Path | None = None,
) -> grpc.Server:
    """创建 TLS/mTLS 加密的 gRPC 服务器 / Create TLS/mTLS gRPC server."""
    # 读取证书和密钥 / Read cert and key
    cert_chain = cert_file.read_bytes()
    private_key = key_file.read_bytes()

    # 配置服务器凭证 / Configure server credentials
    if ca_file:
        # mTLS：要求客户端证书 / mTLS: require client cert
        ca_cert = ca_file.read_bytes()
        server_creds = grpc.ssl_server_credentials(
            [(private_key, cert_chain)],
            root_certificates=ca_cert,
            require_client_auth=True,  # 强制双向认证 / Enforce mutual auth
        )
    else:
        # 单向 TLS：仅服务器证书 / One-way TLS: server cert only
        server_creds = grpc.ssl_server_credentials(
            [(private_key, cert_chain)],
        )

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    server.add_secure_port("0.0.0.0:50051", server_creds)
    return server
```

### 11.4 本项目安全实践 / This Project's Security Practice

| 安全层 / Security Layer | 实现 / Implementation | 配置方式 / Config |
|---|---|---|
| 传输加密 / Transport encryption | TLS 1.2+ | `PRIVACY_TLS_ENABLED=true` |
| 双向认证 / Mutual auth | mTLS（可选）/ Optional | `PRIVACY_TLS_CA_FILE` |
| API 认证 / API auth | API Key header | `PRIVACY_AUTH_ENABLED=true` |
| 限流 / Rate limiting | 令牌桶 / Token bucket | `PRIVACY_RATE_LIMIT_ENABLED=true` |
| 消息大小 / Message size | 64MB 上限 / 64MB cap | gRPC MaxRecvMsgSize |
| 证书管理 / Cert management | 手动 / Manual | K8s Secret 挂载 / K8s Secret mount |

### 11.5 证书生成与轮换 / Certificate Generation & Rotation

```bash
# 本项目提供的证书生成脚本 / Project cert generation script
# console/backend-go/scripts/gen-certs.sh

# 1. 生成 CA / Generate CA
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout ca.key -out ca.crt \
  -subj "/CN=PrivacyAgentCA"

# 2. 生成服务器证书 / Generate server cert
openssl req -newkey rsa:4096 -nodes \
  -keyout server.key -out server.csr \
  -subj "/CN=localhost"
openssl x509 -req -in server.csr -days 365 \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt \
  -extfile <(echo "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 3. 生成客户端证书（mTLS）/ Generate client cert (mTLS)
openssl req -newkey rsa:4096 -nodes \
  -keyout client.key -out client.csr \
  -subj "/CN=console-backend"
openssl x509 -req -in client.csr -days 365 \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt
```

| 环境 / Environment | 证书管理 / Cert Management | 轮换策略 / Rotation |
|---|---|---|
| 本地开发 / Local dev | gen-certs.sh 自签名 / Self-signed | 手动（365天）/ Manual (365d) |
| K8s 生产 / K8s prod | cert-manager 自动 / Auto | 自动轮换（90天）/ Auto (90d) |
| Docker Compose | 挂载 volume / Mount volume | 手动更新 / Manual update |

---

## 12. gRPC-Web 与浏览器集成 / gRPC-Web & Browser Integration

### 12.1 gRPC-Web 架构与原理 / gRPC-Web Architecture & Principles

浏览器无法直接使用 HTTP/2 的完整功能，gRPC-Web 是一个适配协议：

```
┌─────────────────────────────────────────────────────────────────┐
│          gRPC-Web 架构 / gRPC-Web Architecture                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  浏览器 / Browser                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  JavaScript Client (grpc-web)                       │   │
│  │  - 使用 HTTP/1.1 或 HTTP/2 (fetch)                  │   │
│  │  - Content-Type: application/grpc-web+proto         │   │
│  │  - 不支持客户端流 / No client streaming              │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          │ HTTP/1.1 or HTTP/2                   │
│                          ▼                                      │
│  代理层 / Proxy Layer                                          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Envoy / Nginx / grpcwebproxy                       │   │
│  │  - 转换 grpc-web ↔ grpc                             │   │
│  │  - 处理 CORS 头 / Handle CORS headers               │   │
│  │  - 协议转换 / Protocol translation                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          │ HTTP/2 (native gRPC)                 │
│                          ▼                                      │
│  gRPC 服务器 / gRPC Server                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  PrivShield (Python grpcio)                │   │
│  │  - 标准 gRPC 服务 / Standard gRPC service           │   │
│  │  - 无需修改 / No modification needed                │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 12.2 前端 gRPC-Web 客户端 / Frontend gRPC-Web Client

```typescript
// ===== 生成 TypeScript 客户端 / Generate TypeScript Client =====
// 使用 protoc + ts_protoc_gen 插件 / Using protoc + ts_protoc_gen plugin

// 生成命令 / Generation command:
// protoc -I proto \
//   --js_out=import_style=commonjs:src/generated \
//   --grpc-web_out=import_style=typescript,mode=grpcwebtext:src/generated \
//   proto/privacy.proto

// ===== 客户端使用 / Client Usage =====
import { PrivacyServiceClient } from './generated/privacy_grpc_web_pb';
import { MaskRequest, MaskResponse } from './generated/privacy_pb';

// 创建客户端 / Create client
const client = new PrivacyServiceClient(
  'http://localhost:8080',  // Envoy 代理地址 / Envoy proxy address
  null,
  { 'X-API-Key': 'your-api-key' }  // 元数据 / Metadata
);

// Unary RPC 调用 / Unary RPC call
function callMask(data: Record<string, unknown>): Promise<MaskResponse> {
  return new Promise((resolve, reject) => {
    const request = new MaskRequest();
    request.setData(JSON.stringify(data));

    client.mask(request, {}, (err, response) => {
      if (err) {
        reject(new Error(`gRPC error: ${err.code} - ${err.message}`));
        return;
      }
      resolve(response);
    });
  });
}

// Promise 风格封装 / Promise-style wrapper
class PrivacyGrpcWebClient {
  private client: PrivacyServiceClient;

  constructor(baseUrl: string) {
    this.client = new PrivacyServiceClient(baseUrl);
  }

  async mask(data: object): Promise<object> {
    const request = new MaskRequest();
    request.setData(JSON.stringify(data));

    return new Promise((resolve, reject) => {
      this.client.mask(request, {}, (err, resp) => {
        if (err) reject(err);
        else resolve(JSON.parse(resp.getMaskedData()));
      });
    });
  }

  async dpQuery(query: string, epsilon: number): Promise<number> {
    const request = new DPQueryRequest();
    request.setQuery(query);
    request.setEpsilon(epsilon);

    return new Promise((resolve, reject) => {
      this.client.dpQuery(request, {}, (err, resp) => {
        if (err) reject(err);
        else resolve(resp.getValue());
      });
    });
  }
}
```

### 12.3 gRPC-Web vs REST 对比 / gRPC-Web vs REST Comparison

| 特性 / Feature | gRPC-Web | REST (JSON) | 本项目选择 / Project Choice |
|---|---|---|---|
| 协议 / Protocol | HTTP/1.1 + 代理 / + proxy | HTTP/1.1 直接 / Direct | REST |
| 序列化 / Serialization | Protobuf (binary) | JSON (text) | JSON |
| 浏览器支持 / Browser | 需要代理 / Needs proxy | 原生 / Native | REST |
| 类型安全 / Type safety | 强类型 / Strong | 弱类型 / Weak | 均可 / Both OK |
| 调试便利 / Debugging | 需要工具 / Needs tools | curl 即可 / curl OK | REST |
| 流式传输 / Streaming | 仅服务器流 / Server only | SSE/WebSocket | REST |
| 复杂度 / Complexity | 高 / High | 低 / Low | REST |

---

## 13. 负载均衡与服务发现 / Load Balancing & Service Discovery

### 13.1 gRPC 负载均衡挑战 / gRPC Load Balancing Challenges

gRPC 使用 HTTP/2 长连接，传统 L4 负载均衡无法有效分配请求：

```
┌─────────────────────────────────────────────────────────────────┐
│       gRPC 负载均衡问题 / gRPC LB Problem                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  问题: L4 LB + HTTP/2 长连接 / Problem: L4 LB + HTTP/2         │
│                                                                 │
│  Client ────────────────────────────────────────────────┐    │
│         │                                               │    │
│         │ 单个 TCP 连接 / Single TCP connection        │    │
│         ▼                                               │    │
│  ┌────────────┐                                      │    │
│  │  L4 LB     │  ──▶  Server 1 (所有请求 / all reqs)  │    │
│  │  (TCP)     │      Server 2 (空闲 / idle)           │    │
│  └────────────┘      Server 3 (空闲 / idle)           │    │
│                                                                 │
│  解决方案 / Solutions:                                          │
│                                                                 │
│  1. L7 LB (Envoy/Nginx)                                        │
│     ┌───────────────────────────────────────────────────┐   │
│     │  Client ──▶ L7 LB ──▶ Server 1                   │   │
│     │              │       ──▶ Server 2  (每请求路由)    │   │
│     │              │       ──▶ Server 3  (per-request)  │   │
│     └───────────────────────────────────────────────────┘   │
│                                                                 │
│  2. 客户端 LB (xDS)                                            │
│     ┌───────────────────────────────────────────────────┐   │
│     │  Client ──▶ Server 1                              │   │
│     │    │     ──▶ Server 2  (客户端直接路由)           │   │
│     │    │     ──▶ Server 3  (client-side routing)     │   │
│     │    └──▶ xDS Server (服务发现 / discovery)         │   │
│     └───────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 13.2 本项目负载均衡策略 / This Project's LB Strategy

```python
# ===== 本项目网关负载均衡 / This Project's Gateway LB =====
# PrivShield/gateway/balancer.py

from enum import Enum
from typing import List
import random
import asyncio

class LBStrategy(str, Enum):
    ROUND_ROBIN = "round_robin"
    RANDOM = "random"
    LEAST_CONNECTIONS = "least_connections"

class Backend:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port
        self.active_connections = 0
        self.healthy = True

    @property
    def address(self) -> str:
        return f"{self.host}:{self.port}"

class LoadBalancer:
    """gRPC 负载均衡器 / gRPC Load Balancer"""

    def __init__(self, backends: List[Backend], strategy: LBStrategy):
        self.backends = backends
        self.strategy = strategy
        self._rr_index = 0

    def select_backend(self) -> Backend:
        """选择后端 / Select backend"""
        healthy = [b for b in self.backends if b.healthy]
        if not healthy:
            raise RuntimeError("No healthy backends available")

        if self.strategy == LBStrategy.ROUND_ROBIN:
            backend = healthy[self._rr_index % len(healthy)]
            self._rr_index += 1
            return backend

        elif self.strategy == LBStrategy.RANDOM:
            return random.choice(healthy)

        elif self.strategy == LBStrategy.LEAST_CONNECTIONS:
            return min(healthy, key=lambda b: b.active_connections)

        raise ValueError(f"Unknown strategy: {self.strategy}")

    async def health_check_loop(self, interval: float = 5.0):
        """健康检查循环 / Health check loop"""
        while True:
            for backend in self.backends:
                try:
                    # gRPC 健康检查 / gRPC health check
                    backend.healthy = await self._check_health(backend)
                except Exception:
                    backend.healthy = False
            await asyncio.sleep(interval)
```

### 13.3 服务发现方案对比 / Service Discovery Comparison

| 方案 / Solution | 适用场景 / Use Case | 复杂度 / Complexity | 本项目 / Project |
|---|---|---|---|
| 静态配置 / Static config | 单实例 / Single instance | 低 / Low | ✅ 主要 / Primary |
| K8s Service | K8s 集群 / K8s cluster | 中 / Medium | ✅ Helm 部署 / Helm deploy |
| Envoy xDS | 大规模微服务 / Large microservices | 高 / High | 未使用 / Not used |
| Consul/etcd | 多数据中心 / Multi-DC | 高 / High | 未使用 / Not used |
| DNS SRV | 简单发现 / Simple discovery | 低 / Low | 可选 / Optional |

---

## 14. Protocol Buffers 编码原理 / Protocol Buffers Encoding Principles

### 14.1 Varint 编码机制 / Varint Encoding Mechanism

Protobuf 使用变长整数编码（Varint）来高效存储整数：

```
┌─────────────────────────────────────────────────────────────────┐
│           Varint 编码原理 / Varint Encoding Principles          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  规则 / Rules:                                                  │
│  - 每字节 7 位有效数据 / 7 bits of data per byte               │
│  - 最高位 (MSB) 是继续标志 / MSB is continuation flag          │
│  - 小数字用更少字节 / Small numbers use fewer bytes            │
│                                                                 │
│  示例 / Examples:                                               │
│                                                                 │
│  数字 1:                                                        │
│  二进制 / Binary: 0000001                                      │
│  Varint:    00000001  (1 字节 / 1 byte)                        │
│             ^ MSB=0, 结束 / end                                 │
│                                                                 │
│  数字 300:                                                      │
│  二进制 / Binary: 100101100                                     │
│  分组 / Group:  0000010  0101100                               │
│  Varint:    10101100 00000010  (2 字节 / 2 bytes)              │
│             ^ MSB=1    ^ MSB=0                                  │
│             继续       结束 / continue, end                     │
│                                                                 │
│  数字 150:                                                      │
│  二进制 / Binary: 10010110                                      │
│  分组 / Group:  0000001  0010110                               │
│  Varint:    10010110 00000001  (2 字节 / 2 bytes)              │
│                                                                 │
│  字节效率 / Byte Efficiency:                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  值范围 / Value Range    │ 字节数 / Bytes │ 节省 / Save │   │
│  │  0 - 127               │ 1           │ 87.5%     │   │
│  │  128 - 16,383          │ 2           │ 75%       │   │
│  │  16,384 - 2,097,151    │ 3           │ 62.5%     │   │
│  │  int32 最大 / max       │ 5           │ 37.5%     │   │
│  │  int64 最大 / max       │ 10          │ 21.9%     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 14.2 消息编码结构 / Message Encoding Structure

```protobuf
// ===== Protobuf 消息编码 / Protobuf Message Encoding =====

// 定义 / Definition:
message MaskRequest {
  string data = 1;        // field_number=1, wire_type=2 (LEN)
  repeated string fields = 2;  // field_number=2, wire_type=2 (LEN)
  double epsilon = 3;     // field_number=3, wire_type=1 (64-bit)
  bool auto_detect = 4;   // field_number=4, wire_type=0 (Varint)
}

// 编码示例 / Encoding Example:
// 消息 / Message: { data: "test", fields: ["email"], epsilon: 0.5, auto_detect: true }

// 字节流 / Byte stream:
// 0A 04 74 65 73 74     -> field 1 (data): "test"
// │  │  └─ UTF-8 内容 / UTF-8 content
// │  └─ 长度 4 / length 4
// └─ (1 << 3) | 2 = 0x0A (field 1, wire type 2)

// 12 05 65 6D 61 69 6C  -> field 2 (fields): ["email"]
// │  │  └─ "email"
// │  └─ 长度 5 / length 5
// └─ (2 << 3) | 2 = 0x12 (field 2, wire type 2)

// 19 00 00 00 00 00 00 E0 3F  -> field 3 (epsilon): 0.5
// │  └─ 8 字节 little-endian double / 8-byte LE double
// └─ (3 << 3) | 1 = 0x19 (field 3, wire type 1)

// 20 01                 -> field 4 (auto_detect): true
// │  └─ varint 1
// └─ (4 << 3) | 0 = 0x20 (field 4, wire type 0)

// 总大小 / Total size: 24 字节 / bytes
// 等价 JSON / Equivalent JSON: ~75 字节 / bytes
// 压缩率 / Compression: ~68%
```

### 14.3 Wire Types 详解 / Wire Types Details

| Wire Type | 名称 / Name | 用于 / Used For | 示例 / Example |
|---|---|---|---|
| 0 | Varint | int32, int64, bool, enum | `int32 count = 1;` |
| 1 | 64-bit | fixed64, double | `double epsilon = 2;` |
| 2 | Length-delimited | string, bytes, message, repeated | `string name = 3;` |
| 5 | 32-bit | fixed32, float | `float ratio = 4;` |

### 14.4 编码优化技巧 / Encoding Optimization Tips

| 技巧 / Tip | 说明 / Description | 效果 / Effect |
|---|---|---|
| 小字段号 / Small field numbers | 1-15 用 1 字节 / 1 byte | 减少消息头开销 / Reduce header overhead |
| 使用 sint32/sint64 | 负数用 ZigZag 编码 / ZigZag for negatives | 负数更高效 / More efficient for negatives |
| packed repeated | 重复字段打包 / Pack repeated fields | 减少字段头 / Reduce field headers |
| 避免大字段号 / Avoid large field numbers | >15 用 2+ 字节 / 2+ bytes | 常用字段用小号 / Common fields small numbers |
| 使用 bytes 而非 string | 已知是 UTF-8 时 / When known UTF-8 | 跳过验证 / Skip validation |
