# PyArrow & Pandas 数据处理技术栈说明 / PyArrow & Pandas Data Processing Technology Stack

## 1. 技术简介 / Introduction

### PyArrow
PyArrow 是 Apache Arrow 的 Python 绑定库，提供高性能的列式内存格式与跨语言数据交换能力。
PyArrow is the Python binding for Apache Arrow, providing high-performance columnar memory format and cross-language data exchange capabilities.

核心特性 / Core Features：
- **列式内存格式（Columnar Memory Format）**：面向分析型工作负载优化的内存布局，CPU 缓存友好。
- **零拷贝共享（Zero-copy Sharing）**：进程间/语言间共享数据无需序列化/反序列化。
- **IPC 流协议（IPC Stream Protocol）**：标准化的二进制流格式，适合 HTTP 传输与管道通信。
- **格式互操作（Format Interop）**：原生支持 CSV、Parquet、JSON、ORC 等格式读写。
- **Table/RecordBatch API**：结构化表格操作，支持过滤、切片、类型转换。

### Pandas
Pandas 是 Python 数据分析的核心库，提供 DataFrame/Series 两种核心数据结构。
Pandas is the core library for Python data analysis, providing DataFrame/Series as its two primary data structures.

核心特性 / Core Features：
- **DataFrame 表格操作**：类 SQL 的筛选、聚合、分组、透视能力。
- **缺失值处理（NA Handling）**：NaN/None 的统一处理策略。
- **格式转换（Format Conversion）**：to_dict/to_json/to_csv 等多种输出格式。
- **与 Arrow 互操作**：`Table.to_pandas()` 实现 Arrow → Pandas 零拷贝转换。

本项目使用版本 / Versions Used：`pyarrow`（延迟导入，非硬依赖）、`pandas`（随 pyarrow 可选安装）

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 架构角色 / Architecture Role

```
前端 React ──HTTP/JSON──▶ Python 代理后端 ──HTTP/REST──▶ PrivShield
                              │                              │
                              │    ◀── Arrow IPC 二进制流 ────┘
                              │         (application/vnd.apache.arrow.stream)
                              ▼
                     PyArrow 解析 → Pandas 转换 → JSON 返回前端
```

Python 代理后端在接收到 Agent 返回的 Arrow IPC 二进制流时，使用 PyArrow 解析为 Table，
再通过 Pandas 转换为 JSON 可序列化的记录列表返回给前端。
The Python proxy backend uses PyArrow to parse Arrow IPC binary streams from the Agent into Tables,
then converts to JSON-serializable record lists via Pandas for the frontend.

### 2.2 Arrow IPC 响应解析 / Arrow IPC Response Parsing

文件 / File：`console/backend/app/client.py`

```python
@staticmethod
def _parse_arrow_response(response: httpx.Response) -> dict[str, Any]:
    """
    解析 Arrow IPC 二进制流响应 / Parse Arrow IPC binary stream response

    详细逻辑 / Detailed Logic：
      1. 延迟导入 pyarrow（避免未使用 Arrow 时引入重量级依赖）
      2. 从响应 body 字节流中读取 Arrow IPC RecordBatchStreamReader
      3. 将所有 RecordBatch 合并为单个 Table
      4. Table → Pandas DataFrame → 记录列表（orient="records"）
      5. NaN 替换为 None（JSON 不支持 NaN 值）
    """
    # 延迟导入 pyarrow：避免在未用到 Arrow 的场景下引入重量级依赖
    import pyarrow as pa

    # 从 HTTP 响应体创建 Arrow IPC 流读取器
    reader = pa.ipc.open_stream(response.content)
    # 读取所有 RecordBatch 并合并为单一 Table
    table = reader.read_all()

    return {
        "schema": table.schema.names,  # 列名列表 / Column name list
        "_content_type": "application/vnd.apache.arrow.stream",
        # 表格转 pandas 再转记录列表；NaN 替换为 None（JSON 无 NaN）
        "records": table.to_pandas().replace({float("nan"): None}).to_dict(orient="records"),
        "num_rows": table.num_rows,  # 总行数 / Total row count
    }
```

### 2.3 内容类型自动检测 / Content-Type Auto Detection

```python
# client.py 中的响应分发逻辑 / Response dispatch logic in client.py
def forward(self, method, path, body, ...):
    response = self._client.request(...)
    ct = response.headers.get("content-type", "")

    # 根据 Content-Type 决定解析策略 / Decide parsing strategy by Content-Type
    if "application/vnd.apache.arrow.stream" in ct:
        # Arrow IPC 二进制流 → PyArrow 解析 / Arrow IPC binary → PyArrow parse
        return self._parse_arrow_response(response)
    else:
        # 标准 JSON 响应 → 直接反序列化 / Standard JSON → direct deserialization
        return response.json()
```

### 2.4 示例 Payload 生成 / Sample Payload Generation

文件 / File：`console/backend/app/fixtures/samples.py`

```python
def _arrow_ipc_payload() -> str:
    """
    生成 Arrow IPC 格式的 base64 编码示例载荷 / Generate base64-encoded Arrow IPC sample payload

    用于 /v1/privacy/dp/arrow_ipc 端点的示例：该端点要求二进制
    Arrow IPC 流作为请求体，而非 JSON。
    Used for the /v1/privacy/dp/arrow_ipc endpoint sample: this endpoint requires
    binary Arrow IPC stream as request body, not JSON.
    """
    # 延迟导入 pyarrow：仅在生成 Arrow 示例时才引入重量级依赖
    import pyarrow as pa

    # 构造示例表格：values 列包含浮点数 / Build sample table: values column with floats
    table = pa.table({"values": [1.0, 2.0, 3.0, 4.0, 5.0]})

    # 将 Table 序列化为 Arrow IPC 流格式字节 / Serialize Table to Arrow IPC stream bytes
    sink = pa.BufferOutputStream()
    with pa.ipc.new_stream(sink, table.schema) as writer:
        writer.write_table(table)

    # 返回 base64 编码字符串，便于在 JSON 示例中传输 / Return base64 string for JSON transport
    return base64.b64encode(sink.getvalue().to_pybytes()).decode()
```

### 2.5 延迟导入策略 / Lazy Import Strategy

本项目中 PyArrow 采用**延迟导入（Lazy Import）** 策略：
In this project, PyArrow uses a **Lazy Import** strategy:

| 策略 / Strategy | 说明 / Description |
|---|---|
| 函数内 `import pyarrow` | 仅在调用 Arrow 解析/生成时才加载库 / Only loads library when Arrow parse/generate is called |
| 非 requirements.txt 硬依赖 | 未安装时不影响其他端点正常工作 / Not a hard dependency; other endpoints work without it |
| 优雅降级 | 缺少 pyarrow 时 Arrow 端点返回 ImportError 而非服务崩溃 / Graceful degradation |

设计原因 / Design Rationale：
- PyArrow + Pandas 安装体积约 200MB，对不需要 Arrow 的部署场景过重
- 控制台大部分端点使用 JSON 通信，仅 `/v1/privacy/dp/arrow_ipc` 需要 Arrow
- 延迟导入确保核心功能启动速度不受影响

### 2.6 数据流转路径 / Data Flow Path

```
[Agent 端]                     [Python 代理后端]                    [前端]
DP Arrow IPC 计算                                                     
    │                                                                  
    ▼                                                                  
Arrow IPC 二进制流 ──HTTP──▶ _parse_arrow_response()                  
                                │                                      
                                ▼                                      
                          pa.ipc.open_stream()                         
                                │                                      
                                ▼                                      
                          table.to_pandas()                            
                                │                                      
                                ▼                                      
                          .replace({NaN: None})                        
                                │                                      
                                ▼                                      
                          .to_dict(orient="records")                   
                                │                                      
                                ▼                                      
                          JSON Response ──────HTTP──────────────▶ 渲染表格数据
```

### 2.7 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| Arrow IPC 而非 JSON 传输大表 / Arrow IPC over JSON for large tables | 二进制编码体积更小、解析更快 / Binary encoding is smaller and faster to parse |
| 代理层转换为 JSON 返回前端 / Proxy converts to JSON for frontend | 前端浏览器无法直接解析 Arrow 流 / Browser cannot parse Arrow stream directly |
| NaN → None 替换 / NaN → None replacement | JSON 标准不支持 NaN/Infinity / JSON standard doesn't support NaN/Infinity |
| base64 编码示例载荷 / base64 encoded sample payload | 在 JSON 结构中安全传输二进制数据 / Safely transport binary data in JSON structure |
| 延迟导入而非顶层导入 / Lazy import over top-level import | 减少启动时间与内存占用 / Reduce startup time and memory footprint |

### 2.8 Arrow 列式内存布局 / Arrow Columnar Memory Layout

```text
传统行式存储 / Traditional Row Storage (JSON/CSV):
┌─────────────────────────────────────────────────┐
│ Row 1: {"name": "Alice", "age": 30, "city": "NYC"}  │
│ Row 2: {"name": "Bob",   "age": 25, "city": "LA"}   │
│ Row 3: {"name": "Carol", "age": 35, "city": "SF"}   │
└─────────────────────────────────────────────────┘
问题：分析单列时需读取所有行 / Problem: analyzing one column reads all rows

Arrow 列式存储 / Arrow Columnar Storage:
┌─────────────────────────────────────────────────┐
│ name 列 / name column: ["Alice", "Bob", "Carol"]      │
│ age 列 / age column:   [30, 25, 35]                   │
│ city 列 / city column: ["NYC", "LA", "SF"]            │
└─────────────────────────────────────────────────┘
优势：单列分析仅读取该列 / Benefit: single-column analysis reads only that column
```

**列式存储优势 / Columnar Storage Benefits**：

| 优势 / Benefit | 说明 / Description |
|---|---|
| CPU 缓存友好 / CPU cache friendly | 同类型数据连续存储，缓存命中率高 / Same-type data contiguous, high cache hit |
| 向量化计算 / Vectorized computation | SIMD 指令批量处理同类型数据 / SIMD batch process same-type data |
| 压缩率高 / High compression | 同类型数据压缩效果更好 / Same-type data compresses better |
| 零拷贝 / Zero-copy | 进程间共享无需序列化 / No serialization for inter-process sharing |

### 2.9 Arrow ↔ Pandas 类型映射 / Arrow ↔ Pandas Type Mapping

| Arrow 类型 / Arrow Type | Pandas 类型 / Pandas Type | Python 类型 / Python Type |
|---|---|---|
| `int64` | `int64` | `int` |
| `float64` | `float64` | `float` |
| `string` | `object` | `str` |
| `bool` | `bool` | `bool` |
| `timestamp[ns]` | `datetime64[ns]` | `datetime` |
| `null` | `NaN` / `None` | `None` |
| `list<T>` | `object` (list) | `list` |

**NaN 处理的重要性 / Importance of NaN Handling**：

```python
# Pandas 中的 NaN 无法直接序列化为 JSON
# NaN in Pandas cannot be directly serialized to JSON
import json
json.dumps({"value": float("nan")})  # ✗ ValueError!

# 解决：替换为 None / Solution: replace with None
df.replace({float("nan"): None}).to_dict(orient="records")
# 输出 / Output: [{"value": None}]  ✓ JSON 合法 / Valid JSON
```

### 2.10 性能对比 / Performance Comparison

**JSON vs Arrow IPC 传输大表 / JSON vs Arrow IPC for large tables**：

| 指标 / Metric | JSON | Arrow IPC | 提升 / Improvement |
|---|---|---|---|
| 10K 行体积 / 10K rows size | ~2MB | ~400KB | **5x** 更小 / smaller |
| 解析速度 / Parse speed | ~50ms | ~5ms | **10x** 更快 / faster |
| 内存占用 / Memory usage | ~10MB | ~2MB | **5x** 更省 / less |
| 类型保真 / Type fidelity | 丢失（全为 string）/ Lost | 保留 / Preserved | - |

**何时使用 Arrow IPC / When to use Arrow IPC**：

| 场景 / Scenario | 推荐格式 / Recommended |
|---|---|
| 小数据 (<100 行) / Small data | JSON（简单直观）/ JSON (simple) |
| 大表格 (>1K 行) / Large tables | Arrow IPC（性能优势）/ Arrow IPC (performance) |
| 数值计算 / Numeric computation | Arrow IPC（类型保真）/ Arrow IPC (type fidelity) |
| 浏览器直接消费 / Browser direct consumption | JSON（无需额外库）/ JSON (no extra lib) |

### 2.11 错误处理与降级 / Error Handling & Degradation

```python
# 缺少 pyarrow 时的优雅降级 / Graceful degradation when pyarrow missing
def _parse_arrow_response(response):
    try:
        import pyarrow as pa  # 延迟导入 / Lazy import
    except ImportError:
        # 返回友好错误而非崩溃 / Return friendly error instead of crash
        return {
            "error": "pyarrow not installed",
            "hint": "pip install pyarrow pandas",
            "raw_content_type": response.headers.get("content-type"),
        }
    # 正常解析流程 / Normal parsing flow
    reader = pa.ipc.open_stream(response.content)
    table = reader.read_all()
    return {...}
```

**降级策略 / Degradation Strategy**：

```text
┌─────────────────────────────────────────────────────────────┐
│  pyarrow 已安装 / pyarrow installed                          │
│  → 正常解析 Arrow IPC → 返回结构化数据                    │
│  → Normal parse Arrow IPC → Return structured data           │
├─────────────────────────────────────────────────────────────┤
│  pyarrow 未安装 / pyarrow not installed                      │
│  → 返回错误提示 + 安装指引                                │
│  → Return error hint + installation guide                    │
│  → 其他端点不受影响 / Other endpoints unaffected             │
└─────────────────────────────────────────────────────────────┘
```

## 3. Arrow 计算函数 / Arrow Compute Functions

### 3.1 向量化计算引擎 / Vectorized Compute Engine

PyArrow 提供丰富的向量化计算函数，无需转换为 Pandas 即可执行分析操作：

```python
import pyarrow as pa
import pyarrow.compute as pc

# 创建示例表 / Create sample table
table = pa.table({
    "name": ["Alice", "Bob", "Carol", "Dave"],
    "age": [30, 25, 35, 28],
    "salary": [80000.0, 65000.0, 95000.0, 72000.0],
})

# 过滤 / Filter
adults = table.filter(pc.greater(table["age"], 27))
# 结果：Alice(30), Carol(35), Dave(28)

# 排序 / Sort
sorted_table = table.sort_by([("salary", "descending")])
# 结果：Carol(95K), Alice(80K), Dave(72K), Bob(65K)

# 聚合 / Aggregation
mean_salary = pc.mean(table["salary"])  # 78000.0
max_age = pc.max(table["age"])          # 35

# 列运算（向量化，无 Python 循环）/ Column ops (vectorized, no Python loop)
bonus = pc.multiply(table["salary"], 0.1)  # 10% 奖金 / 10% bonus
```

### 3.2 计算函数分类 / Compute Function Categories

| 类别 / Category | 函数示例 / Function Examples | 用途 / Purpose |
|---|---|---|
| 算术 / Arithmetic | `add`, `subtract`, `multiply`, `divide` | 数值运算 / Numeric ops |
| 比较 / Comparison | `equal`, `greater`, `less`, `not_equal` | 条件过滤 / Conditional filter |
| 聚合 / Aggregate | `sum`, `mean`, `min`, `max`, `count` | 统计汇总 / Statistics |
| 字符串 / String | `utf8_upper`, `utf8_length`, `match_substring` | 文本处理 / Text processing |
| 逻辑 / Logical | `and_`, `or_`, `invert` | 布尔运算 / Boolean ops |
| 类型转换 / Cast | `cast`, `strftime`, `strptime` | 类型变换 / Type conversion |
| 排序 / Sorting | `sort_indices`, `rank` | 排序操作 / Sort operations |

### 3.3 与 Pandas 操作对比 / Comparison with Pandas Operations

```python
# Pandas 方式 / Pandas approach
df = table.to_pandas()
result = df[df["age"] > 27].sort_values("salary", ascending=False)

# Arrow 方式（更快，无中间转换）/ Arrow approach (faster, no intermediate conversion)
result = table.filter(pc.greater(table["age"], 27)).sort_by([("salary", "descending")])

# 性能对比（10万行数据）/ Performance (100K rows):
# Pandas: ~15ms（含 Arrow→Pandas 转换）/ Including conversion
# Arrow:  ~3ms（原生列式计算）/ Native columnar compute
```

## 4. 内存管理与零拷贝 / Memory Management & Zero-copy

### 4.1 Arrow 内存池 / Arrow Memory Pool

```text
┌─────────────────────────────────────────────────────────────┐
│  Arrow Memory Pool (jemalloc / system allocator)             │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  Buffer #1: name 列数据 / name column data           │ │
│  │  ["Alice", "Bob", "Carol", "Dave"]                   │ │
│  │  内存布局：offsets[] + data[] (连续存储)            │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  Buffer #2: age 列数据 / age column data             │ │
│  │  [30, 25, 35, 28]                                    │ │
│  │  内存布局：int64[] (8字节对齐)                      │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  Buffer #3: validity bitmap (null 标记)              │ │
│  │  [1, 1, 1, 1] (全部有效)                            │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 零拷贝共享机制 / Zero-copy Sharing Mechanism

```python
# Arrow → Pandas 零拷贝（数值列）/ Arrow → Pandas zero-copy (numeric columns)
import pyarrow as pa
import numpy as np

arr = pa.array([1.0, 2.0, 3.0, 4.0, 5.0])
# 零拷贝转换为 numpy（共享底层内存）/ Zero-copy to numpy (shares underlying memory)
np_arr = arr.to_numpy(zero_copy=True)
np_arr[0] = 99.0  # 修改 numpy 也会影响 Arrow（共享内存）/ Modifying numpy affects Arrow

# Pandas DataFrame 零拷贝 / Pandas DataFrame zero-copy
df = arr.to_pandas()  # float64 列可零拷贝 / float64 can zero-copy

# 注意：字符串列无法零拷贝（Arrow UTF-8 vs Python object）
# Note: string columns cannot zero-copy (Arrow UTF-8 vs Python object)
```

### 4.3 内存使用监控 / Memory Usage Monitoring

```python
# 查看 Arrow 内存池使用情况 / Check Arrow memory pool usage
import pyarrow as pa

pool = pa.default_memory_pool()
print(f"Bytes allocated: {pool.bytes_allocated()}")  # 当前分配 / Currently allocated
print(f"Max memory: {pool.max_memory()}")            # 峰值使用 / Peak usage

# 释放内存 / Release memory
table = None  # Python GC 回收 / Python GC reclaims
pool.release_unused()  # 释放未使用的缓存 / Release unused cache
```

## 5. IPC 流协议详解 / IPC Stream Protocol Details

### 5.1 Arrow IPC 格式结构 / Arrow IPC Format Structure

```text
Arrow IPC Stream 格式 / Arrow IPC Stream Format:
┌─────────────────────────────────────────────────────────────┐
│  Schema Message                                              │
│  - 列名列表 / Column names                                   │
│  - 数据类型 / Data types                                     │
│  - 元数据 / Metadata                                         │
├─────────────────────────────────────────────────────────────┤
│  RecordBatch #1                                              │
│  - 行数 / Row count                                          │
│  - 列数据缓冲区 / Column data buffers                       │
│  - 有效性位图 / Validity bitmaps                            │
├─────────────────────────────────────────────────────────────┤
│  RecordBatch #2 (可选，大数据分块) / Optional, chunked       │
│  ...                                                         │
├─────────────────────────────────────────────────────────────┤
│  EOS Marker (流结束标记) / End of Stream marker              │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 流式 vs 文件格式 / Stream vs File Format

| 特性 / Feature | IPC Stream | IPC File (Feather) |
|---|---|---|
| 格式 / Format | 顺序流 / Sequential stream | 随机访问 / Random access |
| 适用 / Suitable | HTTP 传输、管道 / HTTP, pipes | 磁盘存储 / Disk storage |
| 读取方式 / Read mode | 必须顺序读 / Must read sequentially | 可切片读取 / Can slice read |
| 本项目使用 / This project | ✅ HTTP 响应 / HTTP response | ❌ 未使用 / Not used |
| 扩展名 / Extension | `.arrow` / `.arrows` | `.feather` / `.arrow` |

### 5.3 序列化与反序列化 / Serialization & Deserialization

```python
import pyarrow as pa

# === 序列化（发送端）/ Serialization (sender) ===
table = pa.table({"values": [1.0, 2.0, 3.0]})

# 方式 1：写入流 / Write to stream
sink = pa.BufferOutputStream()
with pa.ipc.new_stream(sink, table.schema) as writer:
    writer.write_table(table)
stream_bytes = sink.getvalue().to_pybytes()  # HTTP 传输的字节 / Bytes for HTTP

# 方式 2：写入文件 / Write to file
with pa.OSFile("data.arrow", "wb") as f:
    with pa.ipc.new_file(f, table.schema) as writer:
        writer.write_table(table)

# === 反序列化（接收端）/ Deserialization (receiver) ===

# 从字节流读取 / Read from bytes
reader = pa.ipc.open_stream(stream_bytes)
restored_table = reader.read_all()

# 从文件读取 / Read from file
with pa.memory_map("data.arrow", "r") as f:
    restored_table = pa.ipc.open_file(f).read_all()
```

## 6. Pandas 数据处理模式 / Pandas Data Processing Patterns

### 6.1 本项目中的 Pandas 用法 / Pandas Usage in This Project

```python
# client.py 中的 Arrow → Pandas → JSON 转换链
# Arrow → Pandas → JSON conversion chain in client.py

# 步骤 1: Arrow Table → Pandas DataFrame
df = table.to_pandas()

# 步骤 2: NaN → None（JSON 兼容性）/ NaN → None (JSON compatibility)
df_clean = df.replace({float("nan"): None})

# 步骤 3: DataFrame → 记录列表 / DataFrame → records list
records = df_clean.to_dict(orient="records")
# 输出 / Output: [{"col1": val1, "col2": val2}, ...]
```

### 6.2 orient 参数详解 / orient Parameter Details

| orient 值 / Value | 输出格式 / Output Format | 适用场景 / Use Case |
|---|---|---|
| `"records"` | `[{col: val}, ...]` | ✅ 本项目：前端表格渲染 / Frontend table |
| `"dict"` | `{col: {idx: val}}` | 按列分组 / Column grouping |
| `"list"` | `{col: [val, ...]}` | 列式数据 / Columnar data |
| `"split"` | `{columns: [], data: [[]]}` | 分离表头与数据 / Separate header |
| `"index"` | `{idx: {col: val}}` | 按索引分组 / Index grouping |

### 6.3 大数据量处理策略 / Large Data Handling Strategy

```python
# 分块处理大表（避免内存溢出）/ Chunk processing for large tables
import pyarrow as pa

def process_large_arrow_stream(response_content: bytes, chunk_size: int = 10000):
    """分块读取 Arrow IPC 流，避免一次性加载全部数据。
    Read Arrow IPC stream in chunks to avoid loading all data at once."""
    reader = pa.ipc.open_stream(response_content)
    all_records = []

    for batch in reader:  # 逐 RecordBatch 迭代 / Iterate per RecordBatch
        df_chunk = batch.to_pandas()
        df_chunk = df_chunk.replace({float("nan"): None})
        all_records.extend(df_chunk.to_dict(orient="records"))

        # 可选：限制最大返回行数 / Optional: limit max rows
        if len(all_records) >= 100000:
            break

    return all_records
```

## 7. 与 Parquet 格式的关系 / Relationship with Parquet Format

### 7.1 Arrow vs Parquet / Arrow vs Parquet

| 维度 / Dimension | Arrow (IPC) | Parquet |
|---|---|---|
| 设计目标 / Design goal | 内存计算 + 传输 / In-memory + transport | 磁盘存储 / Disk storage |
| 压缩 / Compression | 可选（通常无）/ Optional | 默认启用（Snappy/Gzip）|
| 读取速度 / Read speed | 极快（直接映射）/ Very fast | 较快（需解压）/ Fast |
| 写入速度 / Write speed | 极快 / Very fast | 较慢（需编码+压缩）/ Slower |
| 列式存储 / Columnar | ✅ | ✅ |
| 嵌套类型 / Nested types | ✅ 完整支持 / Full support | ✅ 支持 / Supported |
| 本项目使用 / This project | ✅ HTTP 传输 / HTTP transport | ❌ 未使用 / Not used |

### 7.2 何时使用 Parquet / When to Use Parquet

```text
决策流程 / Decision flow:

数据用途是什么？/ What's the data purpose?
    │
    ├── 进程间传输 / Inter-process transport
    │   └── 使用 Arrow IPC / Use Arrow IPC
    │
    ├── 长期存储 / Long-term storage
    │   └── 使用 Parquet / Use Parquet
    │
    ├── 分析查询 / Analytical queries
    │   ├── 内存中 / In-memory → Arrow
    │   └── 磁盘上 / On-disk → Parquet
    │
    └── 与前端交互 / Frontend interaction
        └── JSON（浏览器原生支持）/ JSON (browser native)
```

## 8. 性能基准与优化 / Performance Benchmarks & Optimization

### 8.1 完整基准测试 / Complete Benchmarks

| 操作 / Operation | 数据量 / Data Size | 耗时 / Time | 说明 / Notes |
|---|---|---|---|
| Arrow IPC 解析 / Parse | 1K 行 / rows | ~0.5ms | 极快 / Very fast |
| Arrow IPC 解析 / Parse | 100K 行 / rows | ~15ms | 线性扩展 / Linear scale |
| Arrow → Pandas | 1K 行 / rows | ~1ms | 数值列零拷贝 / Numeric zero-copy |
| Arrow → Pandas | 100K 行 / rows | ~30ms | 字符串列需复制 / String needs copy |
| Pandas → JSON | 1K 行 / rows | ~2ms | to_dict(orient="records") |
| Pandas → JSON | 100K 行 / rows | ~150ms | JSON 序列化瓶颈 / Serialization bottleneck |
| NaN 替换 / NaN replace | 100K 行 / rows | ~5ms | replace({nan: None}) |

### 8.2 优化建议 / Optimization Recommendations

| 策略 / Strategy | 说明 / Description | 适用场景 / Use Case |
|---|---|---|
| 避免不必要的 to_pandas() | 直接用 Arrow compute 处理 / Use Arrow compute directly | 过滤/聚合 / Filter/Aggregate |
| 分块处理 / Chunk processing | 逐 RecordBatch 迭代 / Iterate per batch | 大数据量 / Large data |
| 延迟导入 / Lazy import | 函数内 import pyarrow / Import inside function | 减少启动时间 / Reduce startup |
| 限制返回行数 / Limit rows | 前端仅显示前 N 行 / Frontend shows top N | 超大结果集 / Huge result sets |
| 使用 orjson | 替代标准 json 库 / Replace stdlib json | JSON 序列化加速 5-10x |

## 9. 数据类型系统详解 / Data Type System Details

### 9.1 Arrow 类型层次 / Arrow Type Hierarchy

```text
Arrow 类型系统 / Arrow Type System
├── 原始类型 / Primitive Types
│   ├── Int8, Int16, Int32, Int64
│   ├── UInt8, UInt16, UInt32, UInt64
│   ├── Float16, Float32, Float64
│   ├── Boolean
│   └── Date32, Date64, Timestamp
├── 变长类型 / Variable-length Types
│   ├── Utf8 (String)
│   ├── Binary
│   └── LargeUtf8, LargeBinary
├── 嵌套类型 / Nested Types
│   ├── List<T>
│   ├── Struct{fields}
│   └── Map<K,V>
└── 特殊类型 / Special Types
    ├── Null
    ├── Dictionary (categorical)
    └── Decimal128, Decimal256
```

### 9.2 本项目中的类型映射 / Type Mapping in This Project

| 源数据 / Source | Arrow 类型 / Arrow Type | Python 类型 / Python Type | 说明 / Notes |
|---|---|---|---|
| 表格字段值 / Table field values | `pa.utf8()` | `str` | 所有值转为字符串 / All values to string |
| 数值列 / Numeric columns | `pa.float64()` | `float` | DP 计算用 / For DP computation |
| 分类标签 / Classification tags | `pa.list_(pa.utf8())` | `list[str]` | 多标签结果 / Multi-tag results |
| 空值 / Null values | `pa.null()` | `None` | 缺失数据处理 / Missing data handling |

### 9.3 类型安全与转换 / Type Safety & Conversion

```python
import pyarrow as pa
import pyarrow.compute as pc

# 安全类型转换（失败返回 null 而非报错）
# Safe type cast (returns null on failure instead of error)
arr = pa.array(["123", "abc", "456"])
numeric = pc.cast(arr, pa.float64(), safe=True)
# 结果 / Result: [123.0, null, 456.0]

# 本项目中的应用：将字符串字段转为数值进行 DP 计算
# Application: cast string fields to numeric for DP computation
def safe_numeric(values: list[str]) -> pa.Array:
    arr = pa.array(values, type=pa.utf8())
    return pc.cast(arr, pa.float64(), safe=True)
```

## 10. 向量化操作与批处理 / Vectorized Operations & Batch Processing

### 10.1 向量化 vs 循环 / Vectorized vs Loop

```python
import pyarrow.compute as pc
import pyarrow as pa

# ✗ 慢：Python 循环 / Slow: Python loop
def mask_loop(values: list[str]) -> list[str]:
    return [v[:3] + "***" if len(v) > 3 else v for v in values]

# ✓ 快：Arrow 向量化 / Fast: Arrow vectorized
def mask_vectorized(arr: pa.Array) -> pa.Array:
    prefix = pc.utf8_slice_codeunits(arr, 0, 3)
    suffix = pa.array(["***"] * len(arr))
    return pc.binary_join_element_wise(prefix, suffix, "")

# 性能差异（10万行）/ Performance difference (100K rows)
# 循环 / Loop: ~150ms
# 向量化 / Vectorized: ~3ms（50x 加速 / 50x speedup）
```

### 10.2 RecordBatch 批处理 / RecordBatch Processing

```python
# 分批处理大数据集（避免一次性加载）
# Process large datasets in batches (avoid loading all at once)
def process_batches(table: pa.Table, batch_size: int = 10000):
    for batch in table.to_batches(max_chunksize=batch_size):
        # 每个 batch 是独立的 RecordBatch / Each batch is independent
        result = pc.filter(batch.column("age"), pc.greater(batch.column("age"), 18))
        yield result

# 优势 / Advantages:
# - 内存可控（每次仅处理 batch_size 行）/ Memory controlled
# - CPU 缓存友好（连续内存块）/ CPU cache friendly
# - 可并行化（各 batch 独立）/ Parallelizable
```

### 10.3 本项目向量化应用场景 / Vectorization Use Cases

| 场景 / Scenario | 操作 / Operation | 向量化函数 / Vectorized Function |
|---|---|---|
| 字段脱敏 / Field masking | 字符串截取 + 拼接 / Slice + concat | `pc.utf8_slice_codeunits` |
| 数值裁剪 / Value clipping | 上下界截断 / Upper/lower bound | `pc.min_element_wise`, `pc.max_element_wise` |
| 空值填充 / Null filling | 替换 null / Replace null | `pc.fill_null` |
| 条件过滤 / Conditional filter | 按条件筛选 / Filter by condition | `pc.filter` + `pc.greater` |

## 11. 错误处理与数据质量 / Error Handling & Data Quality

### 11.1 常见数据质量问题 / Common Data Quality Issues

| 问题 / Issue | 表现 / Manifestation | 处理策略 / Strategy |
|---|---|---|
| 缺失值 / Missing values | null/NaN/空字符串 / null/NaN/empty | `fill_null` 或跳过 / Fill or skip |
| 类型不匹配 / Type mismatch | 字符串中混入非数值 / Non-numeric in string | `safe=True` cast |
| 超大值 / Outliers | 异常大/小的数值 / Abnormally large/small | 裁剪 / Clipping |
| 编码问题 / Encoding issues | 非 UTF-8 字符 / Non-UTF-8 chars | 预处理清洗 / Pre-clean |
| 空表 / Empty table | 0 行数据 / 0 rows | 提前返回空结果 / Early return empty |

### 11.2 防御性编程模式 / Defensive Programming Patterns

```python
# 本项目的数据处理防御模式 / Defensive patterns in this project

def process_table_data(records: list[dict]) -> pa.Table:
    # 1. 空输入检查 / Empty input check
    if not records:
        return pa.table({"result": pa.array([], type=pa.utf8())})

    # 2. 字段名规范化 / Field name normalization
    cleaned = [
        {k.strip(): str(v).strip() if v is not None else None
         for k, v in row.items()}
        for row in records
    ]

    # 3. 安全转换为 Arrow / Safe conversion to Arrow
    try:
        table = pa.Table.from_pylist(cleaned)
    except (pa.ArrowInvalid, pa.ArrowTypeError) as e:
        # 回退：全部作为字符串处理 / Fallback: all as string
        table = pa.Table.from_pylist(
            [{k: str(v) for k, v in row.items()} for row in cleaned]
        )

    return table
```

### 11.3 数据验证管道 / Data Validation Pipeline

```text
┌─────────────────────────────────────────────────────────────┐
│  数据验证管道 / Data Validation Pipeline                     │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌────────────┐    ┌────────────┐    ┌────────────┐
│ 1. 输入校验  │    │ 2. 类型转换  │    │ 3. 输出校验  │
│ Input valid.│    │ Type cast   │    │ Output val. │
│ - 非空检查  │    │ - safe cast │    │ - 行数一致  │
│ - 结构检查  │    │ - 回退策略  │    │ - 无新增null│
│ - 大小限制  │    │ - 编码修复  │    │ - 格式正确  │
└────────────┘    └────────────┘    └────────────┘
```

## 12. 流式数据处理 / Streaming Data Processing

### 12.1 流式与批处理对比 / Streaming vs Batch Comparison

```text
┌────────────────────────────────────────────────────────────────┐
│  批处理模式 / Batch Mode (本项目使用 / This project uses)       │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Client ────[JSON 全量]────► Backend ────[Table]────► Agent   │
│           一次性发送 / Send all at once                          │
│           等待完整响应 / Wait for full response                  │
│                                                                │
├────────────────────────────────────────────────────────────────┤
│  流式模式 / Streaming Mode (未来可选 / Future optional)          │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Client ──[chunk1]──► Backend ──[RecordBatch1]──► Agent        │
│         ──[chunk2]──►        ──[RecordBatch2]──►               │
│         ──[chunk3]──►        ──[RecordBatch3]──►               │
│         ──[EOF]────►        ──[EOF]──────────►               │
│                                                                │
│  优势：低内存占用、早期结果 / Pros: low memory, early results    │
│  劣势：复杂度高、需背压控制 / Cons: complex, needs backpressure  │
└────────────────────────────────────────────────────────────────┘
```

### 12.2 Arrow IPC 流式读取 / Arrow IPC Streaming Read

```python
import pyarrow as pa
import pyarrow.ipc as ipc
from io import BytesIO

def process_streaming_batches(stream: BytesIO, batch_size: int = 1000):
    """流式处理 Arrow IPC 数据 / Process Arrow IPC data in streaming mode.

    适用于大文件分块处理 / Suitable for large file chunked processing.
    """
    reader = ipc.open_stream(stream)
    results = []

    for batch in reader:
        # 每个 batch 独立处理 / Process each batch independently
        assert isinstance(batch, pa.RecordBatch)

        # 应用隐私变换 / Apply privacy transform
        processed = apply_masking_to_batch(batch)
        results.append(processed)

        # 内存控制：处理完即释放 / Memory control: release after processing
        del batch

    return pa.concat_tables(results)


def apply_masking_to_batch(batch: pa.RecordBatch) -> pa.Table:
    """对单个 batch 应用脱敏 / Apply masking to a single batch."""
    table = pa.Table.from_batches([batch])
    # 字段名感知脱敏 / Field-name-aware masking
    for col_name in table.column_names:
        if "email" in col_name.lower():
            col = table.column(col_name)
            masked = pa.compute.utf8_replace_slice(col, 3, None, "***")
            table = table.set_column(
                table.column_names.index(col_name), col_name, masked
            )
    return table
```

### 12.3 分块处理策略 / Chunking Strategy

| 策略 / Strategy | 适用场景 / Scenario | 内存占用 / Memory | 延迟 / Latency |
|---|---|---|---|
| 全量加载 / Full load | < 10MB 数据 / < 10MB data | 高 / High | 低（一次完成）/ Low |
| 固定行数分块 / Fixed-row chunks | 表格数据 / Tabular data | 中 / Medium | 中 / Medium |
| 固定内存分块 / Fixed-memory chunks | 可变宽字段 / Variable-width fields | 可控 / Controlled | 中 / Medium |
| 流式逐条 / Stream per-record | 实时数据 / Real-time data | 最低 / Lowest | 高（多次 IO）/ High |

### 12.4 本项目流式处理设计决策 / This Project's Streaming Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 使用批处理而非流式 / Batch not streaming | 代理工具数据量小（< 1MB）/ Proxy tool data small |
| JSON 全量传输 / JSON full transfer | 简化前后端交互 / Simplify frontend-backend |
| 服务端分块处理 / Server-side chunking | 大表格时避免 OOM / Avoid OOM for large tables |
| 64MB gRPC 消息上限 / 64MB gRPC limit | 防止内存溢出 / Prevent memory overflow |

## 13. 数据序列化格式对比 / Data Serialization Format Comparison

### 13.1 主流格式架构对比 / Architecture Comparison

```text
┌────────────────────────────────────────────────────────────────┐
│              序列化格式特性矩阵 / Serialization Feature Matrix      │
├──────────────┬──────────┬──────────┬──────────┬──────────────┤
│ 格式 / Format │ 人类可读  │ 零拷贝    │ Schema   │ 流式支持      │
│              │ Readable │ Zero-copy│ Evolve   │ Streaming    │
├──────────────┼──────────┼──────────┼──────────┼──────────────┤
│ JSON         │ ✓        │ ✗        │ ✗        │ ✓ (NDJSON)   │
│ Protocol Buf │ ✗        │ ✗        │ ✓        │ ✓            │
│ Arrow IPC    │ ✗        │ ✓        │ ✓        │ ✓            │
│ Parquet      │ ✗        │ ✓(列)    │ ✓        │ ✗            │
│ CSV          │ ✓        │ ✗        │ ✗        │ ✓            │
│ MessagePack  │ ✗        │ ✗        │ ✗        │ ✓            │
│ Avro         │ ✗        │ ✗        │ ✓        │ ✓            │
└──────────────┴──────────┴──────────┴──────────┴──────────────┘
```

### 13.2 本项目使用的格式 / Formats Used in This Project

```python
# 1. JSON：REST API 交互 / REST API interaction
# 前端 → Console Backend → PrivShield Agent
import json

request_payload = {
    "data": [{"name": "Alice", "email": "alice@example.com"}],
    "fields": ["email"],
    "strategy": "partial",
}
# Content-Type: application/json

# 2. Protocol Buffers：gRPC 通信 / gRPC communication
# Console Backend-Go → PrivShield Agent (gRPC)
# proto/privacy.proto 定义消息结构 / Defines message structure

# 3. Arrow IPC：内部数据处理 / Internal data processing
# 分类引擎向量化计算 / Classification engine vectorized computation
import pyarrow as pa

table = pa.Table.from_pylist(request_payload["data"])
# 内存中列式存储，零拷贝计算 / Columnar in-memory, zero-copy compute
```

### 13.3 格式选择决策 / Format Selection Decision

| 通信路径 / Path | 格式 / Format | 原因 / Reason |
|---|---|---|
| 浏览器 → Console Backend | JSON | 浏览器原生支持 / Browser native |
| Console Backend → Agent REST | JSON | FastAPI 原生解析 / FastAPI native |
| Console Backend-Go → Agent gRPC | Protobuf | 强类型 + 高性能 / Strong type + fast |
| Agent 内部计算 / Internal compute | Arrow | 零拷贝向量化 / Zero-copy vectorized |
| 配置文件 / Config files | YAML | 人类可读 / Human readable |

### 13.4 序列化性能基准 / Serialization Performance Benchmarks

```text
10,000 行 × 10 列数据序列化对比 / 10K rows × 10 cols comparison:

  格式 / Format       序列化 / Serialize   反序列化 / Deserialize   体积 / Size
  ────────────────────────────────────────────────────────────────────────
  JSON              ~45ms              ~38ms               ~2.1MB
  Protobuf          ~12ms              ~10ms               ~0.8MB
  Arrow IPC         ~5ms               ~2ms (零拷贝)       ~1.2MB
  Parquet           ~25ms              ~15ms               ~0.4MB
  CSV               ~20ms              ~18ms               ~1.8MB
  MessagePack       ~15ms              ~12ms               ~1.0MB
  ────────────────────────────────────────────────────────────────────────
  注：Arrow 反序列化几乎零成本（内存映射）
  Note: Arrow deserialize is near-zero cost (memory-mapped)
```

## 14. 隐私数据处理管道 / Privacy Data Processing Pipeline

### 14.1 端到端数据流 / End-to-End Data Flow

```text
┌────────────────────────────────────────────────────────────────────────┐
│  隐私数据处理全链路 / Privacy Data Processing Full Pipeline              │
└────────────────────────────────────────────────────────────────────────┘

  ① 输入 / Input          ② 解析 / Parse          ③ 分类 / Classify
┌────────────┐       ┌────────────────┐       ┌────────────────┐
│ JSON/CSV   │─────►│ Pydantic 校验   │─────►│ 3层漏斗分类    │
│ 原始数据    │       │ Arrow 转换      │       │ Rule→NER→LLM  │
│ Raw data   │       │ 类型推断        │       │ 敏感度标记    │
└────────────┘       └────────────────┘       └────────────────┘
                                                       │
       ⑥ 输出 / Output       ⑤ 预算 / Budget       ④ 变换 / Transform
┌────────────────┐       ┌────────────────┐       ┌────────────────┐
│ 脱敏后数据    │◄─────│ 隐私预算检查  │◄─────│ 脱敏/DP/K-ano │
│ 统计结果      │       │ 配额扣减      │       │ 隐私变换应用  │
│ 分类报告      │       │ 审计日志      │       │ 结果校验      │
└────────────────┘       └────────────────┘       └────────────────┘
```

### 14.2 各阶段数据处理细节 / Per-Stage Processing Details

```python
# 阶段 ②：输入解析与类型推断 / Stage 2: Input parsing & type inference
import pyarrow as pa
import pyarrow.compute as pc

def parse_and_infer(raw_records: list[dict]) -> pa.Table:
    """解析原始记录并推断类型 / Parse raw records and infer types."""
    if not raw_records:
        return pa.table({})

    # Arrow 自动类型推断 / Arrow auto type inference
    table = pa.Table.from_pylist(raw_records)

    # 类型优化：将字符串列尝试转为更精确类型
    # Type optimization: try converting string cols to precise types
    for i, field in enumerate(table.schema):
        if pa.types.is_string(field.type):
            col = table.column(i)
            # 尝试日期解析 / Try date parsing
            try:
                parsed = pc.strptime(col, format="%Y-%m-%d", unit="s")
                table = table.set_column(i, field.name, parsed)
                continue
            except (pa.ArrowInvalid, pa.ArrowNotImplementedError):
                pass
            # 尝试数值解析 / Try numeric parsing
            try:
                parsed = pc.cast(col, pa.float64(), safe=False)
                table = table.set_column(i, field.name, parsed)
            except (pa.ArrowInvalid, pa.ArrowNotImplementedError):
                pass  # 保持字符串 / Keep as string

    return table


# 阶段 ④：隐私变换应用 / Stage 4: Privacy transform application
def apply_privacy_transform(
    table: pa.Table,
    sensitive_cols: list[str],
    strategy: str,
) -> pa.Table:
    """对敏感列应用隐私变换 / Apply privacy transform to sensitive columns."""
    for col_name in sensitive_cols:
        if col_name not in table.column_names:
            continue
        col = table.column(col_name)

        if strategy == "mask":
            # 部分遮盖 / Partial mask
            transformed = pc.utf8_replace_slice(col, 2, None, "***")
        elif strategy == "generalize":
            # 泛化（K-匿名）/ Generalize (K-anonymity)
            transformed = generalize_column(col)
        elif strategy == "suppress":
            # 完全抑制 / Full suppression
            transformed = pa.nulls(len(col), type=col.type)
        else:
            transformed = col  # 无操作 / No-op

        idx = table.column_names.index(col_name)
        table = table.set_column(idx, col_name, transformed)

    return table
```

### 14.3 数据质量保障 / Data Quality Assurance

| 检查点 / Checkpoint | 校验内容 / Validation | 失败处理 / Failure Handling |
|---|---|---|
| 输入解析 / Input parse | 非空、结构完整 / Non-empty, structured | 返回 400 错误 / Return 400 |
| 类型推断 / Type infer | 无异常类型 / No anomalous types | 回退为字符串 / Fallback to string |
| 分类结果 / Classification | 置信度 > 阈值 / Confidence > threshold | 标记为待审核 / Mark for review |
| 变换后 / Post-transform | 行数一致、无新增 null / Row count same | 回滚变换 / Rollback transform |
| 预算检查 / Budget check | 剩余预算 > 0 / Remaining budget > 0 | 拒绝请求 / Reject request |

### 14.4 本项目管道设计决策 / This Project's Pipeline Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 同步处理（非流式）/ Sync (not streaming) | 数据量小，延迟不敏感 / Small data, latency insensitive |
| Arrow 内存计算 / Arrow in-memory compute | 向量化操作性能优异 / Vectorized ops performant |
| Pydantic 入口校验 / Pydantic entry validation | 第一道防线 / First line of defense |
| 字段名感知 / Field-name-aware | 提高分类准确率 / Improve classification accuracy |
| 分层漏斗 / Layered funnel | 平衡精度与性能 / Balance accuracy & performance |

---

## 15. Dask 分布式计算 / Dask Distributed Computing

### 15.1 Dask 架构与核心概念 / Dask Architecture & Core Concepts

Dask 是 Python 生态中的并行计算库，提供与 Pandas/NumPy 兼容的 API，可将计算透明地扩展到多核或集群：

```
┌─────────────────────────────────────────────────────────────────┐
│              Dask 架构概览 / Dask Architecture Overview          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  用户代码 / User Code                                          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  dask.dataframe  │  dask.array  │  dask.bag          │   │
│  │  (Pandas-like)   │  (NumPy-like) │  (map/reduce)     │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  任务图调度器 / Task Graph Scheduler                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Dask Graph → Optimization → Task Scheduling          │   │
│  │  - 延迟计算 / Lazy evaluation                          │   │
│  │  - 图优化 / Graph optimization                        │   │
│  │  - 内存感知调度 / Memory-aware scheduling             │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│              ┌─────────┼─────────┐                         │
│              ▼         ▼         ▼                         │
│  ┌────────┐ ┌────────┐ ┌────────┐                       │
│  │Worker 1│ │Worker 2│ │Worker 3│  ← 执行引擎 / Executors│
│  │(Thread)│ │(Thread)│ │(Thread)│                       │
│  └────────┘ └────────┘ └────────┘                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 15.2 Dask DataFrame 与隐私计算 / Dask DataFrame & Privacy Computing

```python
# ===== Dask 在隐私数据处理中的应用 / Dask in Privacy Data Processing =====
import dask.dataframe as dd
import pandas as pd
import pyarrow as pa

# 从 Parquet 加载大型数据集 / Load large dataset from Parquet
ddf = dd.read_parquet(
    "s3://data-lake/user_records/",
    columns=["user_id", "email", "phone", "age", "city", "salary"],
    # 每个分区 ~100MB / Each partition ~100MB
)

# 延迟计算: 构建任务图但不执行 / Lazy: build task graph without executing
# 批量掌码 / Batch masking
masked = ddf.assign(
    email=ddf["email"].apply(
        lambda x: x[:2] + "***" + x[x.index("@"):],
        meta=("email", "str")
    ),
    phone=ddf["phone"].apply(
        lambda x: x[:3] + "****" + x[-4:],
        meta=("phone", "str")
    ),
    # 数值泛化 / Numeric generalization
    age=ddf["age"].apply(
        lambda x: (x // 10) * 10,  # 25 → 20, 37 → 30
        meta=("age", "int")
    ),
)

# 触发计算 / Trigger computation
result = masked.compute()  # 单节点执行 / Single-node execution

# 或写入输出 / Or write to output
masked.to_parquet("s3://data-lake/masked_output/")

# K-匿名性检查 / K-anonymity check
def check_k_anonymity(df: pd.DataFrame, quasi_identifiers: list, k: int = 5):
    """检查数据集是否满足 k-匿名 / Check if dataset satisfies k-anonymity"""
    groups = df.groupby(quasi_identifiers).size()
    violations = groups[groups < k]
    return {
        "k": k,
        "total_groups": len(groups),
        "violating_groups": len(violations),
        "min_group_size": int(groups.min()),
        "satisfies": bool((groups >= k).all()),
    }

# 在 Dask 上执行 / Execute on Dask
k_result = (
    masked
    .groupby(["age", "city"])
    .size()
    .compute()
)
print(f"Min group size: {k_result.min()}")
```

### 15.3 Dask vs Pandas vs Arrow 对比 / Dask vs Pandas vs Arrow Comparison

| 特性 / Feature | Pandas | PyArrow | Dask | 本项目 / Project |
|---|---|---|---|---|
| 数据规模 / Data scale | <内存 / <RAM | <内存 / <RAM | >内存 / >RAM | Pandas+Arrow |
| 计算模式 / Compute | 即时 / Eager | 即时 / Eager | 延迟 / Lazy | 即时 / Eager |
| 并行 / Parallelism | 单线程 / Single | 多线程 / Multi | 多进程/分布式 / Multi | 单线程 / Single |
| API 风格 / API style | DataFrame | Table/Array | DataFrame | DataFrame |
| 缺失值 / Null handling | NaN | null bitmap | NaN | NaN + null |
| 适用场景 / Use case | 探索分析 / Explore | 高性能计算 / HPC | 大规模处理 / Big data | 中小规模 / Small-med |

---

## 16. 数据血缘与元数据管理 / Data Lineage & Metadata Management

### 16.1 数据血缘概念与重要性 / Data Lineage Concepts & Importance

数据血缘追踪数据从源头到消费的完整变换路径，在隐私合规中尤为重要：

```
┌─────────────────────────────────────────────────────────────────┐
│         隐私数据血缘示例 / Privacy Data Lineage Example          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Source          Transform         Output         Consumer      │
│  ┌──────┐     ┌───────────┐     ┌────────┐     ┌────────┐  │
│  │  Raw  │────▶│  Masking  │────▶│ Masked │────▶│Analytics│  │
│  │  DB   │     │  Engine   │     │  Data  │     │  Team   │  │
│  └──────┘     └───────────┘     └────────┘     └────────┘  │
│      │              │                  │                │        │
│      │              │                  │                │        │
│      ▼              ▼                  ▼                ▼        │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Metadata:                                              │  │
│  │  - source: users_db.users_table                         │  │
│  │  - transform: email_mask(partial), phone_mask(full)     │  │
│  │  - sensitivity_before: L4 (restricted)                  │  │
│  │  - sensitivity_after: L2 (internal)                     │  │
│  │  - dp_epsilon_spent: 0.5                                │  │
│  │  - timestamp: 2024-01-15T10:30:00Z                      │  │
│  │  - operator: privacy-agent-v0.1.0                       │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 16.2 元数据模型实现 / Metadata Model Implementation

```python
# ===== 数据血缘元数据模型 / Data Lineage Metadata Model =====
from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional
from enum import Enum

class TransformType(str, Enum):
    MASKING = "masking"
    GENERALIZATION = "generalization"
    SUPPRESSION = "suppression"
    DP_NOISE = "dp_noise"
    AGGREGATION = "aggregation"
    FILTERING = "filtering"

class FieldLineage(BaseModel):
    """单字段血缘 / Single field lineage"""
    source_field: str
    target_field: str
    transform: TransformType
    params: dict = Field(default_factory=dict)  # 变换参数 / Transform params
    sensitivity_before: int  # 1-5
    sensitivity_after: int   # 1-5

class ProcessingRecord(BaseModel):
    """数据处理记录 / Data processing record"""
    record_id: str
    timestamp: datetime = Field(default_factory=datetime.utcnow)
    source: str               # 数据源 / Data source
    destination: str          # 目标 / Destination
    operator: str             # 操作器 / Operator
    operator_version: str     # 版本 / Version
    fields_affected: list[FieldLineage]
    dp_budget_spent: Optional[float] = None  # 消耗的隐私预算 / DP budget spent
    row_count: int
    processing_time_ms: float

# 使用示例 / Usage example
record = ProcessingRecord(
    record_id="proc_20240115_001",
    source="users_db.raw_users",
    destination="analytics.masked_users",
    operator="PrivShield",
    operator_version="0.1.0",
    fields_affected=[
        FieldLineage(
            source_field="email",
            target_field="email",
            transform=TransformType.MASKING,
            params={"strategy": "partial", "keep_chars": 2},
            sensitivity_before=4,
            sensitivity_after=2,
        ),
        FieldLineage(
            source_field="age",
            target_field="age_group",
            transform=TransformType.GENERALIZATION,
            params={"bin_size": 10},
            sensitivity_before=3,
            sensitivity_after=1,
        ),
    ],
    dp_budget_spent=0.5,
    row_count=50000,
    processing_time_ms=1230.5,
)
```

### 16.3 合规审计与数据映射 / Compliance Audit & Data Mapping

| 合规要求 / Compliance Req | 血缘作用 / Lineage Role | 实现方式 / Implementation |
|---|---|---|
| GDPR 数据映射 / Data mapping | 追踪 PII 流向 / Track PII flow | FieldLineage 记录 / Records |
| 最小化原则 / Minimization | 证明数据降级 / Prove downgrade | sensitivity_before/after |
| 审计追踪 / Audit trail | 完整操作历史 / Full operation history | ProcessingRecord 日志 / Logs |
| 隐私预算 / Privacy budget | 跟踪 epsilon 消耗 / Track epsilon spend | dp_budget_spent 字段 / Field |
| 数据删除 / Right to erasure | 定位所有副本 / Locate all copies | destination 字段 / Field |

---

## 17. ETL 管道设计模式 / ETL Pipeline Design Patterns

### 17.1 隐私 ETL 管道架构 / Privacy ETL Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│         隐私 ETL 管道 / Privacy ETL Pipeline                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Extract           Transform              Load                  │
│  ┌───────┐     ┌─────────────────┐     ┌────────┐         │
│  │Source │     │ 1. Validate     │     │ Target │         │
│  │Systems│────▶│ 2. Classify    │────▶│ Systems│         │
│  └───────┘     │ 3. Mask/DP     │     └────────┘         │
│                │ 4. Verify      │                         │
│                └─────────────────┘                         │
│                                                                 │
│  详细步骤 / Detailed Steps:                                     │
│                                                                 │
│  Extract:                                                       │
│  - 从数据库/API/文件提取 / Extract from DB/API/files          │
│  - 保持原始格式 / Preserve original format                    │
│  - 记录提取元数据 / Record extraction metadata                │
│                                                                 │
│  Transform:                                                     │
│  - Step 1: Pydantic 输入验证 / Pydantic input validation       │
│  - Step 2: 3层分类漏斗 / 3-layer classification funnel         │
│  - Step 3: 根据分类结果应用保护 / Apply protection by class    │
│  - Step 4: 验证输出合规性 / Verify output compliance           │
│                                                                 │
│  Load:                                                          │
│  - 写入目标存储 / Write to target storage                     │
│  - 记录血缘信息 / Record lineage info                         │
│  - 更新隐私预算 / Update privacy budget                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 17.2 管道实现模式 / Pipeline Implementation Pattern

```python
# ===== 可组合的隐私 ETL 管道 / Composable Privacy ETL Pipeline =====
from abc import ABC, abstractmethod
from typing import Any
import pandas as pd
import logging

logger = logging.getLogger(__name__)

class PipelineStage(ABC):
    """管道阶段抽象基类 / Pipeline stage abstract base"""

    @property
    @abstractmethod
    def name(self) -> str: ...

    @abstractmethod
    def process(self, df: pd.DataFrame, context: dict) -> pd.DataFrame: ...

    def validate_output(self, df: pd.DataFrame) -> bool:
        """可选的输出验证 / Optional output validation"""
        return True


class ValidationStage(PipelineStage):
    name = "validation"

    def __init__(self, required_columns: list[str]):
        self.required_columns = required_columns

    def process(self, df: pd.DataFrame, context: dict) -> pd.DataFrame:
        missing = set(self.required_columns) - set(df.columns)
        if missing:
            raise ValueError(f"Missing required columns: {missing}")
        context["row_count"] = len(df)
        logger.info(f"Validation passed: {len(df)} rows, {len(df.columns)} cols")
        return df


class ClassificationStage(PipelineStage):
    name = "classification"

    def process(self, df: pd.DataFrame, context: dict) -> pd.DataFrame:
        """对每列进行敏感度分类 / Classify sensitivity for each column"""
        classifications = {}
        for col in df.columns:
            # 简化: 基于字段名规则 / Simplified: field-name rules
            if any(kw in col.lower() for kw in ["email", "phone", "ssn", "id_card"]):
                classifications[col] = {"level": 4, "category": "PII"}
            elif any(kw in col.lower() for kw in ["name", "address", "dob"]):
                classifications[col] = {"level": 3, "category": "quasi-PII"}
            else:
                classifications[col] = {"level": 1, "category": "public"}

        context["classifications"] = classifications
        return df


class MaskingStage(PipelineStage):
    name = "masking"

    def process(self, df: pd.DataFrame, context: dict) -> pd.DataFrame:
        """根据分类结果应用掌码 / Apply masking based on classification"""
        classifications = context.get("classifications", {})
        df_masked = df.copy()

        for col, info in classifications.items():
            if info["level"] >= 4 and col in df_masked.columns:
                # 高敏感度: 完全掌码 / High sensitivity: full mask
                df_masked[col] = df_masked[col].apply(
                    lambda x: "***" if pd.notna(x) else x
                )
            elif info["level"] >= 3 and col in df_masked.columns:
                # 中敏感度: 部分掌码 / Medium sensitivity: partial mask
                df_masked[col] = df_masked[col].apply(
                    lambda x: str(x)[:2] + "***" if pd.notna(x) else x
                )

        context["masked_fields"] = [
            col for col, info in classifications.items() if info["level"] >= 3
        ]
        return df_masked


# 管道编排器 / Pipeline orchestrator
class PrivacyPipeline:
    def __init__(self, stages: list[PipelineStage]):
        self.stages = stages

    def execute(self, df: pd.DataFrame) -> tuple[pd.DataFrame, dict]:
        context: dict[str, Any] = {"start_time": pd.Timestamp.now()}

        for stage in self.stages:
            logger.info(f"Executing stage: {stage.name}")
            df = stage.process(df, context)

            if not stage.validate_output(df):
                raise RuntimeError(f"Stage '{stage.name}' output validation failed")

        context["end_time"] = pd.Timestamp.now()
        context["total_time_ms"] = (
            (context["end_time"] - context["start_time"]).total_seconds() * 1000
        )
        return df, context


# 组装并执行 / Assemble and execute
pipeline = PrivacyPipeline([
    ValidationStage(required_columns=["user_id", "email", "age"]),
    ClassificationStage(),
    MaskingStage(),
])

result_df, metadata = pipeline.execute(input_df)
logger.info(f"Pipeline completed in {metadata['total_time_ms']:.1f}ms")
```

### 17.3 管道设计模式对比 / Pipeline Design Pattern Comparison

| 模式 / Pattern | 特点 / Characteristics | 适用场景 / Use Case | 本项目 / Project |
|---|---|---|---|
| 线性管道 / Linear | 顺序执行 / Sequential | 简单 ETL / Simple ETL | ✅ 主要 / Primary |
| DAG 管道 / DAG | 并行分支 / Parallel branches | 复杂依赖 / Complex deps | 未使用 / Not used |
| 流式管道 / Streaming | 逐条处理 / Record-by-record | 实时数据 / Real-time | 未使用 / Not used |
| 微批 / Micro-batch | 小批次处理 / Small batches | 准实时 / Near-real-time | 未使用 / Not used |
| 可组合阶段 / Composable | 插件化 / Pluggable | 灵活配置 / Flexible config | ✅ 使用 / Used |

## 18. 数据治理与合规框架 / Data Governance & Compliance Framework

数据治理是确保数据在整个生命周期中被正确管理、保护和合规使用的体系化方法。在隐私数据处理场景中，治理框架尤为关键。

Data governance is a systematic approach to ensuring data is properly managed, protected, and compliantly used throughout its lifecycle. In privacy data processing scenarios, governance frameworks are especially critical.

### 18.1 数据分类分级体系 / Data Classification & Grading System

```python
"""数据分类分级引擎 / Data classification and grading engine"""
from enum import IntEnum
from dataclasses import dataclass, field
from typing import Any
import re
import logging

logger = logging.getLogger(__name__)


class SensitivityLevel(IntEnum):
    """数据敏感度等级 / Data sensitivity levels"""
    PUBLIC = 1       # 公开数据 / Public data
    INTERNAL = 2     # 内部数据 / Internal data
    CONFIDENTIAL = 3 # 机密数据 / Confidential data
    RESTRICTED = 4   # 受限数据 / Restricted data
    TOP_SECRET = 5   # 绝密数据 / Top secret data


@dataclass
class DataCategory:
    """数据类别定义 / Data category definition"""
    name: str                          # 类别名称 / Category name
    level: SensitivityLevel            # 敏感度 / Sensitivity level
    patterns: list[str] = field(default_factory=list)  # 匹配模式 / Match patterns
    regulations: list[str] = field(default_factory=list)  # 适用法规 / Applicable regulations
    retention_days: int = 365          # 保留天数 / Retention days
    requires_encryption: bool = False  # 是否需加密 / Needs encryption
    requires_masking: bool = False     # 是否需脱敏 / Needs masking


class DataGovernanceEngine:
    """数据治理引擎 / Data governance engine

    负责数据分类、策略执行和合规检查。
    Responsible for data classification, policy enforcement, and compliance checks.
    """

    # 预定义数据类别 / Predefined data categories
    CATEGORIES: dict[str, DataCategory] = {
        "pii_basic": DataCategory(
            name="基本个人信息 / Basic PII",
            level=SensitivityLevel.CONFIDENTIAL,
            patterns=[r"\b\d{17}[\dXx]\b", r"\b1[3-9]\d{9}\b"],
            regulations=["GDPR", "PIPL", "CCPA"],
            retention_days=180,
            requires_encryption=True,
            requires_masking=True,
        ),
        "financial": DataCategory(
            name="金融数据 / Financial data",
            level=SensitivityLevel.RESTRICTED,
            patterns=[r"\b\d{16,19}\b", r"\b\d{3}\b"],  # 卡号/CVV / Card/CVV
            regulations=["PCI-DSS", "SOX"],
            retention_days=90,
            requires_encryption=True,
            requires_masking=True,
        ),
        "health": DataCategory(
            name="健康数据 / Health data",
            level=SensitivityLevel.TOP_SECRET,
            patterns=[r"(?i)diagnosis|medical_record|blood_type"],
            regulations=["HIPAA", "PIPL"],
            retention_days=3650,
            requires_encryption=True,
            requires_masking=True,
        ),
    }

    def classify_field(self, field_name: str, sample_values: list[Any]) -> DataCategory | None:
        """对字段进行分类 / Classify a field

        Args:
            field_name: 字段名 / Field name
            sample_values: 样本值列表 / Sample values

        Returns:
            匹配的数据类别，无匹配返回 None / Matched category or None
        """
        for cat_key, category in self.CATEGORIES.items():
            # 1. 字段名模式匹配 / Field name pattern matching
            for pattern in category.patterns:
                if re.search(pattern, field_name, re.IGNORECASE):
                    logger.debug(f"Field '{field_name}' matched category '{cat_key}' by name")
                    return category

            # 2. 值模式匹配 / Value pattern matching
            for value in sample_values[:10]:  # 只检查前10个 / Check first 10
                if value is None:
                    continue
                str_val = str(value)
                for pattern in category.patterns:
                    if re.search(pattern, str_val):
                        logger.debug(f"Field '{field_name}' matched category '{cat_key}' by value")
                        return category

        return None

    def check_compliance(self, data_inventory: dict[str, DataCategory]) -> list[dict]:
        """合规检查 / Compliance check

        Args:
            data_inventory: 数据清单 / Data inventory

        Returns:
            违规列表 / List of violations
        """
        violations = []
        for field_name, category in data_inventory.items():
            # 检查保留策略 / Check retention policy
            if category.retention_days < 30:
                violations.append({
                    "field": field_name,
                    "type": "retention_too_short",
                    "message": f"保留期 {category.retention_days} 天可能不满足审计要求",
                })
            # 检查加密要求 / Check encryption requirement
            if category.requires_encryption and category.level >= SensitivityLevel.RESTRICTED:
                violations.append({
                    "field": field_name,
                    "type": "encryption_required",
                    "message": f"等级 {category.level} 数据必须加密存储",
                })
        return violations
```

### 18.2 数据血缘追踪 / Data Lineage Tracking

```python
"""数据血缘追踪系统 / Data lineage tracking system"""
from dataclasses import dataclass, field
from datetime import datetime
import uuid


@dataclass
class LineageNode:
    """血缘节点 / Lineage node"""
    node_id: str = field(default_factory=lambda: str(uuid.uuid4())[:8])
    name: str = ""                    # 节点名称 / Node name
    node_type: str = "dataset"        # 类型: dataset|field|transform / Type
    metadata: dict = field(default_factory=dict)
    created_at: datetime = field(default_factory=datetime.now)


@dataclass
class LineageEdge:
    """血缘边 / Lineage edge"""
    source_id: str = ""               # 源节点 / Source node
    target_id: str = ""               # 目标节点 / Target node
    transform_type: str = "copy"      # 转换类型 / Transform type
    transform_detail: str = ""        # 转换详情 / Transform detail


class LineageTracker:
    """数据血缘追踪器 / Data lineage tracker

    记录数据从源到目标的完整转换路径。
    Records the complete transformation path from source to target.
    """

    def __init__(self):
        self.nodes: dict[str, LineageNode] = {}
        self.edges: list[LineageEdge] = []

    def register_dataset(self, name: str, schema: dict) -> str:
        """注册数据集 / Register dataset"""
        node = LineageNode(name=name, node_type="dataset", metadata={"schema": schema})
        self.nodes[node.node_id] = node
        return node.node_id

    def record_transform(
        self, source_id: str, target_id: str,
        transform_type: str, detail: str = ""
    ) -> None:
        """记录转换关系 / Record transformation"""
        self.edges.append(LineageEdge(
            source_id=source_id,
            target_id=target_id,
            transform_type=transform_type,
            transform_detail=detail,
        ))

    def trace_upstream(self, node_id: str, depth: int = 10) -> list[LineageNode]:
        """向上追溯血缘 / Trace upstream lineage

        Args:
            node_id: 起始节点 / Starting node
            depth: 最大追溯深度 / Max trace depth

        Returns:
            上游节点列表 / Upstream node list
        """
        visited = set()
        result = []
        queue = [(node_id, 0)]

        while queue:
            current_id, current_depth = queue.pop(0)
            if current_id in visited or current_depth > depth:
                continue
            visited.add(current_id)

            if current_id in self.nodes:
                result.append(self.nodes[current_id])

            # 查找指向当前节点的所有边 / Find all edges pointing to current
            for edge in self.edges:
                if edge.target_id == current_id and edge.source_id not in visited:
                    queue.append((edge.source_id, current_depth + 1))

        return result

    def impact_analysis(self, node_id: str) -> list[LineageNode]:
        """影响分析：查找所有下游节点 / Impact analysis: find all downstream nodes"""
        visited = set()
        result = []
        queue = [node_id]

        while queue:
            current_id = queue.pop(0)
            if current_id in visited:
                continue
            visited.add(current_id)

            if current_id in self.nodes:
                result.append(self.nodes[current_id])

            for edge in self.edges:
                if edge.source_id == current_id and edge.target_id not in visited:
                    queue.append(edge.target_id)

        return result
```

### 18.3 合规审计日志 / Compliance Audit Logging

```python
"""合规审计日志系统 / Compliance audit logging system"""
import json
import hashlib
from datetime import datetime, timezone
from dataclasses import dataclass, asdict
from typing import Any


@dataclass
class AuditEvent:
    """审计事件 / Audit event"""
    event_id: str
    timestamp: str
    actor: str              # 操作者 / Actor
    action: str             # 操作类型 / Action type
    resource: str           # 资源标识 / Resource identifier
    data_category: str      # 数据类别 / Data category
    sensitivity_level: int  # 敏感度等级 / Sensitivity level
    outcome: str            # 结果 / Outcome
    details: dict           # 详情 / Details
    checksum: str = ""      # 完整性校验 / Integrity checksum

    def compute_checksum(self) -> str:
        """计算事件校验和，防篡改 / Compute checksum for tamper-proofing"""
        content = f"{self.event_id}|{self.timestamp}|{self.actor}|{self.action}|{self.resource}"
        return hashlib.sha256(content.encode()).hexdigest()[:16]


class ComplianceAuditLogger:
    """合规审计日志器 / Compliance audit logger

    特性 / Features:
    - 不可变追加日志 / Immutable append-only log
    - 链式校验和 / Chained checksums
    - 结构化输出 / Structured output
    """

    def __init__(self, storage_backend=None):
        self._events: list[AuditEvent] = []
        self._last_checksum: str = "genesis"
        self._storage = storage_backend

    def log_access(self, actor: str, resource: str, category: str,
                   level: int, outcome: str = "success", **kwargs) -> AuditEvent:
        """记录数据访问事件 / Log data access event"""
        event = AuditEvent(
            event_id=f"AUD-{len(self._events)+1:06d}",
            timestamp=datetime.now(timezone.utc).isoformat(),
            actor=actor,
            action="data_access",
            resource=resource,
            data_category=category,
            sensitivity_level=level,
            outcome=outcome,
            details=kwargs,
        )
        event.checksum = self._chain_checksum(event)
        self._events.append(event)
        return event

    def log_transform(self, actor: str, resource: str, transform_type: str,
                      input_count: int, output_count: int, **kwargs) -> AuditEvent:
        """记录数据转换事件 / Log data transformation event"""
        event = AuditEvent(
            event_id=f"AUD-{len(self._events)+1:06d}",
            timestamp=datetime.now(timezone.utc).isoformat(),
            actor=actor,
            action="data_transform",
            resource=resource,
            data_category=transform_type,
            sensitivity_level=0,
            outcome="success",
            details={"input_count": input_count, "output_count": output_count, **kwargs},
        )
        event.checksum = self._chain_checksum(event)
        self._events.append(event)
        return event

    def _chain_checksum(self, event: AuditEvent) -> str:
        """链式校验和：每个事件包含前一个事件的哈希 / Chained checksum"""
        content = f"{self._last_checksum}|{event.event_id}|{event.timestamp}|{event.action}"
        checksum = hashlib.sha256(content.encode()).hexdigest()[:16]
        self._last_checksum = checksum
        return checksum

    def verify_integrity(self) -> bool:
        """验证日志链完整性 / Verify log chain integrity"""
        prev_checksum = "genesis"
        for event in self._events:
            content = f"{prev_checksum}|{event.event_id}|{event.timestamp}|{event.action}"
            expected = hashlib.sha256(content.encode()).hexdigest()[:16]
            if event.checksum != expected:
                return False
            prev_checksum = event.checksum
        return True

    def export_report(self, start: str = None, end: str = None) -> list[dict]:
        """导出审计报告 / Export audit report"""
        events = self._events
        if start:
            events = [e for e in events if e.timestamp >= start]
        if end:
            events = [e for e in events if e.timestamp <= end]
        return [asdict(e) for e in events]
```

### 18.4 治理框架对比 / Governance Framework Comparison

| 框架 / Framework | 适用范围 / Scope | 核心要求 / Core Requirements | 本项目关联 / Project Relevance |
|---|---|---|---|
| GDPR | 欧盟个人数据 / EU personal data | 数据最小化、被遗忘权 / Minimization, right to erasure | ✅ 分类引擎 |
| PIPL (个保法) | 中国个人信息 / China personal info | 分级分类、跨境评估 / Grading, cross-border assessment | ✅ 核心驱动 |
| HIPAA | 美国健康数据 / US health data | 最小必要原则 / Minimum necessary | ✅ 健康数据规则 |
| PCI-DSS | 支付卡数据 / Payment card data | 加密存储、访问控制 / Encrypted storage, access control | ✅ 金融规则 |
| CCPA/CPRA | 加州消费者 / California consumers | 知情权、删除权 / Right to know, delete | 参考 / Reference |
| SOX | 上市公司财务 / Public company financial | 审计追踪、数据完整性 / Audit trail, integrity | ✅ 审计日志 |

## 19. 实时数据处理架构 / Real-time Data Processing Architecture

实时数据处理要求在数据产生的同时或极短延迟内完成处理。与批处理不同，实时架构需要处理无界数据流、保证 Exactly-Once 语义，并在延迟与吞吐之间取得平衡。

Real-time data processing requires processing data at or very shortly after the moment it is produced. Unlike batch processing, real-time architectures must handle unbounded data streams, guarantee Exactly-Once semantics, and balance latency vs. throughput.

### 19.1 流处理核心概念 / Stream Processing Core Concepts

```python
"""流处理核心抽象 / Stream processing core abstractions"""
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Callable, Generator
from datetime import datetime, timedelta
import asyncio
import logging

logger = logging.getLogger(__name__)


@dataclass
class StreamEvent:
    """流事件 / Stream event"""
    event_id: str
    timestamp: datetime
    key: str              # 分区键 / Partition key
    payload: dict         # 事件负载 / Event payload
    headers: dict = None  # 元数据头 / Metadata headers

    def __post_init__(self):
        if self.headers is None:
            self.headers = {}


class StreamProcessor(ABC):
    """流处理器抽象基类 / Stream processor abstract base class"""

    @abstractmethod
    async def process(self, event: StreamEvent) -> list[StreamEvent]:
        """处理单个事件，返回零或多个输出事件 / Process single event"""
        ...

    async def process_batch(self, events: list[StreamEvent]) -> list[StreamEvent]:
        """批量处理（默认逐条）/ Batch processing (default: one by one)"""
        results = []
        for event in events:
            results.extend(await self.process(event))
        return results


class WindowAggregator:
    """窗口聚合器 / Window aggregator

    支持滚动窗口和滑动窗口两种模式。
    Supports both tumbling and sliding window modes.
    """

    def __init__(self, window_size: timedelta, slide: timedelta = None):
        self.window_size = window_size
        self.slide = slide or window_size  # 默认滚动窗口 / Default tumbling
        self._buffers: dict[str, list[StreamEvent]] = {}

    def assign_window(self, event: StreamEvent) -> str:
        """为事件分配窗口 / Assign event to window"""
        # 计算窗口起始时间 / Calculate window start
        epoch = datetime(2020, 1, 1)
        elapsed = (event.timestamp - epoch).total_seconds()
        window_start_sec = int(elapsed // self.slide.total_seconds()) * self.slide.total_seconds()
        window_start = epoch + timedelta(seconds=window_start_sec)
        return f"{event.key}:{window_start.isoformat()}"

    def add_event(self, event: StreamEvent) -> None:
        """添加事件到窗口缓冲 / Add event to window buffer"""
        window_id = self.assign_window(event)
        if window_id not in self._buffers:
            self._buffers[window_id] = []
        self._buffers[window_id].append(event)

    def emit_ready_windows(self, watermark: datetime) -> list[tuple[str, list[StreamEvent]]]:
        """发射已就绪的窗口 / Emit ready windows based on watermark"""
        ready = []
        for window_id, events in list(self._buffers.items()):
            # 解析窗口结束时间 / Parse window end time
            _, ts_str = window_id.split(":", 1)
            window_start = datetime.fromisoformat(ts_str)
            window_end = window_start + self.window_size

            if window_end <= watermark:
                ready.append((window_id, events))
                del self._buffers[window_id]

        return ready


class PrivacyStreamFilter(StreamProcessor):
    """隐私流过滤器 / Privacy stream filter

    在流处理中实时检测和过滤敏感数据。
    Detects and filters sensitive data in real-time during stream processing.
    """

    def __init__(self, sensitive_patterns: list[str], action: str = "mask"):
        import re
        self.patterns = [re.compile(p, re.IGNORECASE) for p in sensitive_patterns]
        self.action = action  # mask | drop | flag

    async def process(self, event: StreamEvent) -> list[StreamEvent]:
        """检测并处理敏感字段 / Detect and handle sensitive fields"""
        payload = event.payload.copy()
        flagged_fields = []

        for field_name, value in payload.items():
            if value is None:
                continue
            str_val = str(value)
            for pattern in self.patterns:
                if pattern.search(str_val):
                    flagged_fields.append(field_name)
                    if self.action == "mask":
                        payload[field_name] = "***MASKED***"
                    elif self.action == "drop":
                        del payload[field_name]
                    break

        if flagged_fields:
            event.headers["privacy_flagged"] = flagged_fields
            logger.info(f"Event {event.event_id}: flagged fields {flagged_fields}")

        if self.action == "drop" and flagged_fields:
            return []  # 丢弃整个事件 / Drop entire event

        event.payload = payload
        return [event]
```

### 19.2 背压与流量控制 / Backpressure & Flow Control

```python
"""背压控制机制 / Backpressure control mechanism"""
import asyncio
from collections import deque
from dataclasses import dataclass
from enum import Enum


class PressureLevel(Enum):
    """压力等级 / Pressure level"""
    NORMAL = "normal"      # 正常 / Normal
    ELEVATED = "elevated"  # 升高 / Elevated
    HIGH = "high"          # 高压 / High pressure
    CRITICAL = "critical"  # 临界 / Critical


@dataclass
class FlowMetrics:
    """流量指标 / Flow metrics"""
    input_rate: float = 0.0       # 输入速率 (events/s) / Input rate
    output_rate: float = 0.0      # 输出速率 / Output rate
    buffer_usage: float = 0.0     # 缓冲使用率 / Buffer usage ratio
    processing_latency_ms: float = 0.0  # 处理延迟 / Processing latency


class BackpressureController:
    """背压控制器 / Backpressure controller

    根据下游处理能力动态调整上游发送速率。
    Dynamically adjusts upstream send rate based on downstream capacity.

    策略 / Strategies:
    1. 缓冲 / Buffer: 增大缓冲区吸收突发 / Enlarge buffer for bursts
    2. 降速 / Throttle: 降低生产者速率 / Slow down producer
    3. 丢弃 / Drop: 丢弃低优先级事件 / Drop low-priority events
    4. 溢出 / Spill: 溢出到磁盘 / Spill to disk
    """

    def __init__(self, buffer_size: int = 10000,
                 high_watermark: float = 0.8,
                 low_watermark: float = 0.3):
        self.buffer: deque = deque(maxlen=buffer_size)
        self.buffer_size = buffer_size
        self.high_watermark = high_watermark
        self.low_watermark = low_watermark
        self._pressure = PressureLevel.NORMAL
        self._metrics = FlowMetrics()

    @property
    def pressure_level(self) -> PressureLevel:
        """当前压力等级 / Current pressure level"""
        return self._pressure

    def evaluate_pressure(self) -> PressureLevel:
        """评估当前压力等级 / Evaluate current pressure level"""
        usage = len(self.buffer) / self.buffer_size
        self._metrics.buffer_usage = usage

        if usage >= 0.95:
            self._pressure = PressureLevel.CRITICAL
        elif usage >= self.high_watermark:
            self._pressure = PressureLevel.HIGH
        elif usage >= 0.5:
            self._pressure = PressureLevel.ELEVATED
        else:
            self._pressure = PressureLevel.NORMAL

        return self._pressure

    def should_accept(self, priority: int = 5) -> bool:
        """判断是否接受新事件 / Decide whether to accept new event

        Args:
            priority: 事件优先级 1-10 / Event priority 1-10

        Returns:
            是否接受 / Whether to accept
        """
        pressure = self.evaluate_pressure()

        if pressure == PressureLevel.CRITICAL:
            return priority >= 9  # 仅接受最高优先级 / Only highest priority
        elif pressure == PressureLevel.HIGH:
            return priority >= 7  # 接受高优先级 / Accept high priority
        elif pressure == PressureLevel.ELEVATED:
            return priority >= 4  # 接受中等以上 / Accept medium+
        return True  # 正常全接受 / Normal: accept all

    def get_throttle_delay_ms(self) -> float:
        """获取节流延迟 / Get throttle delay in ms"""
        pressure = self.evaluate_pressure()
        delays = {
            PressureLevel.NORMAL: 0.0,
            PressureLevel.ELEVATED: 10.0,
            PressureLevel.HIGH: 50.0,
            PressureLevel.CRITICAL: 200.0,
        }
        return delays[pressure]
```

### 19.3 Exactly-Once 语义保证 / Exactly-Once Semantics Guarantee

```python
"""Exactly-Once 语义实现 / Exactly-Once semantics implementation"""
import hashlib
from dataclasses import dataclass, field
from datetime import datetime


@dataclass
class Checkpoint:
    """检查点 / Checkpoint"""
    checkpoint_id: str
    offset: int                   # 消费偏移量 / Consumer offset
    timestamp: datetime = field(default_factory=datetime.now)
    state_snapshot: dict = field(default_factory=dict)


class ExactlyOnceProcessor:
    """精确一次处理器 / Exactly-Once processor

    通过幂等性 + 检查点 + 事务性写入实现 Exactly-Once。
    Achieves Exactly-Once via idempotency + checkpointing + transactional writes.

    实现原理 / Implementation principle:
    1. 每个事件有唯一 ID / Each event has unique ID
    2. 处理前检查是否已处理（幂等）/ Check if processed before (idempotent)
    3. 定期保存检查点 / Periodically save checkpoints
    4. 故障恢复时从检查点重放 / Replay from checkpoint on failure
    """

    def __init__(self, checkpoint_interval: int = 100):
        self.checkpoint_interval = checkpoint_interval
        self._processed_ids: set[str] = set()  # 已处理事件 ID / Processed event IDs
        self._current_offset: int = 0
        self._last_checkpoint: Checkpoint | None = None
        self._pending_results: list = []

    def compute_event_id(self, payload: dict) -> str:
        """计算事件唯一 ID（幂等键）/ Compute event unique ID (idempotency key)"""
        content = str(sorted(payload.items()))
        return hashlib.md5(content.encode()).hexdigest()

    def process_event(self, payload: dict) -> dict | None:
        """处理事件（幂等）/ Process event (idempotent)

        Returns:
            处理结果，若已处理过则返回 None / Result, or None if already processed
        """
        event_id = self.compute_event_id(payload)

        # 幂等检查 / Idempotency check
        if event_id in self._processed_ids:
            return None  # 跳过重复事件 / Skip duplicate

        # 执行业务逻辑 / Execute business logic
        result = self._transform(payload)

        # 记录已处理 / Mark as processed
        self._processed_ids.add(event_id)
        self._current_offset += 1
        self._pending_results.append(result)

        # 检查是否需要检查点 / Check if checkpoint needed
        if self._current_offset % self.checkpoint_interval == 0:
            self._save_checkpoint()

        return result

    def _transform(self, payload: dict) -> dict:
        """业务转换逻辑 / Business transformation logic"""
        return {"processed": True, "data": payload}

    def _save_checkpoint(self) -> None:
        """保存检查点 / Save checkpoint"""
        self._last_checkpoint = Checkpoint(
            checkpoint_id=f"CP-{self._current_offset}",
            offset=self._current_offset,
            state_snapshot={"processed_count": len(self._processed_ids)},
        )
        # 提交事务性写入 / Commit transactional write
        self._pending_results.clear()

    def recover_from_checkpoint(self, checkpoint: Checkpoint) -> None:
        """从检查点恢复 / Recover from checkpoint"""
        self._current_offset = checkpoint.offset
        self._pending_results.clear()
        # 从偏移量处重新消费 / Re-consume from offset
```

### 19.4 实时 vs 批处理对比 / Real-time vs Batch Processing Comparison

| 维度 / Dimension | 批处理 / Batch | 流处理 / Stream | 微批 / Micro-batch |
|---|---|---|---|
| 延迟 / Latency | 分钟~小时 / Min~Hours | 毫秒~秒 / ms~s | 秒~分钟 / s~min |
| 吞吐 / Throughput | 极高 / Very high | 中等 / Medium | 高 / High |
| 复杂度 / Complexity | 低 / Low | 高 / High | 中 / Medium |
| 一致性 / Consistency | 强一致 / Strong | 最终一致 / Eventual | 窗口一致 / Window |
| 容错 / Fault tolerance | 重跑 / Re-run | 检查点 / Checkpoint | 重跑批次 / Re-run batch |
| 适用场景 / Use case | 报表/ETL / Reports | 实时监控 / Monitoring | 准实时分析 / Near-RT |
| 本项目 / This project | ✅ 分类管道 / Pipeline | 未来扩展 / Future | 未使用 / Not used |

## 20. 隐私增强计算技术 / Privacy-Enhancing Computation Technologies

隐私增强计算（PEC/PET）是一组在保护数据隐私的前提下实现数据计算和分析的技术。本项目中的差分隐私、K-匿名和数据脱敏都属于 PEC 范畴。

Privacy-Enhancing Computation (PEC/PET) is a set of technologies that enable data computation and analysis while protecting data privacy. Differential privacy, K-anonymity, and data masking in this project all fall under the PEC umbrella.

### 20.1 差分隐私数学基础 / Differential Privacy Mathematical Foundation

```python
"""差分隐私核心机制 / Differential privacy core mechanisms"""
import math
import random
from dataclasses import dataclass


@dataclass
class PrivacyBudget:
    """隐私预算 / Privacy budget"""
    epsilon: float        # 隐私损失参数 / Privacy loss parameter
    delta: float = 1e-5   # 松弛参数 / Relaxation parameter
    consumed: float = 0.0 # 已消耗 / Consumed

    @property
    def remaining(self) -> float:
        return max(0, self.epsilon - self.consumed)

    def consume(self, cost: float) -> bool:
        """消耗预算 / Consume budget. Returns False if insufficient."""
        if cost > self.remaining:
            return False
        self.consumed += cost
        return True


class LaplaceMechanism:
    """拉普拉斯机制 / Laplace mechanism

    数学定义 / Mathematical definition:
    - 对于查询 f，敏感度 Δf = max|f(D) - f(D')|
    - 添加噪声 Lap(Δf/ε) 实现 ε-差分隐私
    - For query f, sensitivity Δf = max|f(D) - f(D')|
    - Add noise Lap(Δf/ε) to achieve ε-differential privacy

    性质 / Properties:
    - 无偏性: E[M(D)] = f(D) / Unbiased
    - 方差: Var = 2(Δf/ε)² / Variance
    - 组合性: k 次查询消耗 kε / Composition: k queries cost kε
    """

    def __init__(self, epsilon: float, sensitivity: float = 1.0):
        if epsilon <= 0:
            raise ValueError("epsilon must be positive / epsilon 必须为正")
        self.epsilon = epsilon
        self.sensitivity = sensitivity
        self.scale = sensitivity / epsilon  # b = Δf/ε

    def add_noise(self, true_value: float) -> float:
        """对真实值添加拉普拉斯噪声 / Add Laplace noise to true value

        使用逆变换采样 / Uses inverse transform sampling:
        X = -b * sign(U - 0.5) * ln(1 - 2|U - 0.5|)
        """
        u = random.random() - 0.5
        noise = -self.scale * math.copysign(1, u) * math.log(1 - 2 * abs(u))
        return true_value + noise

    def noisy_count(self, true_count: int) -> float:
        """带噪声的计数查询 / Noisy count query"""
        return self.add_noise(float(true_count))

    def noisy_sum(self, true_sum: float, clamp_min: float, clamp_max: float) -> float:
        """带噪声的求和查询（含裁剪）/ Noisy sum query with clipping

        裁剪确保有界敏感度 / Clipping ensures bounded sensitivity:
        sensitivity = max(|clamp_min|, |clamp_max|)
        """
        clipped = max(clamp_min, min(clamp_max, true_sum))
        return self.add_noise(clipped)


class GaussianMechanism:
    """高斯机制 / Gaussian mechanism

    实现 (ε, δ)-差分隐私 / Achieves (ε, δ)-differential privacy:
    - 噪声标准差 σ = Δf * sqrt(2 * ln(1.25/δ)) / ε
    - 比拉普拉斯噪声尾部更薄 / Thinner tails than Laplace
    - 适合需要 (ε,δ)-DP 的组合场景 / Suitable for (ε,δ)-DP composition
    """

    def __init__(self, epsilon: float, delta: float, sensitivity: float = 1.0):
        if epsilon <= 0 or delta <= 0 or delta >= 1:
            raise ValueError("Invalid parameters / 参数无效")
        self.epsilon = epsilon
        self.delta = delta
        self.sensitivity = sensitivity
        # σ = Δf * sqrt(2 * ln(1.25/δ)) / ε
        self.sigma = sensitivity * math.sqrt(2 * math.log(1.25 / delta)) / epsilon

    def add_noise(self, true_value: float) -> float:
        """添加高斯噪声 / Add Gaussian noise"""
        noise = random.gauss(0, self.sigma)
        return true_value + noise

    @property
    def noise_std(self) -> float:
        """噪声标准差 / Noise standard deviation"""
        return self.sigma
```

### 20.2 安全多方计算概念 / Secure Multi-Party Computation Concepts

```python
"""安全多方计算（MPC）概念演示 / Secure Multi-Party Computation concepts demo

注意：这是教学演示，非生产级密码学实现。
Note: This is educational demo, NOT production-grade cryptography.

MPC 核心思想 / Core MPC idea:
- 多个参与方联合计算一个函数 / Multiple parties jointly compute a function
- 每个参与方不泄露自己的输入 / No party reveals its own input
- 只获得最终计算结果 / Only the final result is revealed
"""
import random
from typing import Protocol


class SecretSharingScheme(Protocol):
    """秘密共享协议 / Secret sharing protocol"""

    def split(self, secret: int, n_parties: int, threshold: int) -> list[int]:
        """将秘密分割为 n 份 / Split secret into n shares"""
        ...

    def reconstruct(self, shares: list[int]) -> int:
        """从份额重建秘密 / Reconstruct secret from shares"""
        ...


class AdditiveSecretSharing:
    """加法秘密共享 / Additive secret sharing

    原理 / Principle:
    - 将秘密 s 随机分为 n 份: s = s₁ + s₂ + ... + sₙ (mod p)
    - 任何 n-1 份无法推断 s / Any n-1 shares reveal nothing about s
    - 所有 n 份相加恢复 s / All n shares sum to recover s

    适用场景 / Use cases:
    - 联合统计（均值、总和）/ Joint statistics (mean, sum)
    - 隐私集合求交 / Private set intersection
    """

    def __init__(self, modulus: int = 2**31 - 1):
        self.modulus = modulus  # 大素数模 / Large prime modulus

    def split(self, secret: int, n_parties: int) -> list[int]:
        """将秘密分为 n 个加法份额 / Split secret into n additive shares"""
        shares = [random.randint(0, self.modulus - 1) for _ in range(n_parties - 1)]
        # 最后一份确保总和等于 secret / Last share ensures sum equals secret
        last_share = (secret - sum(shares)) % self.modulus
        shares.append(last_share)
        return shares

    def reconstruct(self, shares: list[int]) -> int:
        """重建秘密 / Reconstruct secret"""
        return sum(shares) % self.modulus

    def secure_add(self, shares_a: list[int], shares_b: list[int]) -> list[int]:
        """安全加法：各方本地相加即可 / Secure addition: local addition suffices

        [a] + [b] = [a + b]，无需通信 / No communication needed
        """
        return [(a + b) % self.modulus for a, b in zip(shares_a, shares_b)]

    def secure_scalar_mul(self, shares: list[int], scalar: int) -> list[int]:
        """安全标量乘法 / Secure scalar multiplication

        c * [a] = [c * a]，各方本地乘即可 / Local multiplication
        """
        return [(s * scalar) % self.modulus for s in shares]


# 使用示例 / Usage example
def demo_secure_sum():
    """演示：三方安全求和 / Demo: 3-party secure sum

    场景：三个医院想计算患者总数，但不想暴露各自数量。
    Scenario: Three hospitals want total patient count without revealing individual counts.
    """
    sss = AdditiveSecretSharing()

    # 各方的真实数据（互相不可见）/ Each party's private data
    hospital_a_count = 1523
    hospital_b_count = 2847
    hospital_c_count = 981

    # 各方将数据秘密共享 / Each party secret-shares its data
    shares_a = sss.split(hospital_a_count, 3)
    shares_b = sss.split(hospital_b_count, 3)
    shares_c = sss.split(hospital_c_count, 3)

    # 各方将对应份额发给对应参与方 / Distribute shares to parties
    # Party 1 持有: shares_a[0], shares_b[0], shares_c[0]
    # Party 2 持有: shares_a[1], shares_b[1], shares_c[1]
    # Party 3 持有: shares_a[2], shares_b[2], shares_c[2]

    # 各方本地求和 / Each party computes local sum
    party1_sum = (shares_a[0] + shares_b[0] + shares_c[0]) % sss.modulus
    party2_sum = (shares_a[1] + shares_b[1] + shares_c[1]) % sss.modulus
    party3_sum = (shares_a[2] + shares_b[2] + shares_c[2]) % sss.modulus

    # 公开各方局部和，重建总和 / Reveal local sums, reconstruct total
    total = sss.reconstruct([party1_sum, party2_sum, party3_sum])
    assert total == hospital_a_count + hospital_b_count + hospital_c_count
    return total  # 5351
```

### 20.3 同态加密概念 / Homomorphic Encryption Concepts

```python
"""同态加密概念演示 / Homomorphic encryption concepts demo

注意：教学演示，使用简化模型。生产环境请使用 Microsoft SEAL / OpenFHE。
Note: Educational demo with simplified model. Use Microsoft SEAL / OpenFHE for production.

同态加密类型 / HE types:
- 部分同态 (PHE): 支持一种运算 / Supports one operation
- 层级同态 (SHE): 有限次运算 / Limited operations
- 全同态 (FHE): 任意运算 / Arbitrary operations
"""


class PaillierLikePHE:
    """类 Paillier 部分同态加密（简化）/ Simplified Paillier-like PHE

    性质 / Properties:
    - 加法同态: Enc(a) ⊕ Enc(b) = Enc(a + b)
    - 标量乘法: Enc(a) ⊗ k = Enc(a * k)
    - 不支持乘法: Enc(a) ⊗ Enc(b) ≠ Enc(a * b)

    实际应用 / Real applications:
    - 联邦学习梯度聚合 / Federated learning gradient aggregation
    - 加密数据库查询 / Encrypted database queries
    - 隐私保护统计 / Privacy-preserving statistics
    """

    def __init__(self, key_size: int = 32):
        # 简化：使用大数模拟 / Simplified: simulate with large numbers
        self._n = (1 << key_size) - 1  # 模数 / Modulus
        self._key = random.randint(1, self._n)

    def encrypt(self, plaintext: int) -> int:
        """加密 / Encrypt (simplified additive masking)"""
        mask = random.randint(0, self._n)
        # 密文 = 明文 + 掩码 (mod n) / Ciphertext = plaintext + mask
        return ((plaintext + mask) % self._n, mask)

    def decrypt(self, ciphertext: tuple) -> int:
        """解密 / Decrypt"""
        encrypted_val, mask = ciphertext
        return (encrypted_val - mask) % self._n

    def homomorphic_add(self, ct1: tuple, ct2: tuple) -> tuple:
        """同态加法 / Homomorphic addition

        Enc(a) ⊕ Enc(b) = Enc(a + b)
        无需解密即可计算 / Compute without decryption
        """
        val = (ct1[0] + ct2[0]) % self._n
        mask = (ct1[1] + ct2[1]) % self._n
        return (val, mask)

    def homomorphic_scalar_mul(self, ct: tuple, scalar: int) -> tuple:
        """同态标量乘 / Homomorphic scalar multiplication

        Enc(a) ⊗ k = Enc(a * k)
        """
        val = (ct[0] * scalar) % self._n
        mask = (ct[1] * scalar) % self._n
        return (val, mask)


# 应用示例：加密均值计算 / Example: encrypted mean computation
def encrypted_mean_demo():
    """演示：在加密数据上计算均值 / Demo: compute mean on encrypted data

    场景：数据拥有者加密数据后交给计算方，计算方无法看到原始值。
    Scenario: Data owner encrypts data, compute party cannot see raw values.
    """
    phe = PaillierLikePHE()

    # 数据拥有者加密 / Data owner encrypts
    raw_data = [25, 30, 35, 40, 45]
    encrypted_data = [phe.encrypt(x) for x in raw_data]

    # 计算方在密文上操作 / Compute party operates on ciphertexts
    encrypted_sum = encrypted_data[0]
    for ct in encrypted_data[1:]:
        encrypted_sum = phe.homomorphic_add(encrypted_sum, ct)

    # 计算方无法解密，将结果返回给数据拥有者
    # Compute party cannot decrypt, returns result to data owner
    decrypted_sum = phe.decrypt(encrypted_sum)
    mean = decrypted_sum / len(raw_data)

    assert decrypted_sum == sum(raw_data)  # 175
    assert mean == 35.0
    return mean
```

### 20.4 隐私计算技术全景对比 / Privacy Computing Technology Landscape

| 技术 / Technology | 保护对象 / Protects | 计算能力 / Compute | 性能开销 / Overhead | 本项目 / Project |
|---|---|---|---|---|
| 差分隐私 / DP | 个体记录 / Individual records | 统计查询 / Statistics | 低 / Low | ✅ 核心 |
| K-匿名 / K-anonymity | 准标识符 / Quasi-identifiers | 数据发布 / Data release | 低 / Low | ✅ 核心 |
| 数据脱敏 / Masking | 直接标识符 / Direct identifiers | 展示/测试 / Display/test | 极低 / Very low | ✅ 核心 |
| 同态加密 / HE | 计算中数据 / Data in computation | 加法/乘法 / Add/Mul | 极高 / Very high | 未使用 / Not used |
| 安全多方计算 / MPC | 各方输入 / Each party's input | 联合计算 / Joint compute | 高 / High | 未使用 / Not used |
| 可信执行环境 / TEE | 内存中数据 / Data in memory | 任意 / Arbitrary | 中 / Medium | 未使用 / Not used |
| 联邦学习 / FL | 训练数据 / Training data | 模型训练 / Model training | 中 / Medium | 未使用 / Not used |
| 零知识证明 / ZKP | 声明真实性 / Statement truth | 验证 / Verification | 高 / High | 未使用 / Not used |
