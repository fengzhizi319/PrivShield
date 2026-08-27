# PrivShield 全栈统一架构设计再评估与全系统平滑迁移实施方案

> **文档定位**：本文档为 `PrivShield` 体系提供全栈统一架构设计的**深度再评估报告**与**系统级细节迁移落地实施方案（Migration Playbook）**。  
> **版本**：v2.0.0  
> **状态**：🎯 **Target Blueprint & Execution Guide**  
> **覆盖范围**：`engine`（Python 核心隐私引擎）、`services/service-hub`（调度中枢）、`services/datasource-mgr`（数据源管理）、`services/audit-log`（审计存证）、`console/bff-go` & `console/app-lz`（BFF网关与测试执行器）、`console/web` & `console/app-lz/web`（前端控制台群）、`pkg/`（共享基础库）及云原生部署基础设施。

---

## 1. 统一设计顶层再评估与技术代差审计

### 1.1 演进背景与协同现状评估

随着 PrivShield 从最初的**单体 Python 隐私 Sidecar** 演进为**企业级分布式数据安全流通治理中台**，各模块在快速迭代中形成了多语言、多协议、多介质的异构格局。为了实现高内聚、低耦合、零语义漂移的企业级标准，对当前各子系统进行协同度量化评估：

```text
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                各子系统统一设计协同度与成熟度评估矩阵                                │
├──────────────────────┬──────────┬──────────┬──────────┬──────────┬──────────┬────────────────────┤
│ 子系统 / 模块        │ 命名一致性│ 错误信封 │ 分布式追踪│ 存储抽象 │ 零信任安全│ 综合成熟度评级     │
├──────────────────────┼──────────┼──────────┼──────────┼──────────┼──────────┼────────────────────┤
│ **pkg/ 基础共享库**   │ ★★★★★    │ ★★★★☆    │ ★★★★★    │ ★★★★★    │ ★★★★★    │ **Level 5 (准生产)**│
│ **services/audit-log**│ ★★★★★    │ ★★★★☆    │ ★★★★☆    │ ★★★★★    │ ★★★★★    │ **Level 5 (准生产)**│
│ **services/service-hub**│ ★★★★★  │ ★★★★☆    │ ★★★★☆    │ ★★★★★    │ ★★★★☆    │ **Level 5 (准生产)**│
│ **services/datasource-mgr**│ ★★★★☆│ ★★★☆☆   │ ★★★☆☆    │ ★★★★☆    │ ★★★★☆    │ **Level 4 (就绪)** │
│ **console/app-lz**   │ ★★★★★    │ ★★★★☆    │ ★★★★☆    │ ★★★★☆    │ ★★★★☆    │ **Level 5 (就绪)** │
│ **console/bff-go**   │ ★★★★☆    │ ★★★☆☆    │ ★★★☆☆    │ ★★★☆☆    │ ★★★★☆    │ **Level 4 (就绪)** │
│ **engine (Python)**  │ ★★★★☆    │ ★★★☆☆    │ ★★★☆☆    │ ★★★☆☆    │ ★★★★☆    │ **Level 4 (就绪)** │
│ **console 前端群**   │ ★★★★☆    │ ★★★★☆    │ ★★★☆☆    │ N/A      │ N/A      │ **Level 4 (就绪)** │
└──────────────────────┴──────────┴──────────┴──────────┴──────────┴──────────┴────────────────────┘
```

### 1.2 核心技术代差与协同短板诊断

1. **错误响应信封与状态码代差**：
   - Python FastAPI 默认返回 `{"detail": [...]}`，而 Go Gin 返回 `{ "error": "...", "message": "..." }` 或 `{ "code": 400, "msg": "..." }`，前端缺乏单一拦截模型；
2. **追踪上下文（Trace Context）断链风险**：
   - 在高并发与异步 Worker 任务调度场景下，部分协程或 gRPC 调用缺少自动化的 `X-Request-ID` / `traceparent` 上下文继承机制；
3. **数据源命名历史包袱**：
   - 历史代码中偶存裸字符串字面量（如 `"yibao"`、`"kangyang"`），需全面平滑收敛至 `pkg/naming` 的 Canonical ID（`ds_yibao`、`ds_kangyang`）；
4. **单机存储（SQLite）向企业多副本集群（PostgreSQL Phase B）切换的数据平滑割接挑战**：
   - 生产环境中存在存量 SQLite WAL 数据库，需要一套安全无损、保持 9 要素哈希链连续性与 AES-256-GCM 快照密文完整性的迁移割接方案。

---

## 2. 统一设计全景技术架构蓝图

```mermaid
flowchart TD
    subgraph LayerPresentation ["1. 统一表现与接入层 (Presentation & Gateway)"]
        WebFull["console/web<br/>(4大隐私原语 + 分类漏斗)"]
        WebAppLZ["console/app-lz/web<br/>(医保/康养政务流水线)"]
        BFFGo["console/bff-go (:8081)<br/>REST/gRPC 聚合网关"]
        BFFLZ["app-lz/bff-go (:8080)<br/>会话调度与 E2E 测试器"]
        PyGW["engine/gateway<br/>Python 负载均衡网关<br/>(6算法/熔断/重试/动态拓扑)"]
    end

    subgraph LayerMiddleware ["2. 统一中间件与上下文透传层 (Cross-Cutting Middleware)"]
        TraceMW["TraceID 自动注入与 Header 传递"]
        AuthMW["API Key 鉴权与 Leaky Bucket 限流"]
        EnvelopeMW["统一 JSON 响应信封包裹器"]
        DDoSMW["DDoS 防护<br/>(MaxBodySize/MaxConcurrent/RateLimit)"]
        SecHeaders["安全响应头<br/>(CSP/HSTS/X-Frame-Options)"]
    end

    subgraph LayerGovernance ["3. 企业级数据流通调度与存证层 (Services Cluster)"]
        Hub["services/service-hub (:8082)<br/>6 阶段流水线 / Phase B 租约 Worker"]
        DSMgr["services/datasource-mgr (:8083)<br/>多源数据纳管 / 样本切片提取"]
        Audit["services/audit-log (:8084)<br/>9要素防篡改哈希链 / 快照信封加密"]
    end

    subgraph LayerCoreCompute ["4. 核心隐私计算与动态分类引擎 (Core Engine)"]
        Funnel["3-Layer 动态分类漏斗<br/>(Rule → Small-NER → Local LLM)"]
        Primitives["四大隐私原语<br/>(Masking / DP / K-Anon / QoL)"]
        Budget["隐私预算会计<br/>(Epsilon/Delta + 时间窗口重置)"]
        EngineMW["FastAPI 全局异常信封拦截器"]
    end

    subgraph LayerStorageSecurity ["5. 统一存储与密码学基座 (Storage & Crypto Foundations)"]
        SSOT["pkg/naming<br/>(全局唯一事实源)"]
        StoreFacade["pkg/store<br/>(Memory / SQLite / PostgreSQL)"]
        EnvelopeCrypto["pkg/crypto<br/>(AES-256-GCM enc:v1:...)"]
        mTLSAuth["pkg/tlsutil<br/>(TLS 1.3 mTLS + CN 白名单)"]
    end

    subgraph LayerObservability ["6. 全栈可观测性体系 (Observability)"]
        Metrics["Prometheus Metrics<br/>(Python prometheus_client + Go client_golang)"]
        StructLog["结构化日志<br/>(JSON/Text 双格式)"]
        Tracing["OpenTelemetry Tracing<br/>(可选 OTLP 导出)"]
        Grafana["Grafana Dashboard + ServiceMonitor"]
    end

    WebFull --> BFFGo
    WebAppLZ --> BFFLZ
    BFFGo & BFFLZ --> LayerMiddleware
    LayerMiddleware --> Hub & DSMgr & Audit & LayerCoreCompute
    LayerMiddleware --> PyGW
    PyGW --> LayerCoreCompute
    LayerGovernance --> LayerStorageSecurity
    LayerCoreCompute --> LayerStorageSecurity
    LayerCoreCompute --> LayerObservability
    LayerGovernance --> LayerObservability
    LayerMiddleware --> LayerObservability
```

---

## 3. 六大专项技术迁移实施方案 (Detailed Migration Playbooks)

---

### 专项方案 1：跨语言统一 API 错误信封与状态码平滑迁移

#### 1. 迁移目标
消除各微服务（Python + Go）在错误响应上的格式差异，统一输出遵循以下规范的 JSON 响应信封：

```json
{
  "code": "INVALID_DATASOURCE_ID",
  "message": "指定的业务数据源不存在或未激活",
  "detail": "datasource 'ds_unknown' is not registered in canonical naming",
  "trace_id": "req-1787554500-abc12345",
  "timestamp": "2026-08-27T09:30:00.123Z"
}
```

#### 2. 双轨兼容过渡策略 (Dual-Track Compatibility)
为了防止旧版本客户端解析失败，在迁移过渡期采用**双向兼容包装**：
- 响应体中同时保留 `code`（枚举字串）、`message`（人读摘要）、`detail`（兼容原 FastAPI/Gin 的 detail 字段）；
- 响应头中强制下发 `X-Request-ID` 与 `X-Trace-ID`。

#### 3. 详细实施步骤

##### Step 1.1：Python FastAPI 引擎端改造 ([`engine/main.py`](../../engine/main.py))
在 FastAPI 入口注册全局异常处理器，统一捕获 `RequestValidationError`、`HTTPException` 与未捕获异常：

```python
# engine/observability/envelope.py
import time
from datetime import datetime
from fastapi import Request, status
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException

def create_error_envelope(code: str, message: str, detail: any, request: Request, status_code: int) -> JSONResponse:
    trace_id = request.headers.get("X-Request-ID") or request.state.trace_id if hasattr(request.state, "trace_id") else f"req-{int(time.time())}"
    return JSONResponse(
        status_code=status_code,
        headers={"X-Request-ID": trace_id},
        content={
            "code": code,
            "message": message,
            "detail": detail,
            "trace_id": trace_id,
            "timestamp": datetime.utcnow().isoformat() + "Z"
        }
    )

def register_exception_handlers(app):
    @app.exception_handler(RequestValidationError)
    async def validation_handler(request: Request, exc: RequestValidationError):
        return create_error_envelope(
            code="INVALID_ARGUMENT",
            message="请求参数校验失败",
            detail=exc.errors(),
            request=request,
            status_code=status.HTTP_400_BAD_REQUEST
        )

    @app.exception_handler(StarletteHTTPException)
    async def http_exception_handler(request: Request, exc: StarletteHTTPException):
        code_map = {
            400: "INVALID_ARGUMENT",
            401: "UNAUTHORIZED",
            403: "FORBIDDEN",
            404: "NOT_FOUND",
            409: "CONFLICT",
            429: "RATE_LIMITED",
            503: "UPSTREAM_UNAVAILABLE",
        }
        err_code = code_map.get(exc.status_code, "INTERNAL_ERROR")
        return create_error_envelope(
            code=err_code,
            message=str(exc.detail),
            detail=str(exc.detail),
            request=request,
            status_code=exc.status_code
        )
```

##### Step 1.2：Go 中台微服务与 BFF 改造 (`pkg/middleware/envelope.go`)
在 `pkg/middleware` 引入标准响应函数，各 Go 微服务（`service-hub`, `datasource-mgr`, `audit-log`, `bff-go`, `app-lz`）统一调用：

```go
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Detail    any    `json:"detail,omitempty"`
	TraceID   string `json:"trace_id"`
	Timestamp string `json:"timestamp"`
}

func AbortWithError(c *gin.Context, httpStatus int, code string, message string, detail any) {
	traceID := GetTraceID(c)
	c.Header("X-Request-ID", traceID)
	c.AbortWithStatusJSON(httpStatus, ErrorEnvelope{
		Code:      code,
		Message:   message,
		Detail:    detail,
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
```

##### Step 1.3：前端 Axios 全局拦截器对齐 (`console/app-lz/web/src/api/client.ts`)
```typescript
apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const res = error.response;
    if (res && res.data && res.data.code) {
      const { code, message, detail, trace_id } = res.data;
      console.error(`[PrivShield API Error] ${code} (TraceID: ${trace_id}): ${message}`);
      // 触发全局 Toast 提示
      window.dispatchEvent(new CustomEvent('privshield:api-error', {
        detail: { code, message, detail, trace_id }
      }));
    }
    return Promise.reject(error);
  }
);
```

---

### 专项方案 2：全链路分布式追踪 (Trace Context) 贯穿迁移

#### 1. 迁移目标
确保由前端生成的 `X-Request-ID`，在跨越 HTTP REST、Go 内部调度流水线、gRPC 跨机调用、异步 Goroutine 消费以及 Audit Log 存证数据库落盘的全生命周期中**保持绝对单调且不丢失**。

```text
┌────────────────┐  HTTP: X-Request-ID  ┌────────────────┐  gRPC Metadata  ┌────────────────┐
│  前端 React UI  │ ───────────────────▶ │   BFF / Hub    │ ───────────────▶ │ Engine / Audit │
│ (生成 TraceID)  │                      │ (Context 注入) │  (x-request-id) │ (日志结构化输出)│
└────────────────┘                      └────────────────┘                 └────────────────┘
```

#### 2. 详细实施步骤

##### Step 2.1：HTTP Trace 中间件 (`pkg/middleware/trace.go`)
```go
package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

const TraceIDKey = "PrivShield-Trace-ID"
const TraceHeader = "X-Request-ID"

func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(TraceHeader)
		if traceID == "" {
			traceID = fmt.Sprintf("req-%d-%06x", time.Now().Unix(), time.Now().Nanosecond()%0x1000000)
		}
		c.Set(TraceIDKey, traceID)
		c.Header(TraceHeader, traceID)
		c.Header("X-Trace-ID", traceID)
		c.Next()
	}
}

func GetTraceID(c *gin.Context) string {
	if val, ok := c.Get(TraceIDKey); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return c.GetHeader(TraceHeader)
}
```

##### Step 2.2：gRPC 客户端与服务端双向拦截器
- **Go 客户端外发拦截器 (`pkg/agent/grpc_client.go`)**：
  ```go
  func (c *Client) attachTraceMD(ctx context.Context) context.Context {
      traceID, _ := ctx.Value(middleware.TraceIDKey).(string)
      if traceID == "" {
          traceID = fmt.Sprintf("req-%d", time.Now().Unix())
      }
      return metadata.AppendToOutgoingContext(ctx, "x-request-id", traceID, "x-trace-id", traceID)
  }
  ```
- **Python gRPC 服务端元数据提取 (`engine/grpc_server.py`)**：
  ```python
  def _extract_trace_id(context: grpc.ServicerContext) -> str:
      for key, val in context.invocation_metadata():
          if key in ("x-request-id", "x-trace-id"):
              return val
      return f"req-{int(time.time())}"
  ```

##### Step 2.3：异步任务 Worker 上下文继承 (`services/service-hub/internal/handlers/handlers.go`)
在任务分发时，将 `TraceID` 显式持久化至 `models.Task.TraceID`，Worker 消费时还原为 `context.Context`，杜绝孤儿日志。

---

### 专项方案 3：业务标识统一与别名归一化迁移 (SSOT Naming)

#### 1. 迁移目标
彻底消除全栈代码中对数据源名称、API 编号的硬编码，将所有识别、校验与展示逻辑统一收敛至 [`pkg/naming`](../../pkg/naming/)。

#### 2. 平滑迁移矩阵与废弃端点兼容

| 历史/别名标识 | Canonical 数据源 ID | 对应 API 编码 | 兼容处理策略 |
|---|---|---|---|
| `"yibao"`, `"yibao.csv"`, `"医保"` | `ds_yibao` (常量: `naming.DSYibao`) | `api1_yibao` | 边界自动归一化，返回 `Warning: 299 Deprecated alias` |
| `"kangyang"`, `"kangyang.csv"`, `"康养"` | `ds_kangyang` (常量: `naming.DSKangyang`) | `api2_kangyang` | 边界自动归一化，返回 `Warning: 299 Deprecated alias` |
| 任意未知标识 (如 `"custom_test"`) | 拦截拒绝 | N/A | **Fail-Closed 阻断**，返回 `400 INVALID_DATASOURCE_ID` |

#### 3. 自动化检查与静态代码扫描规则
在 Makefile 中增加 `naming-lint` 检查命令，扫描业务代码中是否包含硬编码字面量：
```bash
# 检查 Go 代码中是否存在裸字符串 "ds_yibao" 而非 naming.DSYibao
! grep -rn '"ds_yibao"' services/ console/ | grep -v 'pkg/naming'
```

---

### 专项方案 4：存储底座 Phase A (SQLite) 到 Phase B (PostgreSQL) 生产平滑迁移

#### 1. 迁移目标与挑战
在单机环境下，PrivShield 使用 SQLite WAL 模式（`service-hub.db` 与 `audit-log.db`）。当升级到多节点企业级高并发集群时，需切换至 PostgreSQL Phase B 存储底座。  
**核心挑战**：必须保证存量审计日志在割接过程中 **9 要素连续哈希链不断链**，且 **AES-256-GCM 快照密文无损解密与验真**。

```text
┌───────────────────────┐                               ┌──────────────────────────┐
│  Phase A (SQLite WAL) │                               │ Phase B (PostgreSQL)     │
├───────────────────────┤                               ├──────────────────────────┤
│ • service-hub.db      │  ─── 迁移工具平滑割接 ───▶    │ • 表: tasks (行级锁租约) │
│ • audit-log.db        │      (哈希链完整性校验)        │ • 表: audit_logs (连续链)│
│ • snapshots.db        │                               │ • 表: snapshots (信封密文)│
└───────────────────────┘                               └──────────────────────────┘
```

#### 2. 数据迁移与割接实施流程

##### Step 4.1：准备 PostgreSQL 生产表结构与索引
执行建表脚本（已在 `pkg/store/postgres/audit.go` 与 `task.go` 固化）：
```sql
-- 启用 UUID 扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 任务表 (Phase B 租约争抢)
CREATE TABLE IF NOT EXISTS tasks (
    id VARCHAR(128) PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    stage VARCHAR(32) NOT NULL,
    source VARCHAR(64) NOT NULL,
    operation VARCHAR(32) NOT NULL,
    priority INT NOT NULL DEFAULT 50,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    duration_ms BIGINT DEFAULT 0,
    error TEXT DEFAULT '',
    retry_count INT DEFAULT 0,
    lease_owner VARCHAR(128) DEFAULT '',
    lease_expire TIMESTAMPTZ,
    payload JSONB
);
CREATE INDEX IF NOT EXISTS idx_tasks_lease ON tasks (status, priority DESC, lease_expire);

-- 审计日志表 (9 要素连续哈希链)
CREATE TABLE IF NOT EXISTS audit_logs (
    id VARCHAR(128) PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL,
    datasource VARCHAR(64) NOT NULL,
    api_code VARCHAR(64),
    operation VARCHAR(32) NOT NULL,
    input_hash VARCHAR(128) NOT NULL,
    output_hash VARCHAR(128) NOT NULL,
    algorithm VARCHAR(64) NOT NULL,
    user_name VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL,
    security_level VARCHAR(16) NOT NULL,
    params_json TEXT,
    snapshot_id VARCHAR(128),
    prev_hash VARCHAR(128) NOT NULL,
    integrity_hash VARCHAR(128) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_prev_hash ON audit_logs (prev_hash);

-- 快照加密表 (AES-256-GCM)
CREATE TABLE IF NOT EXISTS snapshots (
    id VARCHAR(128) PRIMARY KEY,
    log_id VARCHAR(128) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    datasource VARCHAR(64) NOT NULL,
    operation VARCHAR(32) NOT NULL,
    input_sample TEXT NOT NULL,
    output_sample TEXT NOT NULL,
    parameters TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
```

##### Step 4.2：开发专用数据迁移与连续性核验脚本 (`scripts/prod/migrate_sqlite_to_pg.go`)
迁移脚本执行以下关键原子步骤：
1. **只读锁定源库**：暂停外部写流量或在只读副本上执行抽取；
2. **按哈希链顺序流式抽取**：`SELECT * FROM audit_logs ORDER BY rowid ASC`；
3. **逐条重算 9 要素哈希链**：验证每一条记录的 `prev_hash` 是否严格等于上一条的 `integrity_hash`；
4. **批量注入 PostgreSQL (`pgx.Batch`)**：单批次 500 条原子提交；
5. **迁移后验真**：在 PostgreSQL 上立即调用 `store.VerifyChain(0)` 全量验真，断链立即报警并回滚。

##### Step 4.3：生产环境变量切换与平滑重启
```bash
# 1. 注入 PostgreSQL 环境变量
export AUDIT_LOG_PG_DSN="postgres://audit_user:audit_pass@pg-prod:5432/privshield_audit?sslmode=verify-full&pool_max_conns=50"
export SERVICE_HUB_PG_DSN="postgres://hub_user:hub_pass@pg-prod:5432/privshield_hub?sslmode=verify-full&pool_max_conns=50"

# 2. 启动服务，自动激活 Phase B 存储后端
./services/audit-log/cmd/server/main &
./services/service-hub/cmd/server/main &
```

---

### 专项方案 5：零信任通信与 mTLS CN 白名单动态热重载迁移

#### 1. 迁移目标
将静态编译在代码或单机环境变量中的证书 CN 列表，迁移为基于动态配置文件的 **微服务访问控制白名单 (`mtls-whitelist.yaml`)**，支持在不停机的情况下通过文件监听（`fsnotify`）实现毫秒级授权热生效。

#### 2. 白名单配置文件标准结构 (`config/mtls-whitelist.yaml`)
```yaml
# PrivShield 微服务内部 mTLS 访问控制白名单
version: "1.0"
updated_at: "2026-08-27T09:00:00Z"

clients:
  - cn: "bff-go.privshield.internal"
    role: "gateway"
    description: "主控制台 BFF 代理网关"
    allowed_scopes:
      - "*"

  - cn: "app-lz-bff.privshield.internal"
    role: "lz_gateway"
    description: "调度之眼控制台 BFF"
    allowed_scopes:
      - "/ServiceHub/*"
      - "/AuditLog/*"
      - "/DatasourceMgr/*"

  - cn: "service-hub.privshield.internal"
    role: "orchestrator"
    description: "数据调度中枢"
    allowed_scopes:
      - "/PrivacyService/Process"
      - "/AuditLog/RecordAudit"
      - "/DatasourceMgr/FetchSlice"

  - cn: "external-hospital-client"
    role: "data_consumer"
    description: "外部医院端调用方（仅限脱敏接口）"
    allowed_scopes:
      - "/ServiceHub/DispatchTask"
```

#### 3. 动态热重载与权限校验实现 (`pkg/tlsutil/whitelist.go`)
```go
package tlsutil

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

type DynamicWhitelist struct {
	mu      sync.RWMutex
	clients map[string][]string // CN -> Scopes
	path    string
}

func NewDynamicWhitelist(path string) (*DynamicWhitelist, error) {
	dw := &DynamicWhitelist{
		clients: make(map[string][]string),
		path:    path,
	}
	if err := dw.reload(); err != nil {
		return nil, err
	}
	go dw.watch()
	return dw, nil
}

func (dw *DynamicWhitelist) reload() error {
	data, err := os.ReadFile(dw.path)
	if err != nil {
		return err
	}
	var conf struct {
		Clients []struct {
			CN     string   `yaml:"cn"`
			Scopes []string `yaml:"allowed_scopes"`
		} `yaml:"clients"`
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}

	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.clients = make(map[string][]string)
	for _, c := range conf.Clients {
		dw.clients[c.CN] = c.Scopes
	}
	log.Printf("[mTLS Whitelist] Successfully reloaded %d authorized CN entries", len(dw.clients))
	return nil
}

func (dw *DynamicWhitelist) watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()
	_ = watcher.Add(dw.path)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				_ = dw.reload()
			}
		case <-watcher.Errors:
			return
		}
	}
}

// UnaryServerInterceptor 提供 gRPC 双向证书 CN 校验与授权
func (dw *DynamicWhitelist) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		p, ok := peer.FromContext(ctx)
		if !ok || p.AuthInfo == nil {
			return nil, status.Error(codes.Unauthenticated, "missing peer authentication info")
		}
		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
			return nil, status.Error(codes.Unauthenticated, "invalid or unverified client certificate")
		}

		clientCN := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
		dw.mu.RLock()
		scopes, exists := dw.clients[clientCN]
		dw.mu.RUnlock()

		if !exists {
			log.Printf("[mTLS Auth Failed] Unauthorized Client CN: %s", clientCN)
			return nil, status.Errorf(codes.PermissionDenied, "client CN '%s' is not authorized", clientCN)
		}

		// 校验 Scope
		authorized := false
		for _, s := range scopes {
			if s == "*" || s == info.FullMethod {
				authorized = true
				break
			}
		}
		if !authorized {
			return nil, status.Errorf(codes.PermissionDenied, "client CN '%s' lacks scope for method '%s'", clientCN, info.FullMethod)
		}

		return handler(ctx, req)
	}
}
```

---

### 专项方案 6：前端双控制台（Web & App-LZ）组件与规范收敛迁移

#### 1. 迁移目标与职责边界划分
- **`console/web`（全量隐私控制台）**：面向数据安全工程师，提供 4 大通用隐私原语、三层漏斗策略调优与算子性能基准测试；
- **`console/app-lz/web`（数联调度之眼）**：面向数据要素流通与业务运营，聚焦 `ds_yibao`（医保）与 `ds_kangyang`（康养）真实数据流水线、租约状态监控、字段手风琴对比与自动化测试大屏。

#### 2. UI 规范收敛实施
1. **状态指示器统一标准**：
   - `completed`: 翠绿色背景（`bg-emerald-500/10 text-emerald-400 border-emerald-500/20`）+ `IconCheckCircle`；
   - `running`: 靛蓝色背景（`bg-indigo-500/10 text-indigo-400 border-indigo-500/20`）+ 呼吸光晕圆点；
   - `failed`: 玫瑰红背景（`bg-rose-500/10 text-rose-400 border-rose-500/20`）+ `IconXCircle`；
   - `pending`: 蓝灰色背景（`bg-slate-800 text-slate-400 border-slate-700`）。
2. **预设数据 API 动态渲染**：
   - 彻底废除前端写死 API 列表的逻辑，统一通过 `GET /api/lz/data-api/definitions` 动态拉取卡片列表，自动适配未来新增的 `ds_xx1` 等新数据源。

---

## 4. 迁移风险矩阵、回滚预案与应急响应 (Rollback Playbook)

```text
┌─────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                   迁移风险矩阵与应急回滚预案                                        │
├───────────────────┬──────────┬───────────────────────┬──────────────────────────────────────────┤
│ 潜在故障风险       │ 严重等级 │ 触发指征              │ 应急处置与一键回滚操作                   │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **1. 错误信封解析** │ High     │ 前端抛出 Unhandled     │ 设置 `FEATURE_FLAG_LEGACY_ERROR=true`，  │
│ 导致老客户端报错   │          │ Exception 或无法渲染  │ 网关自动退化为兼容双轨模式                │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **2. PostgreSQL** │ Critical │ PG 写入超时、租约争抢 │ 清除 `PG_DSN` 环境变量，自动回滚至本地    │
│ 数据库连接池耗尽   │          │ 报错连接拒绝          │ SQLite WAL 模式，保障核心流通不中断       │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **3. 审计哈希链断裂**│ Critical │ VerifyChain 响应返回   │ 运行 `repair_hash_chain` 工具重新锚定断点│
│ (Broken Chain)    │          │ broken_at_id 异常     │ 记录，恢复区块链式连续性                  │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **4. mTLS 热重载** │ High     │ 正常客户端报 403 /    │ 恢复 `mtls-whitelist.yaml.bak` 备份文件， │
│ 配置文件格式损坏   │          │ PermissionDenied      │ 监听器毫秒级自动热更新重载恢复            │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **5. 隐私预算耗尽** │ High     │ DP 查询返回 429 /     │ 配置 `PRIVACY_BUDGET_WINDOW_SECONDS` 自动  │
│ 导致服务不可用     │          │ BudgetExhausted 异常  │ 重置；多实例部署启用 `PRIVACY_BUDGET_DB`   │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **6. 审计日志膨胀** │ Medium   │ 磁盘空间告警 /        │ 配置 `AUDIT_LOG_RETENTION_DAYS`（默认 90） │
│ 超出存储配额       │          │ 查询延迟上升          │ 自动清理超期记录；PG 启用分区表            │
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **7. LLM 推理 OOM** │ High     │ 进程崩溃 /            │ `PRIVACY_LLM_MAX_CONCURRENCY` 信号量限流； │
│ 或推理超时         │          │ OOM Killer 杀进程     │ `PRIVACY_LLM_MIN_FREE_MEM_MB` 内存阈值降级│
├───────────────────┼──────────┼───────────────────────┼──────────────────────────────────────────┤
│ **8. 网关后端全部熔断**│ High  │ 所有请求返回 503     │ 主动健康检查持续探测，半开状态单请求恢复； │
│                    │          │                       │ 全部节点故障时检查后端进程与网络连通性    │
└───────────────────┴──────────┴───────────────────────┴──────────────────────────────────────────┘
```

---

## 5. 全栈迁移验证与验收测试套件 (Verification DoD)

迁移完成后，需依次执行以下验收测试套件，满足 **100% 通过（Definition of Done）** 准则：

### 5.1 自动化测试命令清单

```bash
# 1. 运行所有 Go 共享库与核心微服务测试
go test -v ./pkg/... ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/... ./console/bff-go/... ./console/app-lz/bff-go/...

# 2. 运行 Python 核心隐私引擎测试
PYTHONPATH=. pytest tests/ -q

# 3. 执行端到端全链路集成测试（含真实 HTTP/gRPC 调用）
bash ./scripts/dev/integration-test-new-modules.sh

# 4. 执行 App-LZ 自动化测试套件（TS-01 ~ TS-04）
PRIVSHIELD_E2E=1 go test -v -run TestRunSuites ./console/app-lz/bff-go/internal/runner/

# 5. 前端编译与类型检查
cd console/app-lz/web && pnpm build
cd ../../web && pnpm build
```

### 5.2 核心业务功能验收标准
- [x] **SSOT 唯一事实源**：`ds_yibao` 与 `ds_kangyang` 全链路无字面量硬编码，新增数据源 5 步即可上线；
- [x] **9 要素哈希链与验真**：`POST /api/audit/chain/verify` 返回 `valid: true`，哈希链无任何断裂；
- [x] **信封加密**：数据库中快照样本全部携带 `enc:v1:` 密文前缀，读取时透明还原；
- [x] **Phase B 租约并发**：20 个并发任务无死锁、无重复执行（TS-03 100% 通过）；
- [x] **全链路追踪**：各服务日志中均输出一致的 `X-Request-ID`；
- [x] **Prometheus 指标暴露**：Python `/metrics` 与 Go `/metrics` 均可抓取，包含请求计数、延迟直方图、隐私原语操作计数；
- [x] **DDoS 防护中间件**：所有 Go 服务启用 `MaxBodySize` + `MaxConcurrent` + `RateLimit`，Python 启用 `limit_concurrency` + `limit_max_requests`；
- [x] **优雅停机**：所有服务捕获 SIGTERM/SIGINT，在途请求排空完成后再退出；
- [x] **熔断器保护**：Agent 客户端与 Gateway 负载均衡器均具备三态熔断器（Closed/Open/Half-Open）；
- [x] **数据保留策略**：审计日志超期自动清理（`AUDIT_LOG_RETENTION_DAYS`，默认 90 天）。

---

## 6. 全栈可观测性体系设计 (Observability Architecture)

> 原设计仅覆盖了分布式追踪（专项 2），缺失了 Prometheus 指标体系、结构化日志规范与 OpenTelemetry 集成设计。本节补齐。

### 6.1 Prometheus 指标体系

#### Python 引擎端 (`engine/observability/metrics.py`)

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `privacy_requests_total` | Counter | `method`, `path`, `status` | REST/gRPC 请求计数 |
| `privacy_request_duration_seconds` | Histogram | `method`, `path` | 请求延迟分布（P50/P95/P99） |
| `privacy_dp_queries_total` | Counter | `mechanism`, `noise` | 差分隐私查询计数 |
| `privacy_classification_results_total` | Counter | `layer`, `level` | 分类漏斗各层结果计数 |
| `privacy_masking_operations_total` | Counter | `field_type` | 脱敏操作计数 |
| `privacy_kano_operations_total` | Counter | `algorithm` | K-匿名操作计数 |
| `privacy_qol_operations_total` | Counter | `strategy` | 查询混淆操作计数 |
| `privacy_gateway_healthy_nodes` | Gauge | — | 网关健康后端节点数 |
| `privacy_gateway_retries_total` | Counter | `node` | 网关重试计数 |
| `privacy_gateway_circuit_breaker_state` | Gauge | `node` | 熔断器状态（0=closed, 1=open, 2=half_open） |

#### Go 微服务端 (`pkg/metrics/metrics.go`)

| 指标名称 | 类型 | 标签 | 用途 |
|---|---|---|---|
| `https_requests_total` | Counter | `method`, `path`, `status` | HTTP 请求计数 |
| `http_request_duration_seconds` | Histogram | `method`, `path` | HTTP 请求延迟 |
| `agent_requests_total` | Counter | `method`, `status` | Agent gRPC 调用计数 |
| `agent_request_duration_seconds` | Histogram | `method` | Agent gRPC 调用延迟 |
| `orphaned_tasks_recovered_total` | Counter | — | 崩溃恢复时回收的孤儿任务数 |
| `tasks_retried_total` | Counter | — | 自动重试的任务数 |
| `circuit_breaker_state` | Gauge | `target` | Agent 客户端熔断器状态 |
| `task_lease_conflicts` | Counter | — | 租约争抢冲突计数 |

每个 Go 服务使用独立的 `prometheus.Registry`，避免全局注册冲突。暴露 `/metrics` 端点供 Prometheus 或 ServiceMonitor 抓取。

### 6.2 结构化日志规范

#### Python 引擎
- 通过 `PRIVACY_LOG_FORMAT` 环境变量切换 `text`（开发）或 `json`（生产）格式。
- JSON 格式使用 `python-json-logger`，每条日志自动注入 `service`、`trace_id`、`timestamp` 字段。
- 所有隐私操作日志强制携带 `extra={"trace_id": ...}` 上下文。

#### Go 微服务
- 使用标准 `log/slog` 结构化日志，JSON 格式输出。
- 每条日志自动注入 `trace_id`、`service`、`component` 字段。
- 审计日志额外携带 `integrity_hash` 与 `prev_hash` 用于哈希链验真。

### 6.3 OpenTelemetry 分布式追踪

Python 引擎可选启用 OpenTelemetry（`engine/observability/tracing.py`）：

```text
┌────────────────┐     OTLP/gRPC      ┌──────────────────┐
│  Python Engine  │ ─────────────────▶ │ Jaeger / Tempo   │
│  (SpanExporter) │                    │ (Trace Backend)  │
└────────────────┘                    └──────────────────┘
```

- 通过 `OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量激活，未设置时为 no-op。
- 支持 `BatchSpanProcessor` 批量导出，减少网络开销。
- Span 自动关联 `X-Request-ID`，与 Go 端 TraceMiddleware 形成完整调用链。

### 6.4 Grafana 仪表盘与告警

- 预置仪表盘模板：`deploy/grafana/dashboard.json` 与 `deploy/grafana/service-hub-dashboard.json`。
- K8s 部署通过 `ServiceMonitor` CRD 自动注册 Prometheus 抓取目标（`deploy/helm/PrivShield/templates/servicemonitor.yaml`）。
- 推荐告警规则：
  - `privacy_requests_total{status=~"5.."}` 5 分钟速率突增 → P1 告警
  - `privacy_gateway_healthy_nodes == 0` → P0 告警（全后端不可用）
  - `circuit_breaker_state > 0` 持续 5 分钟 → P2 告警（后端异常）

---

## 7. 韧性与安全加固设计 (Resilience & Security Hardening)

> 原设计缺失跨服务韧性模式（熔断/重试/降级）、DDoS 防护中间件层、优雅停机协议与隐私预算会计模型的设计说明。本节补齐。

### 7.1 跨服务韧性模式

#### 7.1.1 熔断器（Circuit Breaker）

系统中有两处关键熔断器实现：

| 位置 | 保护目标 | 参数 |
|---|---|---|
| `pkg/agent/client.go` | Agent gRPC 客户端 → Engine | 连续失败 5 次触发，30 秒冷却 |
| `engine/gateway/balancer.py` | Gateway → 多后端 Engine 节点 | 连续失败 5 次触发，30 秒冷却，半开单探测许可证 |

三态模型：`Closed`（正常）→ `Open`（熔断）→ `Half-Open`（单请求探测恢复）。

#### 7.1.2 重试策略

| 组件 | 重试条件 | 最大次数 | 退避策略 |
|---|---|---|---|
| Gateway HTTP 代理 | 幂等方法或 ConnectError | 3 | 指数退避 + 随机抖动 |
| Gateway gRPC 代理 | UNAVAILABLE 或未知异常 | 3 | 指数退避 + 随机抖动 |
| BFF-Go → Agent gRPC | gRPC 服务配置 `retryPolicy` | 按配置 | 指数退避 |
| Service-Hub → Datasource | HTTP 连接失败 | 按配置 | 指数退避 |

#### 7.1.3 分类漏斗降级链

```
Layer-1 Rule Engine (确定性规则匹配)
  ↓ 低置信度
Layer-2 Small-NER (轻量实体识别，ONNX Runtime)
  ↓ 仍低于阈值
Layer-3 Local LLM (本地大模型仲裁，可选)
  ↓ LLM 不可用或内存不足
Conservative Fallback (保守回退，不降级安全等级)
```

降级触发条件：
- `PRIVACY_LLM_MIN_FREE_MEM_MB`：系统可用内存低于阈值时跳过 LLM 层
- `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS`：LLM 推理信号量等待超时后降级
- NER/LLM 模型加载失败：缓存错误，后续调用直接走降级路径

### 7.2 DDoS 防护与安全中间件层

#### Go 微服务中间件栈 (`pkg/middleware/`)

所有 Go 服务（service-hub, datasource-mgr, audit-log, bff-go, app-lz）统一启用以下中间件链：

| 中间件 | 功能 | 配置参数 |
|---|---|---|
| `MaxBodySize(maxBytes)` | 限制请求体大小，防止大包 OOM | 32 MB (`32 << 20`) |
| `MaxConcurrent(limit)` | 限制在途请求总数，防止并发耗尽资源 | 按服务配置 |
| `RateLimit(rps, burst)` | 每客户端 IP 令牌桶限流 | 按服务配置 |
| `SecurityHeaders()` | 注入 CSP/HSTS/X-Frame-Options/X-Content-Type-Options | 固定值 |
| `CORS(origins)` | 可配置跨域来源 | 环境变量 |
| `TraceMiddleware()` | 自动注入/传播 X-Request-ID | — |

#### Python 引擎防护

| 参数 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_LIMIT_CONCURRENCY` | 10000 | Uvicorn 最大并发连接数 |
| `PRIVACY_LIMIT_MAX_REQUESTS` | 100000 | 单连接最大请求数（防内存泄漏） |
| `PRIVACY_TIMEOUT_KEEP_ALIVE` | 30 | 空闲连接超时（秒） |
| Python `RateLimitInterceptor` | 按路径配置 | gRPC 拦截器级限流 |

### 7.3 优雅停机协议

#### Go 服务

```text
SIGTERM/SIGINT 到达
  → 停止接收新连接
  → 排空在途请求（最长 shutdown_timeout 秒）
  → 持久化未完成任务状态到 SQLite/PG
  → 关闭数据库连接池
  → 退出（exit 0）
```

所有 Go 服务使用 `signal.NotifyContext` 监听 SIGINT/SIGTERM，通过 `http.Server.Shutdown(ctx)` 或 `grpcServer.GracefulStop()` 实现排空。

#### Python 服务

所有 4 个 Python 入口（`main.py`, `server.py`, `launcher.py`, `gateway/server.py`）统一使用 uvicorn 的 `timeout_graceful_shutdown` 参数（默认 10 秒）。Python gRPC 独立模式使用 `server.stop(grace=5)` 排空在途 RPC。

### 7.4 隐私预算会计模型

`BudgetAccountant`（`engine/privacy/budget.py`）提供严格的差分隐私预算管理：

| 能力 | 实现 |
|---|---|
| 命名空间隔离 | 每个 `namespace` 独立追踪 epsilon/delta 消耗 |
| 预算耗尽保护 | 累计消耗超过上限时抛出 `PrivacyBudgetExhaustedError` |
| 时间窗口自动重置 | `PRIVACY_BUDGET_WINDOW_SECONDS` 配置周期重置 |
| 多实例一致性 | `PRIVACY_BUDGET_DB` (SQLite/PG) 支持跨实例预算同步 |
| 审计日志 | `BudgetAuditLogger` 记录每次 epsilon/delta 支出到防篡改日志 |

### 7.5 数据生命周期管理

| 数据类型 | 保留策略 | 清理机制 |
|---|---|---|
| 审计日志 (`audit_logs`) | `AUDIT_LOG_RETENTION_DAYS`（默认 90 天） | 超期自动清理，保留哈希链完整性 |
| 任务记录 (`tasks`) | 按服务配置 | 已完成任务定期归档 |
| 隐私预算日志 | 永久保留 | 仅追加，不删除 |
| 快照加密数据 | 跟随审计日志 | AES-256-GCM 密文随日志一同清理 |

---

## 8. 生产部署基础设施设计 (Production Deployment Infrastructure)

> 原设计仅提及 Docker/Helm/K8s 的基本安装命令，缺失 K8s 生产级基础设施的架构设计。本节补齐。

### 8.1 K8s 生产级能力矩阵

| 能力 | Helm 模板 | 生产启用条件 |
|---|---|---|
| 水平自动扩缩 (HPA) | `templates/hpa.yaml` | `autoscaling.enabled=true`，CPU 70% / 内存 80% 阈值，2~10 副本 |
| 潮汐预测扩缩 (CronHPA) | `templates/cron-hpa.yaml` | 业务高峰期定时扩容 |
| Pod 中断预算 (PDB) | `templates/pdb.yaml` | `podDisruptionBudget.enabled=true`，保障滚动更新时最小可用副本数 |
| 网络策略 (NetworkPolicy) | `templates/networkpolicy.yaml` | `networkPolicy.enabled=true`，同命名空间隔离 |
| Prometheus 集成 (ServiceMonitor) | `templates/servicemonitor.yaml` | `serviceMonitor.enabled=true`，自动注册抓取目标 |
| 启动探针 (startupProbe) | `templates/deployment.yaml` | 保护慢启动应用（ML 模型加载），最长 150 秒 |
| 存活探针 (livenessProbe) | `templates/deployment.yaml` | `/health` 端点，周期性检查 |
| 就绪探针 (readinessProbe) | `templates/deployment.yaml` | `/api/health` 端点，检查上游连通性 |

### 8.2 数据库 Schema 迁移策略

当前采用**增量 ALTER TABLE** 模式（`pkg/store/sqlite/init.go` 与 `pkg/store/postgres/schema.go`）：

- **Phase A (SQLite)**：服务启动时自动执行 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`，幂等安全。
- **Phase B (PostgreSQL)**：使用 `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 增量演进。
- **迁移工具**：`scripts/prod/migrate_sqlite_to_pg.go` 提供 SQLite → PostgreSQL 的原子割接，带哈希链完整性校验。

> **设计改进方向**：当 Schema 变更频率增加时，应引入正式的迁移框架（如 `golang-migrate` 或 `goose`），
> 支持版本号追踪、回滚和 CI 集成。当前增量 ALTER 模式适用于低频变更阶段。

### 8.3 API 版本控制策略

当前代码库使用 `/v1/` 路径前缀（如 `/v1/privacy/mask`、`/v1/dynclassification/classify`），但尚未制定正式的 API 版本演进策略。

**推荐策略**：
- URL 路径版本控制：`/v1/...` → `/v2/...`
- 旧版本至少维护 2 个发布周期后标记 Deprecated
- BFF 层负责版本路由与协议转换
- gRPC 通过 `.proto` 文件的 `package` 版本实现向后兼容
