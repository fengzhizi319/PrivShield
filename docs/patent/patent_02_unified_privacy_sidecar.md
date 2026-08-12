# 专利草案 02：隐私治理网关的双协议负载均衡与分布式隐私预算记账方法

## 一、创新点提炼（与已有专利的差异定位）

> 已有中文专利 3 已深入覆盖差分隐私预算记账的注册表工厂、SQLite 共享记账、时间窗口重置、HMAC 审计、RDP 会计等。已有中文专利 4 已覆盖 Sidecar 架构、分类分级驱动原语编排、Arrow 元数据流转等。本专利草案聚焦于**网关层负载均衡、健康检查自愈、REST/gRPC 双协议泛化转发**，以及**网关层与后端 Worker 之间的分布式隐私预算一致性**，与上述专利形成互补。

1. **REST 与 gRPC 双协议统一网关**
   - 网关作为独立组件，同时提供 HTTP 代理（FastAPI）与 gRPC 代理（grpc.aio），将外部流量统一分发到后端多个隐私服务 Worker。
   - 支持 REST 通配符路径 `{path:path}` 与 gRPC 泛化转发：初始化时自动反射 `PrivacyService` 中的所有 RPC 方法并绑定转发函数，新增 gRPC 方法无需手写代理方法。

2. **多策略负载均衡与健康检查自愈**
   - 提供轮询（Round-Robin）、随机选择、最小连接数三种负载均衡策略。
   - 后台健康检查任务默认每 5 秒对后端节点执行 HTTP `GET /health` 与 gRPC `Health` RPC 双重探测；仅当两者均通过才标记为健康。
   - 不健康节点被排除出可用池，恢复后自动重新加入，实现故障隔离与自愈。

3. **事件循环感知与连接复用**
   - 复用应用级全局单例 `httpx.AsyncClient` 与 gRPC channel/stub，减少连接建立开销。
   - 实现事件循环感知机制：当检测到当前 Loop 与缓存客户端绑定的 Loop 不一致时，自动重建 `httpx.AsyncClient`，解决 `Event loop is closed` 错误。

4. **网关层隐私预算一致性保护**
   - 网关将请求路由到多个 Worker 时，确保同一命名空间的隐私预算在多个 Worker 间一致消耗。
   - 通过 `PRIVACY_BUDGET_DB` 启用 SQLite 持久化账本，多 Worker 共享同一预算库，避免各 Worker 独立内存记账导致实际预算消耗为宣称预算 N 倍的问题。
   - 网关层在转发 DP 查询时附加命名空间上下文，后端 Worker 扣减预算后返回剩余预算，网关可聚合或透传预算状态。

5. **错误处理与容错**
   - 无可用节点时返回 HTTP 503 / gRPC UNAVAILABLE。
   - 连接超时/网络波动返回 HTTP 502 Bad Gateway。
   - 后端 gRPC 错误透传 `grpc.RpcError` 状态码与描述。

6. **可观测性埋点**
   - 建议埋点指标：`privacy_gateway_requests_total`、`privacy_gateway_latency_seconds`、`privacy_gateway_healthy_nodes`。
   - 通过网关层统一记录请求来源、目标 Worker、协议类型、状态码、延迟。

---

## 二、专利原文

### 发明名称

一种隐私治理网关的双协议负载均衡与分布式隐私预算记账方法及系统

### 技术领域

本发明涉及网络安全、负载均衡与隐私计算技术领域，尤其涉及隐私计算服务网关的 REST/gRPC 双协议代理、多策略负载均衡、健康检查自愈与分布式隐私预算一致性。

### 背景技术

在隐私计算 Sidecar/微服务架构中，多个 Worker 节点共同对外提供隐私保护服务。现有技术存在以下问题：
1. 缺少同时支持 REST 与 gRPC 的统一网关，业务方需分别接入两套入口。
2. 后端 Worker 故障时缺乏自动健康检查与自愈机制，单点故障影响可用性。
3. gRPC 代理通常需为每个 RPC 方法手写转发函数，proto 新增方法后网关必须同步修改。
4. 多 Worker 部署时，若各 Worker 独立维护内存隐私预算账本，实际累计预算消耗会随 Worker 数量线性放大，破坏差分隐私保证。
5. 事件循环切换或进程 fork 后，异步 HTTP 客户端可能绑定到已关闭的 Loop，导致 `Event loop is closed` 异常。

### 发明内容

本发明提供一种隐私治理网关的双协议负载均衡与分布式隐私预算记账方法及系统，通过统一网关实现 REST/gRPC 双协议代理、多策略负载均衡、健康检查自愈，并通过共享预算账本保证多 Worker 场景下隐私预算消耗的全局一致性。

一种隐私治理网关的双协议负载均衡与分布式隐私预算记账方法，包括：
- 部署隐私治理网关作为统一入口，同时暴露 REST 代理服务与 gRPC 代理服务；
- 维护后端隐私服务 Worker 节点池，每个节点记录 HTTP URL、gRPC 地址、健康状态与当前活跃连接数；
- 后台任务周期性地对每个 Worker 执行 HTTP 健康检查与 gRPC 健康检查，仅当两者均通过时标记为健康；
- 接收到请求后，根据负载均衡策略从健康节点中选择一个目标 Worker；
- REST 代理提取 Method、Headers、Query Params、Body 后转发至目标 Worker 的 HTTP URL；
- gRPC 代理通过反射调用目标 Worker gRPC stub 的对应方法，并将响应原样返回；
- 对差分隐私查询请求，在网关层附加命名空间上下文，后端 Worker 从共享数据库读取并扣减隐私预算，保证多 Worker 间预算消耗一致。

进一步地，所述 REST 代理使用 FastAPI 路径通配符 `{path:path}` 匹配所有路径，并复用应用级全局单例 `httpx.AsyncClient` 进行异步转发。

进一步地，所述 gRPC 代理在初始化时自动反射 `PrivacyService` 中的所有 RPC 方法并绑定转发函数，新增 gRPC 方法无需修改网关代码。

进一步地，还包括事件循环感知机制：检测当前事件循环与缓存 HTTP 客户端绑定的事件循环是否一致；不一致时自动重建 `httpx.AsyncClient`。

进一步地，所述负载均衡策略包括：轮询，维护索引并递增；随机选择；最小连接数，分发前活跃连接数加 1，完成后减 1。

进一步地，所述健康检查包括：HTTP `GET <http_url>/health`，预期 200 且 `status == "ok"`；gRPC `Health` RPC，预期 `status == "ok"`；两者必须同时通过。

进一步地，所述分布式隐私预算记账包括：后端 Worker 配置共享的 SQLite 预算数据库；每次 DP 查询通过 `BEGIN IMMEDIATE` 独占事务读取已消耗预算、校验是否超限、写回新消耗量；超预算时拒绝查询并返回错误。

进一步地，还包括错误处理：无可用节点时返回 HTTP 503 或 gRPC UNAVAILABLE；连接超时或网络波动返回 HTTP 502；后端 gRPC 错误透传状态码与描述。

进一步地，还包括网关层可观测性：记录并暴露请求总数、转发延迟、健康节点数指标。

### 具体实施方式

**系统架构**
```
Client → Gateway(HTTP Proxy + gRPC Proxy + LoadBalancer + HealthChecker)
            ↓
        Worker 1 / Worker 2 / ... / Worker N
            ↓
       Shared SQLite Budget DB (optional)
```

**BackendNode 结构**
```python
class BackendNode:
    http_url: str
    grpc_address: str
    is_healthy: bool
    active_connections: int
    grpc_channel: grpc.aio.Channel
    grpc_stub: PrivacyServiceStub
```

**REST 代理实施例**
```python
@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"])
async def proxy_http(path: str, request: Request):
    node = load_balancer.select_node()
    async with get_async_client() as client:
        response = await client.request(
            method=request.method,
            url=f"{node.http_url}/{path}",
            headers=dict(request.headers),
            params=dict(request.query_params),
            content=await request.body(),
        )
    return Response(response.content, status_code=response.status_code, headers=dict(response.headers))
```

**gRPC 泛化转发实施例**
```python
class PrivacyGatewayServicer(PrivacyServiceServicer):
    async def _forward(self, method_name, request, context):
        node = load_balancer.select_node()
        method = getattr(node.grpc_stub, method_name)
        try:
            return await method(request)
        except grpc.RpcError as e:
            context.abort(e.code(), e.details())

# 初始化时自动绑定所有 RPC 方法
for method_name in rpc_method_names:
    setattr(PrivacyGatewayServicer, method_name, lambda self, request, context, name=method_name: self._forward(name, request, context))
```

**健康检查实施例**
- 每 5 秒对每个 Worker 执行：
  - HTTP: `GET http://worker/health`，检查 200 + status ok。
  - gRPC: `Health.Check`，检查 status ok。
- 仅两者均通过时 `is_healthy = True`。
- 可用池为空时返回 503 / UNAVAILABLE。

**事件循环感知实施例**
```python
_cached_loop = None
_cached_client = None

async def get_async_client():
    global _cached_loop, _cached_client
    current_loop = asyncio.get_running_loop()
    if _cached_client is None or _cached_loop != current_loop:
        _cached_client = httpx.AsyncClient()
        _cached_loop = current_loop
    return _cached_client
```

**分布式预算记账实施例**
- 三个 Worker 共享 `PRIVACY_BUDGET_DB=/data/budget.db`。
- Worker A 成功扣减 ε=1 后，Worker B 读取到最新已消耗量。
- 网关层在转发 DP 请求时附加 `x-privacy-namespace: hr_dataset` header 或 gRPC metadata。
- Worker 扣减预算并返回剩余预算，网关可透传至响应元数据。

### 权利要求书

1. 一种隐私治理网关的双协议负载均衡与分布式隐私预算记账方法，其特征在于，包括：
   部署隐私治理网关作为统一入口，同时暴露 REST 代理服务与 gRPC 代理服务；
   维护后端隐私服务 Worker 节点池，记录各 Worker 的 HTTP URL、gRPC 地址、健康状态与当前活跃连接数；
   周期性对每个 Worker 执行 HTTP 健康检查与 gRPC 健康检查，仅当两者均通过时标记为健康；
   接收到请求后，根据负载均衡策略从健康节点中选择一个目标 Worker；
   REST 代理将请求转发至目标 Worker 的 HTTP URL，gRPC 代理通过反射调用目标 Worker gRPC stub 的对应方法；
   对差分隐私查询请求，后端 Worker 从共享数据库读取并扣减隐私预算，保证多 Worker 间预算消耗一致。

2. 根据权利要求 1 所述的方法，其特征在于，所述 REST 代理使用 FastAPI 路径通配符匹配所有路径，并复用应用级全局单例 `httpx.AsyncClient` 进行异步转发。

3. 根据权利要求 1 所述的方法，其特征在于，所述 gRPC 代理在初始化时自动反射 `PrivacyService` 中的所有 RPC 方法并绑定转发函数，新增 gRPC 方法无需修改网关代码。

4. 根据权利要求 1 所述的方法，其特征在于，还包括事件循环感知机制：检测当前事件循环与缓存 HTTP 客户端绑定的事件循环是否一致；不一致时自动重建 `httpx.AsyncClient`。

5. 根据权利要求 1 所述的方法，其特征在于，所述负载均衡策略包括轮询、随机选择、最小连接数。

6. 根据权利要求 1 所述的方法，其特征在于，所述健康检查包括：HTTP `GET /health` 预期 200 且状态 ok；gRPC `Health` RPC 预期状态 ok；两者必须同时通过。

7. 根据权利要求 1 所述的方法，其特征在于，所述分布式隐私预算记账包括：后端 Worker 配置共享的 SQLite 预算数据库；每次 DP 查询通过独占事务读取已消耗预算、校验是否超限、写回新消耗量；超预算时拒绝查询。

8. 根据权利要求 1 所述的方法，其特征在于，还包括错误处理：无可用节点时返回 HTTP 503 或 gRPC UNAVAILABLE；连接超时返回 HTTP 502；后端 gRPC 错误透传状态码与描述。

9. 根据权利要求 1 所述的方法，其特征在于，还包括网关层可观测性：记录并暴露请求总数、转发延迟、健康节点数指标。

10. 一种隐私治理网关的双协议负载均衡与分布式隐私预算记账系统，其特征在于，包括处理器、存储器及存储在所述存储器上的计算机程序，所述处理器执行所述计算机程序时实现如权利要求 1-9 任一项所述的方法。

### 摘要

本发明公开一种隐私治理网关的双协议负载均衡与分布式隐私预算记账方法及系统。该方法部署同时支持 REST 与 gRPC 的统一网关，通过多策略负载均衡、HTTP/gRPC 双重健康检查实现后端 Worker 故障隔离与自愈；通过 gRPC 泛化转发自动适配新增 RPC 方法；通过事件循环感知机制保证异步客户端正确复用；并通过共享 SQLite 预算数据库保证多 Worker 场景下差分隐私预算消耗的全局一致性。本发明解决了隐私计算服务横向扩展时的可用性、一致性与可维护性问题，适用于 Sidecar、微服务与多副本部署场景。
