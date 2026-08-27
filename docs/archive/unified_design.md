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
    end

    subgraph LayerMiddleware ["2. 统一中间件与上下文透传层 (Cross-Cutting Middleware)"]
        TraceMW["TraceID 自动注入与 Header 传递"]
        AuthMW["API Key 鉴权与 Leaky Bucket 限流"]
        EnvelopeMW["统一 JSON 响应信封包裹器"]
    end

    subgraph LayerGovernance ["3. 企业级数据流通调度与存证层 (Services Cluster)"]
        Hub["services/service-hub (:8082)<br/>6 阶段流水线 / Phase B 租约 Worker"]
        DSMgr["services/datasource-mgr (:8083)<br/>多源数据纳管 / 样本切片提取"]
        Audit["services/audit-log (:8084)<br/>9要素防篡改哈希链 / 快照信封加密"]
    end

    subgraph LayerCoreCompute ["4. 核心隐私计算与动态分类引擎 (Core Engine)"]
        Funnel["3-Layer 动态分类漏斗<br/>(Rule → Small-NER → Local LLM)"]
        Primitives["四大隐私原语<br/>(Masking / DP / K-Anon / QoL)"]
        EngineMW["FastAPI 全局异常信封拦截器"]
    end

    subgraph LayerStorageSecurity ["5. 统一存储与密码学基座 (Storage & Crypto Foundations)"]
        SSOT["pkg/naming<br/>(全局唯一事实源)"]
        StoreFacade["pkg/store<br/>(Memory / SQLite / PostgreSQL)"]
        EnvelopeCrypto["pkg/crypto<br/>(AES-256-GCM enc:v1:...)"]
        mTLSAuth["pkg/tlsutil<br/>(TLS 1.3 mTLS + CN 白名单)"]
    end

    WebFull --> BFFGo
    WebAppLZ --> BFFLZ
    BFFGo & BFFLZ --> LayerMiddleware
    LayerMiddleware --> Hub & DSMgr & Audit & LayerCoreCompute
    LayerGovernance --> LayerStorageSecurity
    LayerCoreCompute --> LayerStorageSecurity
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
└───────────────────┴──────────┴───────────────────────┴──────────────────────────────────────────┘
```

---

## 5. 全链路迁移验证与验收测试套件 (Verification DoD)

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
- [x] **全链路追踪**：各服务日志中均输出一致的 `X-Request-ID`。
