# 高并发与负载均衡实现说明

## 1. 请求路径

生产流量应使用 `engine.gateway`。Gateway 负责节点选择、健康检查、熔断和协议转发；`console/backend` 与 `console/backend-go` 的 `/api/lb_test` 仅是测试探测器，不代表生产 Gateway 的调度结果。

```text
Client -> Gateway -> healthy Agent node
                    ├─ REST
                    └─ gRPC
```

## 2. 调度与故障隔离

- `round_robin`：在健康节点间轮询。
- `weighted_round_robin`：按权重平滑轮询。
- `random` / `weighted_random`：随机选择。
- `least_connections`：根据节点当前活动请求数选择，而不是累计命中数。
- 熔断器的状态变更受锁保护；节点连接数必须通过 `track_connection()` 管理，异常、取消和超时都会释放计数。
- 主动健康检查和业务请求失败都会影响节点可用性；生产环境应通过指标观察摘除/恢复频率，避免把短暂网络抖动误认为容量不足。

## 3. 重试安全边界

Gateway 只对 `GET`、`HEAD`、`OPTIONS` 自动故障转移，最多 3 次。`POST`、`PUT`、`PATCH`、`DELETE` 默认只发送一次，因为请求可能已经完成隐私预算扣减、写入或文件处理，响应丢失后重试会产生重复副作用。

如业务确实需要非幂等请求重试，应在业务层提供幂等键和去重存储，而不是简单提高 Gateway 重试次数。

## 4. Console 压测指标

- 并发压测使用固定数量 worker 消费任务，不会为每个请求预先创建 coroutine。
- `concurrency` 是同时运行的请求上限，`total_requests` 是本轮总请求数。
- `qps` 定义为 `total_requests / 总耗时`，表示总完成吞吐量；成功率单独由 `success / total` 表示。
- LB 测试的 `least_connections` 在请求运行期间按活动探测数动态选择；轮询和随机策略则按其策略生成调度序列。

## 5. 安全配置

Gateway 动态拓扑接口默认兼容本地开发，但生产必须设置管理密钥：

```bash
GATEWAY_API_KEY=<strong-secret>
```

设置后，注册和注销节点必须携带：

```text
Authorization: Bearer <strong-secret>
```

Console 的 `/api/lb_test` 应配置 `LB_ALLOWED_HOSTS`，不要在公网暴露任意目标地址探测能力。Go Console 同样使用 `LB_ALLOWED_HOSTS`，未配置白名单时仅适合可信本地网络。

## 6. 部署与验证建议

高并发上线前至少验证：

1. 节点故障期间幂等 GET 可切换，非幂等 POST 不重复执行。
2. 客户端取消请求后节点活动连接数回到 0。
3. 健康检查恢复不会覆盖仍处于熔断/冷却状态的节点。
4. Go Console 启用 Agent API Key 后，健康检查、代理、批量和上传调用均能通过认证。
5. 多副本部署时使用共享预算存储，并显式配置副本数、资源、连接上限和优雅终止时间。

建议结合 Prometheus 的 Gateway 请求数、重试数、健康节点数和延迟分位数观察容量，而不要只看前端压测页面的平均延迟。

