# Pydantic 技术栈说明 / Pydantic Technology Stack

## 1. 技术简介 / Introduction

Pydantic 是 Python 最流行的数据校验库，基于类型提示实现运行时数据验证与序列化。
Pydantic is the most popular data validation library for Python, using type hints for runtime data validation and serialization.

核心特性 / Core Features：
- **类型驱动校验（Type-driven Validation）**：利用 Python 类型注解自动校验数据。
- **高性能（High Performance）**：Pydantic v2 核心用 Rust 重写，校验速度提升 5-50 倍。
- **序列化/反序列化（Serialization）**：自动 JSON schema 生成、model_dump()、model_validate()。
- **设置管理（Settings Management）**：pydantic-settings 从环境变量/.env 加载配置。
- **与 FastAPI 深度集成**：FastAPI 的请求/响应模型直接基于 Pydantic。

本项目使用版本 / Version Used：`pydantic >= 2.6.0` + `pydantic-settings >= 2.2.0`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 请求/响应模型 / Request/Response Models

文件 / File：`console/bff-py/app/main.py`

```python
from pydantic import BaseModel, Field

class ProxyRequest(BaseModel):
    """通用代理请求体 - Pydantic 自动校验字段类型与约束。
    Generic proxy request - Pydantic auto-validates field types and constraints."""
    method: str = Field(..., examples=["POST"])           # 必填字段 / Required field
    path: str = Field(..., examples=["/v1/privacy/mask"]) # 必填字段 / Required field
    body: dict[str, Any] | None = Field(default=None)     # 可选字段 / Optional field
    raw_payload_b64: str | None = Field(default=None)
    content_type: str | None = Field(default=None)

class LbTestRequest(BaseModel):
    """负载均衡测试请求 - 展示 Field 约束能力。
    LB test request - demonstrates Field constraint capabilities."""
    backends: list[LbBackend] = Field(default_factory=list)  # 避免可变默认参数 / Avoid mutable default
    num_requests: int = Field(default=10, ge=1, le=1000)     # 范围约束 1~1000 / Range constraint
    strategy: str = Field(default="round_robin")
```

### 2.2 配置管理 / Configuration Management

文件 / File：`console/bff-py/app/config.py`

```python
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict

class Settings(BaseSettings):
    """从环境变量加载配置 - 零配置即可本地运行。
    Load config from env vars - zero-config local development."""

    # 通过 alias 映射到环境变量名 / Map to env var names via alias
    privacy_agent_url: str = Field(default="http://127.0.0.1:8079", alias="PRIVACY_AGENT_URL")
    console_port: int = Field(default=8080, alias="PRIVACY_CONSOLE_PORT")
    max_upload_bytes: int = Field(default=10 * 1024 * 1024, alias="CONSOLE_MAX_UPLOAD_BYTES")

    model_config = SettingsConfigDict(
        env_file=".env",              # 支持 .env 文件 / Support .env file
        env_file_encoding="utf-8",
        populate_by_name=True,        # 允许按字段名或 alias 赋值 / Allow by name or alias
    )

# 全局单例：模块导入时即完成环境变量解析
# Global singleton: env vars parsed at import time
settings = Settings()
```

### 2.3 Pydantic 在本项目中的三重角色 / Triple Role in This Project

| 角色 / Role | 实现 / Implementation | 收益 / Benefit |
|---|---|---|
| **输入校验 / Input Validation** | 请求体自动校验类型与约束 | 拦截非法输入，减少 400 错误处理代码 |
| **配置管理 / Config Management** | pydantic-settings + 环境变量 | 零配置启动 + 12-Factor App 合规 |
| **序列化 / Serialization** | 响应模型自动转 JSON | 类型安全的 API 响应 |

### 2.4 Pydantic v2 新特性使用 / Pydantic v2 New Features Used

本项目充分利用 Pydantic v2 的新 API：
This project fully leverages Pydantic v2's new APIs:

```python
# v2 风格：model_config 替代旧版 class Config / v2 style: model_config replaces old class Config
class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",              # .env 文件支持 / .env file support
        env_file_encoding="utf-8",    # 编码指定 / Encoding specification
        populate_by_name=True,        # 允许字段名或 alias 赋值 / Allow by name or alias
    )

# v2 序列化 API / v2 serialization API
response_data = model.model_dump()          # 转为 dict / Convert to dict
json_str = model.model_dump_json()          # 转为 JSON 字符串 / Convert to JSON string
validated = Model.model_validate(raw_dict)  # 从 dict 校验创建 / Validate and create from dict

# v2 JSON Schema 生成（FastAPI 自动文档依赖）/ v2 JSON Schema generation (FastAPI auto-docs)
schema = ProxyRequest.model_json_schema()   # 生成 OpenAPI 兼容 schema / Generate OpenAPI schema
```

### 2.5 Field 约束与验证器 / Field Constraints & Validators

```python
from pydantic import BaseModel, Field, field_validator

class LbTestRequest(BaseModel):
    # 数值范围约束：防止 DoS 级超大批量 / Numeric range: prevent DoS-level huge batches
    num_requests: int = Field(default=10, ge=1, le=1000)

    # 字符串枚举约束（通过 Literal 或 validator）/ String enum constraint
    strategy: str = Field(default="round_robin")

    @field_validator("strategy")
    @classmethod
    def validate_strategy(cls, v: str) -> str:
        """only allow known strategies / 仅允许已知策略"""
        allowed = {"round_robin", "random", "least_connections"}
        if v not in allowed:
            raise ValueError(f"strategy must be one of {allowed}")
        return v

# 校验失败时 FastAPI 自动返回 422 + 详细错误信息
# On validation failure, FastAPI auto-returns 422 + detailed errors
# {
#   "detail": [{
#     "type": "less_than_equal",
#     "loc": ["body", "num_requests"],
#     "msg": "Input should be less than or equal to 1000",
#     "input": 99999
#   }]
# }
```

### 2.6 安全防线 / Security Line of Defense

Pydantic 模型是本项目输入安全的第一道防线：
Pydantic models are the first line of input security:

| 防护层 / Protection Layer | 实现 / Implementation | 效果 / Effect |
|---|---|---|
| 类型校验 / Type validation | `method: str`、`body: dict` | 拒绝非 JSON 对象输入 / Reject non-JSON input |
| 范围约束 / Range constraint | `ge=1, le=1000` | 防止 DoS 级超大批量 / Prevent DoS-level batches |
| 必填字段 / Required fields | `Field(...)` | 缺少关键字段时返回 422 / Return 422 when missing |
| 默认值安全 / Safe defaults | `default_factory=list` | 避免可变默认参数陷阱 / Avoid mutable default trap |
| 自动 422 响应 / Auto 422 | FastAPI 集成 | 无需手写校验代码 / No manual validation code |

### 2.7 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| Pydantic v2 而非 v1 | Rust 核心性能提升 5-50x / Rust core 5-50x faster |
| pydantic-settings | 12-Factor App 配置规范 / 12-Factor App config compliance |
| Field 约束而非手写 if | 声明式、自动文档、不可绕过 / Declarative, auto-docs, unbypassable |
| model_config 而非 class Config | v2 推荐方式，更好的类型提示 / v2 recommended, better type hints |
| populate_by_name=True | 测试时可直接用字段名构造 / Tests can construct by field name |

### 2.8 校验性能与 Rust 核心 / Validation Performance & Rust Core

Pydantic v2 的核心校验逻辑用 Rust 重写（`pydantic-core`）：

```text
┌─────────────────────────────────────────────────────────────┐
│  Python 层 / Python Layer                                    │
│  - 模型定义 / Model definition                               │
│  - 验证器装饰器 / Validator decorators                       │
│  - 序列化 API / Serialization API                            │
└──────────────────────────┬──────────────────────────────────┘
                           │ FFI 调用 / FFI call
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Rust 核心 (pydantic-core) / Rust Core                       │
│  - 类型校验 / Type validation                                │
│  - 约束检查 / Constraint checking                            │
│  - JSON 解析 / JSON parsing                                  │
│  - 序列化 / Serialization                                    │
│  性能：比纯 Python 快 5-50x / 5-50x faster than pure Python │
└─────────────────────────────────────────────────────────────┘
```

**性能基准 / Performance Benchmarks**：

| 操作 / Operation | Pydantic v1 | Pydantic v2 | 提升 / Improvement |
|---|---|---|---|
| 简单模型校验 / Simple model validation | ~10μs | ~1μs | **10x** |
| 嵌套模型 / Nested models | ~50μs | ~5μs | **10x** |
| JSON 序列化 / JSON serialization | ~15μs | ~2μs | **7.5x** |
| 大列表校验 / Large list validation | ~500μs | ~20μs | **25x** |

### 2.9 JSON Schema 生成 / JSON Schema Generation

Pydantic 自动生成符合 OpenAPI 规范的 JSON Schema：

```python
# 自动生成 JSON Schema（FastAPI 文档依赖）/ Auto-generate JSON Schema (FastAPI docs)
schema = ProxyRequest.model_json_schema()
# 输出 / Output:
# {
#   "title": "ProxyRequest",
#   "type": "object",
#   "required": ["method", "path"],
#   "properties": {
#     "method": {"type": "string", "examples": ["POST"]},
#     "path": {"type": "string", "examples": ["/v1/privacy/mask"]},
#     "body": {"anyOf": [{"type": "object"}, {"type": "null"}]},
#     ...
#   }
# }

# FastAPI 自动在 /docs 和 /openapi.json 中暴露这些 schema
# FastAPI auto-exposes these schemas at /docs and /openapi.json
```

**Schema 生成流程 / Schema Generation Flow**：

```text
Pydantic 模型 / Pydantic Model
    │
    ▼  model_json_schema()
JSON Schema (Draft 2020-12)
    │
    ▼  FastAPI 集成 / FastAPI integration
OpenAPI 3.1.0 规范 / OpenAPI 3.1.0 spec
    │
    ├──▶ /docs (Swagger UI)
    └──▶ /redoc (ReDoc)
```

### 2.10 模型继承与组合 / Model Inheritance & Composition

```python
# 基础模型 / Base model
class BaseRequest(BaseModel):
    """所有请求的公共字段 / Common fields for all requests"""
    request_id: str | None = Field(default=None, description="请求追踪 ID / Request trace ID")

# 继承扩展 / Inheritance extension
class ProxyRequest(BaseRequest):
    """代理请求：继承 request_id + 新增代理字段"""
    method: str
    path: str
    body: dict[str, Any] | None = None

# 组合模式 / Composition pattern
class BatchRequest(BaseModel):
    """批量请求：包含多个子请求 / Batch: contains multiple sub-requests"""
    requests: list[ProxyRequest]  # 复用已有模型 / Reuse existing model
    parallel: bool = Field(default=False)
```

### 2.11 错误响应格式 / Error Response Format

Pydantic 校验失败时，FastAPI 自动返回结构化错误：

```json
// 请求 / Request: {"method": "POST", "path": "/v1/mask", "num_requests": 99999}
// 响应 / Response: 422 Unprocessable Entity
{
  "detail": [
    {
      "type": "less_than_equal",     // 错误类型 / Error type
      "loc": ["body", "num_requests"], // 错误位置 / Error location
      "msg": "Input should be less than or equal to 1000",  // 可读消息 / Readable message
      "input": 99999,                // 原始输入 / Original input
      "ctx": {"le": 1000}            // 约束上下文 / Constraint context
    }
  ]
}
```

**错误处理最佳实践 / Error Handling Best Practices**：

| 实践 / Practice | 说明 / Description |
|---|---|
| 不捕获 ValidationError | 让 FastAPI 自动处理并返回 422 / Let FastAPI auto-handle and return 422 |
| 前端解析 detail 数组 | 可定位具体字段错误 / Can locate specific field errors |
| 使用 examples 字段 | Swagger UI 中显示示例值 / Show example values in Swagger UI |
| 约束而非正则 | ge/le/min_length 比 regex 更可读 / More readable than regex |

## 3. 高级校验模式 / Advanced Validation Patterns

### 3.1 判别联合（Discriminated Union）/ Discriminated Union

Pydantic v2 支持基于字段值的联合类型自动分发：

```python
from typing import Annotated, Literal, Union
from pydantic import BaseModel, Field

# 定义不同类型的事件 / Define different event types
class MaskEvent(BaseModel):
    """脱敏事件 / Masking event"""
    type: Literal["mask"] = "mask"  # 判别字段 / Discriminator field
    field_name: str
    strategy: str = "partial"

class DPEvent(BaseModel):
    """差分隐私事件 / DP event"""
    type: Literal["dp"] = "dp"
    epsilon: float = Field(ge=0.01, le=10.0)
    mechanism: str = "laplace"

class KAnonEvent(BaseModel):
    """K-匿名事件 / K-anonymity event"""
    type: Literal["kanon"] = "kanon"
    k: int = Field(ge=2, le=100)

# 使用 Annotated + Field(discriminator) 创建判别联合
# Use Annotated + Field(discriminator) to create discriminated union
PrivacyEvent = Annotated[
    Union[MaskEvent, DPEvent, KAnonEvent],
    Field(discriminator="type")  # 根据 type 字段自动选择模型 / Auto-select model by type field
]

class EventBatch(BaseModel):
    events: list[PrivacyEvent]

# 校验时自动根据 type 值选择对应模型 / Auto-select model by type value during validation
batch = EventBatch.model_validate({
    "events": [
        {"type": "mask", "field_name": "phone"},
        {"type": "dp", "epsilon": 1.0},
        {"type": "kanon", "k": 5},
    ]
})
# batch.events[0] 是 MaskEvent 实例 / is MaskEvent instance
# batch.events[1] 是 DPEvent 实例 / is DPEvent instance
```

**判别联合 vs 普通联合 / Discriminated Union vs Plain Union**：

| 特性 / Feature | 判别联合 / Discriminated | 普通联合 / Plain Union |
|---|---|---|
| 校验速度 / Validation speed | O(1) 直接定位 / Direct lookup | O(n) 逐个尝试 / Try each one |
| 错误信息 / Error messages | 精确到具体模型 / Precise to model | 模糊（所有模型错误）/ Vague |
| 适用场景 / Use case | 有明确类型标识 / Clear type tag | 类型可重叠 / Overlapping types |

### 3.2 model_validator 模型级校验 / Model-level Validation

```python
from pydantic import BaseModel, model_validator

class LbTestRequest(BaseModel):
    """负载均衡测试请求 - 展示跨字段校验。
    LB test request - demonstrates cross-field validation."""
    backends: list[dict]
    num_requests: int = Field(default=10, ge=1, le=1000)
    strategy: str = "round_robin"
    timeout_per_request: float = Field(default=5.0, ge=0.1)

    @model_validator(mode="after")
    def validate_total_time(self) -> "LbTestRequest":
        """跨字段校验：总耗时不能超过 300s。
        Cross-field validation: total time must not exceed 300s."""
        max_total = self.num_requests * self.timeout_per_request
        if max_total > 300:
            raise ValueError(
                f"Total timeout {max_total}s exceeds 300s limit. "
                f"Reduce num_requests or timeout_per_request."
            )
        return self

    @model_validator(mode="before")
    @classmethod
    def normalize_backends(cls, data: dict) -> dict:
        """前置校验：规范化输入数据。
        Pre-validation: normalize input data."""
        if isinstance(data.get("backends"), str):
            # 支持逗号分隔的字符串格式 / Support comma-separated string format
            data["backends"] = [
                {"url": url.strip()} for url in data["backends"].split(",")
            ]
        return data
```

**校验器执行顺序 / Validator Execution Order**：

```text
输入数据 / Input data
    │
    ▼  mode="before" model_validator
前置模型校验（原始 dict）/ Pre-model validation (raw dict)
    │
    ▼  字段类型校验 / Field type validation
各字段类型转换与约束检查 / Field type conversion & constraint check
    │
    ▼  @field_validator
字段级自定义校验 / Field-level custom validation
    │
    ▼  mode="after" model_validator
后置模型校验（已校验的模型实例）/ Post-model validation (validated instance)
    │
    ▼
输出模型实例 / Output model instance
```

### 3.3 序列化定制 / Serialization Customization

```python
from pydantic import BaseModel, Field, field_serializer
from datetime import datetime

class ProxyResponse(BaseModel):
    """统一响应包装 - 展示序列化定制。
    Unified response wrapper - demonstrates serialization customization."""
    status: int
    duration_ms: float
    data: Any
    timestamp: datetime = Field(default_factory=datetime.now)
    via: str = "python-rest"

    @field_serializer("duration_ms")
    def serialize_duration(self, v: float) -> float:
        """保留 2 位小数 / Round to 2 decimal places"""
        return round(v, 2)

    @field_serializer("timestamp")
    def serialize_timestamp(self, v: datetime) -> str:
        """ISO 8601 格式 / ISO 8601 format"""
        return v.isoformat(timespec="milliseconds")

# 序列化示例 / Serialization example
resp = ProxyResponse(status=200, duration_ms=123.456789, data={"ok": True})
resp.model_dump()
# {"status": 200, "duration_ms": 123.46, "data": {"ok": True},
#  "timestamp": "2024-01-15T10:30:45.123", "via": "python-rest"}

# 排除字段 / Exclude fields
resp.model_dump(exclude={"timestamp"})

# 仅包含特定字段 / Include only specific fields
resp.model_dump(include={"status", "data"})

# JSON 序列化（自动处理 datetime 等类型）/ JSON serialization (auto handles datetime)
resp.model_dump_json(indent=2)
```

### 3.4 泛型模型 / Generic Models

```python
from typing import Generic, TypeVar
from pydantic import BaseModel

T = TypeVar("T")

class PaginatedResponse(BaseModel, Generic[T]):
    """通用分页响应 - 支持任意数据类型的分页包装。
    Generic paginated response - supports any data type pagination wrapper."""
    items: list[T]
    total: int
    page: int = 1
    page_size: int = 20

    @property
    def total_pages(self) -> int:
        return (self.total + self.page_size - 1) // self.page_size

# 具体化泛型 / Concrete generic
class MaskResult(BaseModel):
    field: str
    masked_value: str

# 使用具体类型 / Use with concrete type
response = PaginatedResponse[MaskResult](
    items=[MaskResult(field="phone", masked_value="138****1234")],
    total=1,
)
# response.items[0].masked_value → "138****1234"  # 类型安全 / Type safe
```

### 3.5 计算字段与属性 / Computed Fields & Properties

```python
from pydantic import BaseModel, computed_field

class BatchResponse(BaseModel):
    """批量响应 - 展示计算字段。
    Batch response - demonstrates computed fields."""
    total: int
    succeeded: int
    failed: int
    total_duration_ms: float

    @computed_field  # 序列化时自动包含 / Auto-included during serialization
    @property
    def success_rate(self) -> float:
        """成功率（百分比）/ Success rate (percentage)"""
        return round(self.succeeded / max(self.total, 1) * 100, 1)

    @computed_field
    @property
    def avg_duration_ms(self) -> float:
        """平均耗时 / Average duration"""
        return round(self.total_duration_ms / max(self.total, 1), 2)

# model_dump() 自动包含计算字段 / model_dump() auto-includes computed fields
resp = BatchResponse(total=10, succeeded=8, failed=2, total_duration_ms=500.0)
resp.model_dump()
# {"total": 10, "succeeded": 8, "failed": 2, "total_duration_ms": 500.0,
#  "success_rate": 80.0, "avg_duration_ms": 50.0}
```

## 4. Pydantic 与 FastAPI 协作机制 / Pydantic & FastAPI Collaboration

### 4.1 请求处理管道 / Request Processing Pipeline

```text
HTTP 请求体 (JSON bytes)
HTTP Request Body (JSON bytes)
        │
        ▼  Uvicorn 解析 / Uvicorn parse
Python dict (raw)
        │
        ▼  FastAPI 路由参数解析 / FastAPI route param resolution
Pydantic model_validate()
        │
        ├── 类型转换 / Type coercion
        ├── 约束检查 / Constraint checking
        ├── 自定义校验器 / Custom validators
        └── 默认值填充 / Default value filling
        │
        ▼
校验通过的模型实例 / Validated model instance
        │
        ▼  处理器执行 / Handler execution
业务逻辑 / Business logic
        │
        ▼  响应序列化 / Response serialization
Pydantic model_dump_json()
        │
        ▼
HTTP 响应体 (JSON bytes)
HTTP Response Body (JSON bytes)
```

### 4.2 响应模型与状态码 / Response Model & Status Code

```python
from fastapi import FastAPI, status

app = FastAPI()

@app.post(
    "/api/proxy",
    response_model=ProxyResponse,           # 响应模型：自动序列化 + 文档 / Auto serialize + docs
    status_code=status.HTTP_200_OK,         # 成功状态码 / Success status code
    responses={
        422: {"description": "校验失败 / Validation error"},
        502: {"description": "上游不可达 / Upstream unreachable"},
    },
)
async def proxy(req: ProxyRequest) -> ProxyResponse:
    ...
```

### 4.3 依赖注入中的 Pydantic / Pydantic in Dependency Injection

```python
from fastapi import Depends, Query
from pydantic import BaseModel

class PaginationParams(BaseModel):
    """分页参数模型 - 用于依赖注入。
    Pagination params model - for dependency injection."""
    page: int = Field(default=1, ge=1)
    size: int = Field(default=20, ge=1, le=100)

    @property
    def offset(self) -> int:
        return (self.page - 1) * self.size

async def get_pagination(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=20, ge=1, le=100),
) -> PaginationParams:
    """FastAPI 依赖：从查询参数构造 Pydantic 模型。
    FastAPI dependency: construct Pydantic model from query params."""
    return PaginationParams(page=page, size=size)

@app.get("/api/history")
async def list_history(pagination: PaginationParams = Depends(get_pagination)):
    # pagination.offset 自动计算 / Auto-computed offset
    ...
```

## 5. 性能优化与最佳实践 / Performance Optimization & Best Practices

### 5.1 模型定义性能影响 / Model Definition Performance Impact

| 模式 / Pattern | 性能影响 / Performance Impact | 建议 / Recommendation |
|---|---|---|
| 简单字段 / Simple fields | 极快（Rust 核心）/ Very fast | 优先使用 / Prefer |
| Field 约束 / Field constraints | 极快 / Very fast | 优先使用 / Prefer |
| field_validator | 快（Python 回调）/ Fast | 适度使用 / Use moderately |
| model_validator(mode="after") | 中等 / Medium | 跨字段校验时使用 / Use for cross-field |
| model_validator(mode="before") | 较慢（原始 dict）/ Slower | 仅数据规范化时用 / Only for normalization |
| 深层嵌套模型 / Deep nested models | 较慢 / Slower | 控制嵌套层级 ≤ 3 / Keep nesting ≤ 3 |

### 5.2 常见陷阱与解决 / Common Pitfalls & Solutions

| 陷阱 / Pitfall | 问题 / Problem | 解决 / Solution |
|---|---|---|
| 可变默认参数 / Mutable default | `items: list = []` 共享引用 / Shared reference | `default_factory=list` |
| 循环引用 / Circular reference | 模型互相引用导致递归 / Models reference each other | `model_rebuild()` 延迟解析 |
| 过度校验 / Over-validation | 内部数据重复校验 / Internal data re-validated | `model_construct()` 跳过校验 |
| alias 与字段名混淆 / alias confusion | 序列化时用 alias 还是字段名 / Serialize with alias or name | `populate_by_name=True` |
| Optional 误解 / Optional misuse | `Optional[str]` ≠ 可有可无 / ≠ omittable | 明确 `= None` 默认值 / Explicit default |

### 5.3 model_construct 跳过校验 / Skip Validation

```python
# 当数据已经可信时，跳过校验提升性能 / Skip validation for trusted data
# 性能提升约 5-10x / ~5-10x performance improvement
trusted_data = {"status": 200, "duration_ms": 42.5, "data": {"ok": True}}

# 方式 1：正常校验（安全但较慢）/ Normal validation (safe but slower)
resp = ProxyResponse.model_validate(trusted_data)

# 方式 2：跳过校验（快但不安全）/ Skip validation (fast but unsafe)
resp = ProxyResponse.model_construct(**trusted_data)
# 注意：不检查类型、不运行校验器、不填充默认值
# Note: no type check, no validators, no default filling
```

## 6. 泛型模型 / Generic Models

### 6.1 泛型模型基础 / Generic Model Basics

Pydantic v2 支持基于 `typing.Generic` 的泛型模型：

```python
from typing import Generic, TypeVar
from pydantic import BaseModel

T = TypeVar('T')

# 通用 API 响应包装 / Generic API response wrapper
class APIResponse(BaseModel, Generic[T]):
    """type-safe 的通用响应模型 / Type-safe generic response model"""
    status: int
    duration_ms: float
    data: T | None = None
    error: str | None = None

# 具体化泛型 / Concrete generic
class HealthData(BaseModel):
    backend: str
    agent: str

# 使用具体类型参数化 / Use with concrete type parameter
HealthResponse = APIResponse[HealthData]
resp = HealthResponse(status=200, duration_ms=5.2, data={"backend": "ok", "agent": "ok"})
resp.data.backend  # ✅ 类型安全 / Type-safe

# 列表泛型 / List generic
BatchResponse = APIResponse[list[dict]]
batch = BatchResponse(status=200, duration_ms=120.0, data=[{"id": 1}, {"id": 2}])
```

### 6.2 泛型模型与 FastAPI / Generic Models with FastAPI

```python
from fastapi import FastAPI

app = FastAPI()

# FastAPI 自动解析泛型参数，生成正确的 OpenAPI schema
# FastAPI auto-resolves generic params, generates correct OpenAPI schema
@app.get("/health", response_model=APIResponse[HealthData])
async def get_health():
    return APIResponse(status=200, duration_ms=3.1, data=HealthData(backend="ok", agent="ok"))

# 嵌套泛型 / Nested generics
@app.post("/batch", response_model=APIResponse[list[dict]])
async def run_batch():
    return APIResponse(status=200, duration_ms=150.0, data=[{"result": "ok"}])
```

## 7. 计算字段与序列化定制 / Computed Fields & Serialization

### 7.1 computed_field 计算属性 / computed_field Computed Properties

```python
from pydantic import BaseModel, computed_field

class FileUpload(BaseModel):
    filename: str
    size_bytes: int

    # 计算字段：序列化时自动包含，不可赋值
    # Computed field: auto-included in serialization, not assignable
    @computed_field
    @property
    def size_human(self) -> str:
        """Human-readable file size / 人类可读的文件大小"""
        for unit in ['B', 'KB', 'MB', 'GB']:
            if self.size_bytes < 1024:
                return f"{self.size_bytes:.1f} {unit}"
            self.size_bytes /= 1024
        return f"{self.size_bytes:.1f} TB"

    @computed_field
    @property
    def extension(self) -> str:
        """File extension / 文件扩展名"""
        return self.filename.rsplit('.', 1)[-1].lower() if '.' in self.filename else ''

file = FileUpload(filename="data.CSV", size_bytes=2048)
file.size_human   # "2.0 KB"
file.extension    # "csv"

# 序列化时自动包含计算字段 / Computed fields auto-included in serialization
file.model_dump()
# {"filename": "data.CSV", "size_bytes": 2048, "size_human": "2.0 KB", "extension": "csv"}
```

### 7.2 field_serializer 序列化定制 / field_serializer Customization

```python
from pydantic import BaseModel, field_serializer
from datetime import datetime

class AuditRecord(BaseModel):
    timestamp: datetime
    data: dict

    # 定制序列化逻辑 / Customize serialization logic
    @field_serializer('timestamp')
    def serialize_timestamp(self, dt: datetime) -> str:
        """ISO 8601 格式 + 时区 / ISO 8601 format + timezone"""
        return dt.isoformat() + 'Z'

    @field_serializer('data')
    def serialize_data(self, data: dict) -> dict:
        """过滤敏感字段 / Filter sensitive fields"""
        return {k: v for k, v in data.items() if not k.startswith('_')}
```

## 8. 错误处理定制 / Error Handling Customization

### 8.1 自定义错误消息 / Custom Error Messages

```python
from pydantic import BaseModel, Field

class LbTestRequest(BaseModel):
    # 通过 json_schema_extra 定制错误提示 / Customize error via json_schema_extra
    num_requests: int = Field(
        default=10,
        ge=1,
        le=1000,
        json_schema_extra={
            "description": "请求次数 (1-1000)",
            "errorMessage": "请求次数必须在 1-1000 之间",
        }
    )

# 通过 field_validator 定制错误 / Customize errors via field_validator
from pydantic import field_validator

class MaskRequest(BaseModel):
    strategy: str

    @field_validator('strategy')
    @classmethod
    def validate_strategy(cls, v: str) -> str:
        allowed = {"partial", "full", "hash", "generalize"}
        if v not in allowed:
            raise ValueError(
                f"strategy 必须是 {allowed} 之一，收到: '{v}'"
            )
        return v
```

### 8.2 FastAPI 错误响应格式 / FastAPI Error Response Format

```python
# Pydantic 校验失败时 FastAPI 自动返回 422:
# FastAPI auto-returns 422 on Pydantic validation failure:
{
    "detail": [
        {
            "type": "greater_than_equal",      # 错误类型 / Error type
            "loc": ["body", "num_requests"],   # 字段位置 / Field location
            "msg": "Input should be >= 1",     # 错误消息 / Error message
            "input": 0,                        # 原始输入 / Original input
            "ctx": {"ge": 1}                   # 约束上下文 / Constraint context
        }
    ]
}

# 自定义异常处理器 / Custom exception handler
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

@app.exception_handler(RequestValidationError)
async def validation_handler(request, exc: RequestValidationError):
    """simplified 错误格式 / Simplified error format"""
    errors = [{"field": ".".join(str(l) for l in e["loc"]), "message": e["msg"]} for e in exc.errors()]
    return JSONResponse(status_code=422, content={"errors": errors})
```

## 9. Pydantic vs dataclass 对比 / Pydantic vs dataclass Comparison

### 9.1 功能对比表 / Feature Comparison Table

| 特性 / Feature | Pydantic BaseModel | dataclasses.dataclass | TypedDict |
|---|---|---|---|
| 运行时校验 / Runtime validation | ✅ 自动 / Auto | ❌ 无 / None | ❌ 无 / None |
| 类型强制转换 / Type coercion | ✅ "1" → 1 | ❌ 无 / None | ❌ 无 / None |
| JSON Schema | ✅ 自动生成 / Auto | ❌ 需第三方 / Needs 3rd party | ❌ 无 / None |
| 序列化 / Serialization | ✅ model_dump() | 手动 / Manual | 无 / None |
| 性能 / Performance | 高（Rust）/ High | 高（纯 Python）/ High | N/A |
| FastAPI 集成 / FastAPI integration | ✅ 原生 / Native | ✅ 支持 / Supported | ✅ 支持 |
| 嵌套校验 / Nested validation | ✅ 自动递归 / Auto recursive | ❌ 不校验 / No validation | ❌ 无 |
| 验证器 / Validators | ✅ field_validator | ❌ 手写 __post_init__ | ❌ 无 |
| 不可变 / Immutable | model_config frozen | frozen=True | N/A |

### 9.2 选择指南 / Selection Guide

```text
┌─────────────────────────────────────────────────────────────┐
│  何时用 Pydantic / When to use Pydantic:                      │
│    - API 请求/响应模型 / API request/response models         │
│    - 配置管理 / Configuration management                    │
│    - 外部数据校验 / External data validation                 │
│    - 需要 JSON Schema / Need JSON Schema                    │
├─────────────────────────────────────────────────────────────┤
│  何时用 dataclass / When to use dataclass:                    │
│    - 内部数据结构（已可信）/ Internal data (trusted)          │
│    - 纯计算结果传递 / Pure computation results               │
│    - 性能敏感场景（避免校验开销）/ Performance critical       │
├─────────────────────────────────────────────────────────────┤
│  本项目选择 / This project's choice:                          │
│    Pydantic —— 因为所有数据来自外部 HTTP 请求，必须校验    │
│    Pydantic -- because all data comes from external HTTP     │
└─────────────────────────────────────────────────────────────┘
```

## 10. 嵌套模型与深层校验 / Nested Models & Deep Validation

### 10.1 嵌套模型自动校验 / Nested Model Auto-validation

```python
from pydantic import BaseModel

class LbBackend(BaseModel):
    """负载均衡后端节点 / Load balancer backend node"""
    name: str
    url: str
    weight: int = 1

class LbTestRequest(BaseModel):
    """嵌套模型：Pydantic 自动递归校验每个 LbBackend"""
    """Nested model: Pydantic auto-recursively validates each LbBackend"""
    backends: list[LbBackend]           # 列表中每个元素都被校验 / Each list item validated
    num_requests: int = 10
    strategy: str = "round_robin"

# 深层嵌套也自动校验 / Deep nesting also auto-validated
raw = {
    "backends": [
        {"name": "agent-1", "url": "http://localhost:8079", "weight": 2},
        {"name": "agent-2", "url": "http://localhost:8080"},  # weight 用默认值 1
    ],
    "num_requests": 50,
}
req = LbTestRequest.model_validate(raw)  # 递归校验所有嵌套模型 / Recursively validate all
req.backends[0].weight  # 2
req.backends[1].weight  # 1 (默认值 / default)
```

### 10.2 嵌套校验错误定位 / Nested Validation Error Location

```python
# 嵌套模型的错误会包含完整路径 / Nested model errors include full path
try:
    LbTestRequest.model_validate({
        "backends": [{"name": "ok", "url": "http://x", "weight": -1}],
        "num_requests": 10,
    })
except ValidationError as e:
    print(e.errors())
    # [{
    #   "type": "greater_than_equal",
    #   "loc": ["backends", 0, "weight"],  ← 完整路径：第 0 个 backend 的 weight
    #   "msg": "Input should be >= 1",
    #   "input": -1
    # }]
```

### 10.3 递归模型 / Recursive Models

```python
from __future__ import annotations
from pydantic import BaseModel

# 自引用模型（树形结构）/ Self-referencing model (tree structure)
class TreeNode(BaseModel):
    value: str
    children: list[TreeNode] = []  # 递归引用自身 / Recursive self-reference

# Pydantic v2 自动处理递归模型，无需 model_rebuild()
# Pydantic v2 handles recursive models automatically
tree = TreeNode.model_validate({
    "value": "root",
    "children": [
        {"value": "child-1", "children": []},
        {"value": "child-2", "children": [{"value": "grandchild"}]},
    ]
})
tree.children[1].children[0].value  # "grandchild"
```

## 11. 模型继承与 Mixin / Model Inheritance & Mixin

### 11.1 继承层次设计 / Inheritance Hierarchy Design

```python
from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional

# 基础抽象模型 / Base abstract model
class PrivacyBaseModel(BaseModel):
    """所有隐私服务模型的基类 / Base class for all privacy service models.

    提供通用配置和序列化行为 /
    Provides common config and serialization behavior.
    """
    model_config = {
        "populate_by_name": True,     # 支持别名填充 / Support alias population
        "str_strip_whitespace": True,  # 自动去除空白 / Auto-strip whitespace
        "validate_default": True,      # 校验默认值 / Validate defaults
    }


# 时间戳 Mixin / Timestamp Mixin
class TimestampMixin(BaseModel):
    """为模型添加创建/更新时间 / Adds created/updated timestamps."""
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: Optional[datetime] = None


# 分页 Mixin / Pagination Mixin
class PaginationMixin(BaseModel):
    """分页参数 / Pagination parameters."""
    page: int = Field(default=1, ge=1)
    page_size: int = Field(default=20, ge=1, le=100)

    @property
    def offset(self) -> int:
        return (self.page - 1) * self.page_size


# 具体业务模型 / Concrete business model
class MaskingRequest(PrivacyBaseModel, TimestampMixin):
    """脱敏请求 / Masking request."""
    data: dict
    fields: list[str] = []
    strategy: str = "default"
```

### 11.2 多继承与 MRO / Multiple Inheritance & MRO

```python
# Python MRO（方法解析顺序）在 Pydantic 中的影响
# Python MRO (Method Resolution Order) impact in Pydantic

class AuditMixin(BaseModel):
    """审计字段 / Audit fields."""
    audit_user: str = "system"
    audit_action: str = "create"

class SoftDeleteMixin(BaseModel):
    """软删除 / Soft delete."""
    is_deleted: bool = False
    deleted_at: Optional[datetime] = None

# 组合多个 Mixin / Combine multiple Mixins
class ClassificationRecord(
    PrivacyBaseModel,
    TimestampMixin,
    AuditMixin,
    SoftDeleteMixin,
):
    """分类记录：组合所有 Mixin / Classification record: combines all Mixins."""
    field_name: str
    classification: str
    confidence: float = Field(ge=0.0, le=1.0)

# MRO: ClassificationRecord → PrivacyBase → Timestamp → Audit → SoftDelete → BaseModel
# 字段合并规则：所有父类字段均被继承 / Field merge: all parent fields inherited
record = ClassificationRecord(field_name="email", classification="PII", confidence=0.95)
print(record.created_at)   # 来自 TimestampMixin / From TimestampMixin
print(record.audit_user)   # 来自 AuditMixin / From AuditMixin
print(record.is_deleted)   # 来自 SoftDeleteMixin / From SoftDeleteMixin
```

### 11.3 抽象基类模式 / Abstract Base Class Pattern

```python
from abc import abstractmethod
from pydantic import BaseModel

class PrivacyPrimitiveRequest(BaseModel):
    """隐私原语请求抽象基类 / Abstract base for privacy primitive requests."""
    model_config = {"populate_by_name": True}

    @abstractmethod
    def get_primitive_name(self) -> str:
        """返回原语名称 / Return primitive name."""
        ...

class DPQueryRequest(PrivacyPrimitiveRequest):
    """差分隐私查询 / Differential privacy query."""
    query_type: str  # count | sum | mean
    epsilon: float = Field(gt=0, le=10)
    sensitivity: float = Field(default=1.0, gt=0)

    def get_primitive_name(self) -> str:
        return "differential_privacy"

class KAnonymityRequest(PrivacyPrimitiveRequest):
    """K-匿名请求 / K-anonymity request."""
    k: int = Field(ge=2)
    quasi_identifiers: list[str]

    def get_primitive_name(self) -> str:
        return "k_anonymity"
```

### 11.4 本项目继承实践 / This Project's Inheritance Practice

| 模式 / Pattern | 应用场景 / Use Case | 优势 / Advantage |
|---|---|---|
| 基类配置 / Base config | 所有请求/响应模型 / All req/resp models | 统一序列化行为 / Unified serialization |
| Mixin 组合 / Mixin composition | 分页、时间戳 / Pagination, timestamps | 可复用、无耦合 / Reusable, decoupled |
| 抽象基类 / ABC | 隐私原语接口 / Privacy primitive interface | 强制实现约束 / Enforced implementation |
| 字段覆盖 / Field override | 子类收紧约束 / Subclass tightens constraints | 渐进式约束 / Progressive constraints |

## 12. 配置管理与 Settings / Configuration Management & Settings

### 12.1 pydantic-settings 架构 / pydantic-settings Architecture

```python
from pydantic_settings import BaseSettings, SettingsConfigDict
from pathlib import Path
from typing import Optional

class PrivacyAgentSettings(BaseSettings):
    """隐私代理服务配置 / PrivShield Agent service settings.

    配置优先级（从高到低）/ Config priority (high to low):
    1. 环境变量 / Environment variables
    2. .env 文件 / .env file
    3. 默认值 / Default values
    """
    model_config = SettingsConfigDict(
        env_prefix="PRIVACY_",          # 环境变量前缀 / Env var prefix
        env_file=".env",                # .env 文件路径 / .env file path
        env_file_encoding="utf-8",
        case_sensitive=False,           # 大小写不敏感 / Case insensitive
        extra="ignore",                 # 忽略额外字段 / Ignore extra fields
    )

    # 服务配置 / Service config
    rest_host: str = "127.0.0.1"        # PRIVACY_REST_HOST
    rest_port: int = 8079               # PRIVACY_REST_PORT
    grpc_host: str = "127.0.0.1"        # PRIVACY_GRPC_HOST
    grpc_port: int = 50051              # PRIVACY_GRPC_PORT

    # 安全配置 / Security config
    tls_enabled: bool = False           # PRIVACY_TLS_ENABLED
    auth_enabled: bool = False          # PRIVACY_AUTH_ENABLED
    rate_limit_enabled: bool = False    # PRIVACY_RATE_LIMIT_ENABLED

    # 可观测性 / Observability
    log_level: str = "INFO"             # PRIVACY_LOG_LEVEL
    log_format: str = "text"            # PRIVACY_LOG_FORMAT
    service_name: str = "PrivShield"  # PRIVACY_SERVICE_NAME

    # 可选路径 / Optional paths
    profile_path: Optional[Path] = None  # PRIVACY_PROFILE
    budget_db: Optional[Path] = None     # PRIVACY_BUDGET_DB


# 单例模式 / Singleton pattern
_settings: Optional[PrivacyAgentSettings] = None

def get_settings() -> PrivacyAgentSettings:
    global _settings
    if _settings is None:
        _settings = PrivacyAgentSettings()
    return _settings
```

### 12.2 嵌套配置 / Nested Configuration

```python
class TLSConfig(BaseSettings):
    """TLS 子配置 / TLS sub-configuration."""
    enabled: bool = False
    cert_file: Optional[Path] = None
    key_file: Optional[Path] = None
    ca_file: Optional[Path] = None
    min_version: str = "TLSv1.2"

class RateLimitConfig(BaseSettings):
    """限流子配置 / Rate limit sub-configuration."""
    enabled: bool = False
    requests_per_minute: int = 60
    burst_size: int = 10

class AppSettings(BaseSettings):
    """应用总配置 / App root settings."""
    model_config = SettingsConfigDict(env_prefix="PRIVACY_")

    tls: TLSConfig = TLSConfig()
    rate_limit: RateLimitConfig = RateLimitConfig()
    debug: bool = False

# 环境变量映射 / Env var mapping:
# PRIVACY_TLS_ENABLED=true
# PRIVACY_TLS_CERT_FILE=/etc/certs/server.crt
# PRIVACY_RATE_LIMIT_REQUESTS_PER_MINUTE=120
```

### 12.3 配置校验与启动检查 / Config Validation & Startup Checks

```python
from pydantic import model_validator

class ValidatedSettings(BaseSettings):
    """带启动校验的配置 / Settings with startup validation."""
    tls_enabled: bool = False
    tls_cert_file: Optional[Path] = None
    tls_key_file: Optional[Path] = None
    auth_enabled: bool = False
    api_keys: list[str] = []

    @model_validator(mode="after")
    def validate_security_config(self) -> "ValidatedSettings":
        """启动时校验安全配置一致性 / Validate security config consistency at startup."""
        if self.tls_enabled:
            if not self.tls_cert_file or not self.tls_key_file:
                raise ValueError(
                    "TLS enabled but cert/key not provided / "
                    "TLS 已启用但未提供证书/密钥"
                )
            if not self.tls_cert_file.exists():
                raise ValueError(f"Cert file not found: {self.tls_cert_file}")
        if self.auth_enabled and not self.api_keys:
            raise ValueError("Auth enabled but no API keys configured")
        return self
```

### 12.4 本项目配置实践 / This Project's Config Practice

| 配置项 / Config Item | 环境变量 / Env Var | 默认值 / Default | 说明 / Notes |
|---|---|---|---|
| REST 端口 / REST port | `PRIVACY_REST_PORT` | 8079 | 代理后端转发目标 / Proxy backend target |
| gRPC 端口 / gRPC port | `PRIVACY_GRPC_PORT` | 50051 | Go 后端连接目标 / Go backend target |
| 日志级别 / Log level | `PRIVACY_LOG_LEVEL` | INFO | 支持 DEBUG/WARNING/ERROR |
| 配置文件 / Profile | `PRIVACY_PROFILE` | 无 / None | YAML 隐私参数 / YAML privacy params |
| 预算 DB / Budget DB | `PRIVACY_BUDGET_DB` | 内存 / In-memory | SQLite 持久化 / SQLite persist |

## 13. 数据迁移与版本化 / Data Migration & Versioning

### 13.1 模型版本化策略 / Model Versioning Strategy

```python
from pydantic import BaseModel, Field, model_validator
from typing import Optional, Literal

class PrivacyProfileV1(BaseModel):
    """版本 1 配置文件格式 / Version 1 profile format."""
    version: Literal[1] = 1
    epsilon: float = 1.0
    k_value: int = 5
    masking_strategy: str = "partial"

class PrivacyProfileV2(BaseModel):
    """版本 2 配置文件格式 / Version 2 profile format.

    变更 / Changes:
    - epsilon 拆分为 per-query 和 global / Split into per-query and global
    - 新增 noise_mechanism 字段 / Added noise_mechanism field
    - masking_strategy 重命名为 masking.mode / Renamed to masking.mode
    """
    version: Literal[2] = 2
    epsilon_per_query: float = 0.5
    epsilon_global: float = 5.0
    k_value: int = 5
    noise_mechanism: str = "laplace"  # laplace | gaussian
    masking: "MaskingConfig" = Field(default_factory=lambda: MaskingConfig())

class MaskingConfig(BaseModel):
    mode: str = "partial"
    preserve_length: bool = True

# 迁移函数 / Migration function
def migrate_v1_to_v2(v1: PrivacyProfileV1) -> PrivacyProfileV2:
    """将 V1 配置升级为 V2 / Upgrade V1 config to V2."""
    return PrivacyProfileV2(
        epsilon_per_query=v1.epsilon,
        epsilon_global=v1.epsilon * 10,
        k_value=v1.k_value,
        noise_mechanism="laplace",
        masking=MaskingConfig(mode=v1.masking_strategy),
    )
```

### 13.2 自动版本检测与迁移 / Auto Version Detection & Migration

```python
import yaml
from pathlib import Path

def load_profile_with_migration(path: Path) -> PrivacyProfileV2:
    """加载配置文件并自动迁移 / Load profile with auto-migration.

    支持多版本向后兼容 / Supports multi-version backward compatibility.
    """
    raw = yaml.safe_load(path.read_text())
    version = raw.get("version", 1)  # 无版本字段视为 V1 / No version = V1

    if version == 1:
        v1 = PrivacyProfileV1.model_validate(raw)
        return migrate_v1_to_v2(v1)
    elif version == 2:
        return PrivacyProfileV2.model_validate(raw)
    else:
        raise ValueError(f"Unsupported profile version: {version}")

# 使用示例 / Usage example
profile = load_profile_with_migration(Path("privacy-profile.yaml"))
```

### 13.3 字段别名与废弃 / Field Aliases & Deprecation

```python
from pydantic import BaseModel, Field, field_validator
import warnings

class APIResponse(BaseModel):
    """支持向后兼容的响应模型 / Backward-compatible response model."""
    # 新字段名 / New field name
    request_duration_ms: float = Field(alias="duration_ms")
    # 废弃字段（保留兼容）/ Deprecated field (kept for compat)
    elapsed: Optional[float] = Field(default=None, deprecated=True)

    @field_validator("elapsed", mode="before")
    @classmethod
    def warn_deprecated_elapsed(cls, v):
        if v is not None:
            warnings.warn(
                "'elapsed' is deprecated, use 'request_duration_ms'",
                DeprecationWarning,
                stacklevel=2,
            )
        return v

    model_config = {"populate_by_name": True}
```

### 13.4 本项目版本化实践 / This Project's Versioning Practice

| 场景 / Scenario | 策略 / Strategy | 说明 / Notes |
|---|---|---|
| gRPC Proto 演进 / Proto evolution | 字段编号不复用 / Never reuse field numbers | 保证向后兼容 / Ensure backward compat |
| REST API 响应 / REST API response | 添加新字段而非修改 / Add fields, don't modify | 客户端不受影响 / Clients unaffected |
| YAML 配置文件 / YAML config | version 字段 + 迁移函数 / version field + migrator | 启动时自动升级 / Auto-upgrade at startup |
| 内部模型 / Internal models | 直接修改（无外部契约）/ Direct modify (no contract) | 测试覆盖即可 / Test coverage suffices |

---

## 14. Validator 深入与自定义类型 / Deep Dive into Validators & Custom Types

### 14.1 field_validator 与 model_validator 详解 / field_validator & model_validator Details

Pydantic v2 的验证器系统提供了精细的控制能力：

```python
# ===== 验证器执行顺序与模式 / Validator Execution Order & Modes =====
from pydantic import BaseModel, field_validator, model_validator, Field
from typing import Self
import re

class PrivacyQuery(BaseModel):
    """隐私查询请求模型 / Privacy query request model"""
    mechanism: str = Field(default="laplace")
    epsilon: float = Field(gt=0, le=10)
    data: dict
    fields: list[str] = Field(default_factory=list)
    output_format: str = Field(default="json")

    # === 字段级验证器 / Field-level validators ===

    @field_validator("mechanism")
    @classmethod
    def validate_mechanism(cls, v: str) -> str:
        """验证机制名称 / Validate mechanism name"""
        allowed = {"laplace", "gaussian", "exponential"}
        v_lower = v.lower().strip()
        if v_lower not in allowed:
            raise ValueError(f"mechanism must be one of {allowed}, got '{v}'")
        return v_lower  # 标准化输出 / Normalize output

    @field_validator("fields")
    @classmethod
    def validate_fields(cls, v: list[str]) -> list[str]:
        """验证字段名格式 / Validate field name format"""
        pattern = re.compile(r'^[a-zA-Z_][a-zA-Z0-9_.]*$')
        for field in v:
            if not pattern.match(field):
                raise ValueError(f"Invalid field name: '{field}'")
        return list(set(v))  # 去重 / Deduplicate

    # === 模型级验证器 / Model-level validators ===

    @model_validator(mode="before")
    @classmethod
    def pre_process(cls, data: dict) -> dict:
        """前置处理: 在字段验证之前执行 / Pre-process: runs before field validation"""
        # 兼容旧版 API 字段名 / Compat with old API field names
        if "eps" in data and "epsilon" not in data:
            data["epsilon"] = data.pop("eps")
        if "method" in data and "mechanism" not in data:
            data["mechanism"] = data.pop("method")
        return data

    @model_validator(mode="after")
    def post_validate(self) -> Self:
        """后置验证: 跨字段约束 / Post-validate: cross-field constraints"""
        # gaussian 机制必须指定 fields / Gaussian requires fields
        if self.mechanism == "gaussian" and not self.fields:
            raise ValueError("gaussian mechanism requires 'fields' to be specified")

        # epsilon 与数据量约束 / Epsilon vs data size constraint
        if len(self.data) > 10000 and self.epsilon > 1.0:
            raise ValueError("epsilon > 1.0 not allowed for datasets > 10k rows")

        return self
```

### 14.2 自定义类型与 Annotated / Custom Types & Annotated

```python
# ===== 使用 Annotated 创建可复用验证类型 / Reusable Validated Types with Annotated =====
from typing import Annotated
from pydantic import AfterValidator, BeforeValidator, Field

# 正数验证 / Positive number validation
def must_be_positive(v: float) -> float:
    if v <= 0:
        raise ValueError("must be positive")
    return v

# 范围限制 / Range constraint
def epsilon_range(v: float) -> float:
    if not (0 < v <= 10):
        raise ValueError("epsilon must be in (0, 10]")
    return round(v, 6)  # 精度截断 / Precision truncation

# 定义可复用类型 / Define reusable types
PositiveFloat = Annotated[float, AfterValidator(must_be_positive)]
Epsilon = Annotated[float, AfterValidator(epsilon_range)]
FieldName = Annotated[str, Field(pattern=r'^[a-zA-Z_][a-zA-Z0-9_.]*$', max_length=128)]

# 在多个模型中复用 / Reuse across multiple models
class DPQuery(BaseModel):
    epsilon: Epsilon          # 自动应用验证 / Auto-applied validation
    sensitivity: PositiveFloat = 1.0

class MaskRequest(BaseModel):
    target_field: FieldName   # 字段名格式验证 / Field name format validation
    epsilon: Epsilon = 1.0    # 复用同一类型 / Reuse same type

# 自定义 Pydantic 类型 / Custom Pydantic type
class SensitivityLevel:
    """敏感度等级 / Sensitivity level"""
    LEVELS = ("public", "internal", "confidential", "restricted")

    def __init__(self, level: str):
        if level not in self.LEVELS:
            raise ValueError(f"Invalid level: {level}")
        self.level = level

    @classmethod
    def __get_pydantic_core_schema__(cls, source_type, handler):
        from pydantic_core import core_schema
        return core_schema.no_info_plain_validator_function(
            cls._validate,
            serialization=core_schema.to_string_ser_schema(),
        )

    @classmethod
    def _validate(cls, v):
        if isinstance(v, cls):
            return v
        return cls(str(v))

    def __str__(self):
        return self.level
```

### 14.3 验证器执行顺序 / Validator Execution Order

```
┌─────────────────────────────────────────────────────────────────┐
│        Pydantic v2 验证执行顺序 / Validation Execution Order     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. model_validator(mode="before")                              │
│     └─ 原始输入 dict，可修改/替换 / Raw input dict, can modify  │
│                                                                 │
│  2. 字段类型强制转换 / Field type coercion                     │
│     └─ str → int, str → float 等 / str → int, str → float     │
│                                                                 │
│  3. field_validator(mode="before")                              │
│     └─ 类型转换前处理 / Pre-coercion processing                │
│                                                                 │
│  4. 字段约束检查 / Field constraint check                       │
│     └─ gt, lt, min_length, pattern 等 / gt, lt, min_length    │
│                                                                 │
│  5. field_validator(mode="after")  ← 默认模式 / Default mode   │
│     └─ 类型已确认，安全处理 / Type confirmed, safe to process  │
│                                                                 │
│  6. model_validator(mode="after")                               │
│     └─ 所有字段已验证，跨字段检查 / All validated, cross-field │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 15. 与 Python 类型系统深度集成 / Deep Integration with Python Type System

### 15.1 泛型模型与 TypeVar / Generic Models & TypeVar

```python
# ===== 泛型响应包装器 / Generic Response Wrapper =====
from typing import TypeVar, Generic, Optional
from pydantic import BaseModel
from datetime import datetime

T = TypeVar("T")

class APIResponse(BaseModel, Generic[T]):
    """统一 API 响应包装 / Unified API response wrapper"""
    success: bool
    data: Optional[T] = None
    error: Optional[str] = None
    timestamp: datetime = Field(default_factory=datetime.utcnow)
    request_id: str

class MaskResult(BaseModel):
    masked_data: dict
    applied_rules: list[str]
    processing_time_ms: float

# 具体化泛型 / Concrete generic
class MaskResponse(APIResponse[MaskResult]):
    pass

# 使用 / Usage
resp = MaskResponse(
    success=True,
    data=MaskResult(
        masked_data={"email": "t***@example.com"},
        applied_rules=["email_mask"],
        processing_time_ms=2.5
    ),
    request_id="req_abc123"
)

# 列表泛型 / List generic
from typing import List

class PaginatedResponse(BaseModel, Generic[T]):
    items: List[T]
    total: int
    page: int
    page_size: int

    @property
    def has_next(self) -> bool:
        return self.page * self.page_size < self.total

# 具体化 / Concrete
EndpointList = PaginatedResponse[MaskResult]
```

### 15.2 判别联合与多态模型 / Discriminated Unions & Polymorphic Models

```python
# ===== 判别联合实现多态 / Discriminated Unions for Polymorphism =====
from typing import Literal, Union, Annotated
from pydantic import BaseModel, Field

# 基础机制定义 / Base mechanism definitions
class LaplaceMechanism(BaseModel):
    type: Literal["laplace"] = "laplace"
    epsilon: float = Field(gt=0)
    sensitivity: float = Field(default=1.0, gt=0)

class GaussianMechanism(BaseModel):
    type: Literal["gaussian"] = "gaussian"
    epsilon: float = Field(gt=0)
    delta: float = Field(gt=0, lt=1)  # 失败概率 / Failure probability
    sensitivity: float = Field(default=1.0, gt=0)

class ExponentialMechanism(BaseModel):
    type: Literal["exponential"] = "exponential"
    epsilon: float = Field(gt=0)
    utility_function: str  # 效用函数名 / Utility function name

# 判别联合类型 / Discriminated union type
DPMechanism = Annotated[
    Union[LaplaceMechanism, GaussianMechanism, ExponentialMechanism],
    Field(discriminator="type")  # 使用 'type' 字段区分 / Discriminate by 'type'
]

class DPQueryRequest(BaseModel):
    query: str
    mechanism: DPMechanism  # 自动根据 type 字段解析 / Auto-parse by type field

# 使用 - 自动解析为正确类型 / Usage - auto-parsed to correct type
req = DPQueryRequest.model_validate({
    "query": "SELECT COUNT(*) FROM users",
    "mechanism": {
        "type": "gaussian",
        "epsilon": 0.5,
        "delta": 1e-5,
        "sensitivity": 1.0
    }
})

# 类型缩窄 / Type narrowing
if isinstance(req.mechanism, GaussianMechanism):
    print(req.mechanism.delta)  # ✅ 类型安全 / Type-safe
```

### 15.3 Protocol 与鸭子类型 / Protocol & Duck Typing

```python
# ===== 使用 Protocol 定义接口契约 / Define Interface Contracts with Protocol =====
from typing import Protocol, runtime_checkable
from pydantic import BaseModel

@runtime_checkable
class PrivacyPrimitive(Protocol):
    """隐私原语接口 / Privacy primitive interface"""

    def apply(self, data: dict, config: dict) -> dict:
        """应用隐私保护 / Apply privacy protection"""
        ...

    def validate_config(self, config: dict) -> bool:
        """验证配置有效性 / Validate config validity"""
        ...

    @property
    def name(self) -> str:
        """原语名称 / Primitive name"""
        ...

# 实现类无需显式继承 / Implementations don't need explicit inheritance
class MaskingPrimitive:
    name = "masking"

    def apply(self, data: dict, config: dict) -> dict:
        # 实现掌码逻辑 / Implement masking logic
        return {k: "***" for k in data}

    def validate_config(self, config: dict) -> bool:
        return "strategy" in config

# 运行时检查 / Runtime check
assert isinstance(MaskingPrimitive(), PrivacyPrimitive)  # ✅ True

# 在 Pydantic 模型中使用 / Use in Pydantic models
class PipelineStep(BaseModel):
    primitive_name: str
    config: dict

    model_config = {"arbitrary_types_allowed": True}
```

---

## 16. 实战模式与 API 设计 / Practical Patterns & API Design

### 16.1 请求/响应模型分离 / Request/Response Model Separation

```python
# ===== API 模型分层设计 / API Model Layered Design =====
from pydantic import BaseModel, Field, ConfigDict
from typing import Optional
from datetime import datetime
from enum import Enum

# --- 枚举定义 / Enum definitions ---
class MaskStrategy(str, Enum):
    PARTIAL = "partial"      # 部分掌码 / Partial mask
    FULL = "full"            # 完全掌码 / Full mask
    HASH = "hash"            # 哈希替换 / Hash replacement
    GENERALIZE = "generalize"  # 泛化 / Generalization

# --- 请求模型 / Request models ---
class MaskFieldConfig(BaseModel):
    """单字段掌码配置 / Single field masking config"""
    field: str = Field(description="字段路径，支持点号嵌套 / Field path, dot notation")
    strategy: MaskStrategy = MaskStrategy.PARTIAL
    pattern: Optional[str] = Field(None, description="自定义掌码模式 / Custom mask pattern")

class MaskRequest(BaseModel):
    """掌码请求 / Masking request"""
    data: dict = Field(description="待掌码数据 / Data to mask")
    fields: list[MaskFieldConfig] = Field(default_factory=list)
    auto_detect: bool = Field(default=True, description="自动检测敏感字段 / Auto-detect PII")

# --- 响应模型 / Response models ---
class FieldMaskResult(BaseModel):
    field: str
    original_type: str
    strategy_applied: MaskStrategy
    was_modified: bool

class MaskResponse(BaseModel):
    """掌码响应 / Masking response"""
    masked_data: dict
    applied_rules: list[FieldMaskResult]
    processing_time_ms: float
    auto_detected_fields: list[str] = Field(default_factory=list)

    model_config = ConfigDict(
        json_schema_extra={
            "examples": [{
                "masked_data": {"email": "t***@example.com", "name": "张**"},
                "applied_rules": [
                    {"field": "email", "original_type": "string",
                     "strategy_applied": "partial", "was_modified": True}
                ],
                "processing_time_ms": 1.23,
                "auto_detected_fields": ["email"]
            }]
        }
    )
```

### 16.2 模型转换与 DTO 模式 / Model Transformation & DTO Pattern

```python
# ===== 分层模型转换 / Layered Model Transformation =====

# 内部领域模型 / Internal domain model
class ClassificationResult(BaseModel):
    field_name: str
    sensitivity_level: int  # 1-5
    category: str
    confidence: float
    matched_rules: list[str]
    layer: int  # 哪一层分类 / Which classification layer

# 外部 API DTO / External API DTO
class ClassificationDTO(BaseModel):
    """API 输出: 隐藏内部实现细节 / API output: hide internal details"""
    field: str
    level: str  # 将数字转为可读标签 / Convert number to readable label
    category: str
    confidence: float

    @classmethod
    def from_domain(cls, result: ClassificationResult) -> "ClassificationDTO":
        """领域模型 → DTO / Domain model → DTO"""
        level_map = {1: "public", 2: "internal", 3: "confidential",
                     4: "restricted", 5: "top_secret"}
        return cls(
            field=result.field_name,
            level=level_map.get(result.sensitivity_level, "unknown"),
            category=result.category,
            confidence=round(result.confidence, 4),
        )

# 批量转换 / Batch transformation
def to_dto_list(results: list[ClassificationResult]) -> list[ClassificationDTO]:
    return [ClassificationDTO.from_domain(r) for r in results]
```

### 16.3 Pydantic 设计模式总结 / Pydantic Design Patterns Summary

| 模式 / Pattern | 适用场景 / Use Case | 优势 / Advantage | 本项目应用 / Project Usage |
|---|---|---|---|
| 请求/响应分离 / Req/Resp split | API 边界 / API boundary | 独立演进 / Independent evolution | ✅ 全部 API |
| 判别联合 / Discriminated union | 多态输入 / Polymorphic input | 类型安全解析 / Type-safe parse | ✅ DP 机制 |
| 泛型包装 / Generic wrapper | 统一响应 / Unified response | 减少重复 / Reduce duplication | ✅ APIResponse |
| DTO 转换 / DTO transform | 内外分离 / Internal/external | 隐藏实现 / Hide implementation | ✅ 分类结果 |
| Annotated 类型 / Annotated types | 复用验证 / Reusable validation | DRY 原则 / DRY principle | ✅ 字段验证 |
| model_validator | 跨字段约束 / Cross-field | 业务规则 / Business rules | ✅ 配置校验 |

## 17. 序列化策略详解 / Serialization Strategy Details

### 17.1 自定义序列化器 / Custom Serializers

```python
# Pydantic v2 序列化器
# Pydantic v2 serializers
from pydantic import BaseModel, field_serializer, model_serializer
from datetime import datetime
from decimal import Decimal


class PrivacyResult(BaseModel):
    """ 隐私处理结果 """
    """ Privacy processing result """
    field_name: str
    original_value: str
    masked_value: str
    confidence: Decimal
    processed_at: datetime
    metadata: dict | None = None
    
    # 字段级序列化器
    # Field-level serializer
    @field_serializer('confidence')
    def serialize_confidence(self, v: Decimal) -> float:
        """ Decimal 转 float，保留 4 位小数 """
        """ Decimal to float, keep 4 decimal places """
        return round(float(v), 4)
    
    @field_serializer('processed_at')
    def serialize_datetime(self, v: datetime) -> str:
        """ ISO 8601 格式 """
        """ ISO 8601 format """
        return v.isoformat(timespec='milliseconds')
    
    # 条件序列化：排除 None 字段
    # Conditional serialization: exclude None fields
    @model_serializer(mode='wrap')
    def serialize_model(self, handler):
        result = handler(self)
        # 移除值为 None 的键
        # Remove keys with None values
        return {k: v for k, v in result.items() if v is not None}


# 使用示例
# Usage
result = PrivacyResult(
    field_name="phone",
    original_value="13812345678",
    masked_value="138****5678",
    confidence=Decimal("0.9876"),
    processed_at=datetime.now(),
)

# 不同序列化模式
# Different serialization modes
result.model_dump()                    # dict（Python 对象）
result.model_dump(mode='json')         # JSON 兼容 dict
result.model_dump_json()               # JSON 字符串
result.model_dump(exclude={'metadata'})  # 排除字段
result.model_dump(by_alias=True)       # 使用别名
```

### 17.2 别名与驼峰转换 / Alias & CamelCase Conversion

```python
# API 响应使用 camelCase，内部使用 snake_case
# API response uses camelCase, internal uses snake_case
from pydantic import BaseModel, ConfigDict
from pydantic.alias_generators import to_camel


class MaskDetail(BaseModel):
    """ 脱敏详情（自动 camelCase 输出） """
    """ Mask detail (auto camelCase output) """
    model_config = ConfigDict(
        alias_generator=to_camel,   # snake_case → camelCase
        populate_by_name=True,       # 允许两种名称访问
    )
    
    field_name: str          # → fieldName
    mask_strategy: str       # → maskStrategy
    original_length: int     # → originalLength
    is_reversible: bool      # → isReversible


detail = MaskDetail(
    field_name="email",
    mask_strategy="partial",
    original_length=20,
    is_reversible=False,
)

# 输出自动为 camelCase
# Output automatically camelCase
detail.model_dump(by_alias=True)
# {'fieldName': 'email', 'maskStrategy': 'partial', ...}

# 输入接受两种格式
# Input accepts both formats
MaskDetail(fieldName="phone", maskStrategy="hash", originalLength=11, isReversible=True)
MaskDetail(field_name="phone", mask_strategy="hash", original_length=11, is_reversible=True)
```

### 17.3 条件字段排除 / Conditional Field Exclusion

```python
# 根据角色排除敏感字段
# Exclude sensitive fields based on role
from pydantic import BaseModel, Field


class ClassificationRecord(BaseModel):
    """ 分类记录（含敏感信息） """
    """ Classification record (with sensitive info) """
    id: str
    field_name: str
    classification: str
    confidence: float
    
    # 敏感字段：仅管理员可见
    # Sensitive fields: admin only
    original_value: str = Field(exclude=True)  # 默认排除
    data_sample: str = Field(exclude=True)
    
    def to_admin_dict(self) -> dict:
        """ 管理员视图：包含所有字段 """
        """ Admin view: include all fields """
        return self.model_dump(force_populate=True)
    
    def to_user_dict(self) -> dict:
        """ 普通用户视图：排除敏感字段 """
        """ Regular user view: exclude sensitive fields """
        return self.model_dump(exclude={'original_value', 'data_sample'})
```

## 18. 性能剖析与优化 / Performance Profiling & Optimization

### 18.1 验证性能基准 / Validation Performance Benchmarks

```python
# Pydantic v2 性能测试
# Pydantic v2 performance testing
import timeit
from pydantic import BaseModel, validate_call


class SimpleModel(BaseModel):
    name: str
    age: int
    email: str


class ComplexModel(BaseModel):
    fields: list[dict[str, str]]
    config: dict[str, float]
    metadata: dict | None


# 基准测试
# Benchmark
simple_data = {"name": "test", "age": 25, "email": "a@b.com"}
complex_data = {
    "fields": [{"name": f"f{i}", "value": f"v{i}"} for i in range(100)],
    "config": {"epsilon": 1.0, "delta": 0.01},
    "metadata": None,
}

# 简单模型：~0.5μs/次
# Simple model: ~0.5μs/call
timeit.timeit(lambda: SimpleModel(**simple_data), number=100000)

# 复杂模型：~50μs/次
# Complex model: ~50μs/call
timeit.timeit(lambda: ComplexModel(**complex_data), number=10000)

# model_validate vs 构造函数
# model_validate vs constructor
timeit.timeit(lambda: SimpleModel.model_validate(simple_data), number=100000)
```

### 18.2 性能优化技巧 / Performance Optimization Tips

```python
# 1. 使用 model_construct 跳过验证（已确认数据有效）
# 1. Use model_construct to skip validation (data already validated)
trusted_data = {"name": "test", "age": 25, "email": "a@b.com"}
obj = SimpleModel.model_construct(**trusted_data)  # 10x 更快 / 10x faster

# 2. 避免动态模型创建
# 2. Avoid dynamic model creation
# ✘ 慢：每次创建新类
# ✘ Slow: creates new class each time
def bad_create(name):
    return create_model(name, field1=(str, ...), field2=(int, ...))

# ✔ 快：复用模型类
# ✔ Fast: reuse model class
class ReusableModel(BaseModel):
    field1: str
    field2: int

# 3. 使用 __slots__ 减少内存
# 3. Use __slots__ to reduce memory
class CompactModel(BaseModel):
    model_config = ConfigDict(frozen=True)  # 不可变 + 优化
    x: float
    y: float

# 4. 批量验证使用 TypeAdapter
# 4. Batch validation using TypeAdapter
from pydantic import TypeAdapter

adapter = TypeAdapter(list[SimpleModel])
results = adapter.validate_python([simple_data] * 1000)  # 比循环快 / Faster than loop
```

### 18.3 内存优化 / Memory Optimization

```python
# 大模型内存对比
# Large model memory comparison
import sys

# Pydantic 模型内存占用分析
# Pydantic model memory analysis
class LargeModel(BaseModel):
    data: list[dict[str, str]]  # 1000 条记录

obj = LargeModel(data=[{"k": "v"} for _ in range(1000)])

# 内存占用约 200KB（含元数据）
# Memory ~200KB (including metadata)
print(sys.getsizeof(obj))           # 对象本身 / Object itself
print(sys.getsizeof(obj.data))      # 列表 / List
print(obj.model_dump_json().__len__())  # JSON 大小 / JSON size

# 优化：使用流式解析大文件
# Optimization: streaming parse for large files
import ijson
from pydantic import TypeAdapter

adapter = TypeAdapter(SimpleModel)

def stream_parse_large_file(path: str):
    """ 流式解析大 JSON 文件，不一次性加载 """
    """ Stream parse large JSON file, not loading all at once """
    with open(path, 'rb') as f:
        for item in ijson.items(f, 'item'):
            yield adapter.validate_python(item)
```

## 19. 与 ORM 及数据库集成 / ORM & Database Integration

### 19.1 SQLAlchemy 集成 / SQLAlchemy Integration

```python
# Pydantic + SQLAlchemy 2.0 集成模式
# Pydantic + SQLAlchemy 2.0 integration pattern
from sqlalchemy import Column, Integer, String, Float
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from pydantic import BaseModel, ConfigDict


# SQLAlchemy 模型（数据库层）
# SQLAlchemy model (database layer)
class Base(DeclarativeBase):
    pass


class ClassificationDB(Base):
    __tablename__ = "classifications"
    
    id: Mapped[int] = mapped_column(primary_key=True)
    field_name: Mapped[str] = mapped_column(String(255))
    classification: Mapped[str] = mapped_column(String(50))
    confidence: Mapped[float] = mapped_column(Float)
    raw_value: Mapped[str] = mapped_column(String(1000))


# Pydantic 模型（API 层）
# Pydantic model (API layer)
class ClassificationCreate(BaseModel):
    """ 创建请求 """
    """ Create request """
    field_name: str
    classification: str
    confidence: float


class ClassificationResponse(BaseModel):
    """ 响应模型（从 ORM 转换） """
    """ Response model (from ORM) """
    model_config = ConfigDict(from_attributes=True)  # 支持 ORM 对象
    
    id: int
    field_name: str
    classification: str
    confidence: float
    # 注意：不暴露 raw_value
    # Note: don't expose raw_value


# 转换示例
# Conversion example
def create_classification(db: Session, data: ClassificationCreate):
    db_obj = ClassificationDB(**data.model_dump())
    db.add(db_obj)
    db.commit()
    db.refresh(db_obj)
    return ClassificationResponse.model_validate(db_obj)  # ORM → Pydantic
```

### 19.2 分层 DTO 模式 / Layered DTO Pattern

```python
# 严格的分层数据传输对象
# Strict layered Data Transfer Objects
from pydantic import BaseModel
from datetime import datetime


# 层 1：数据库实体（内部）
# Layer 1: Database entity (internal)
class UserEntity(BaseModel):
    id: int
    username: str
    password_hash: str      # 绝不暴露 / Never expose
    email: str
    created_at: datetime
    is_admin: bool


# 层 2：服务层 DTO（内部传递）
# Layer 2: Service layer DTO (internal transfer)
class UserServiceDTO(BaseModel):
    id: int
    username: str
    email: str
    is_admin: bool


# 层 3：API 响应（外部）
# Layer 3: API response (external)
class UserResponse(BaseModel):
    id: int
    username: str
    # 不暴露 email 和 is_admin
    # Don't expose email and is_admin


# 转换链
# Conversion chain
def get_user_response(entity: UserEntity) -> UserResponse:
    dto = UserServiceDTO(
        id=entity.id,
        username=entity.username,
        email=entity.email,
        is_admin=entity.is_admin,
    )
    return UserResponse(id=dto.id, username=dto.username)
```

### 19.3 Pydantic 与 ORM 对比 / Pydantic vs ORM Comparison

| 特性 / Feature | Pydantic | SQLAlchemy | 职责 / Responsibility |
|---|---|---|---|
| 数据验证 / Validation | ✅ 核心功能 | ✘ | API 边界 / API boundary |
| 序列化 / Serialization | ✅ JSON/dict | 部分 / Partial | 响应格式化 / Response format |
| 数据库映射 / DB mapping | ✘ | ✅ 核心功能 | 持久化 / Persistence |
| 查询构建 / Query building | ✘ | ✅ | 数据访问 / Data access |
| 关系管理 / Relations | ✘ | ✅ | 关联查询 / Joins |
| 迁移 / Migration | ✘ | ✅ (Alembic) | Schema 演进 / Schema evolution |

### 19.4 集成最佳实践 / Integration Best Practices

| 实践 / Practice | 说明 / Description | 原因 / Reason |
|---|---|---|
| 严格分层 / Strict layering | DB ≠ API 模型 | 安全 + 解耦 / Security + decoupling |
| from_attributes=True | ORM → Pydantic | 零拷贝转换 / Zero-copy |
| 不暴露内部字段 / No internal fields | password_hash 等 | 安全 / Security |
| 复用验证逻辑 / Reuse validation | Annotated 类型 | DRY |
| 批量转换 / Batch convert | TypeAdapter | 性能 / Performance |
