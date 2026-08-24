# 统一控制台与 BFF 代理网关 (Console & BFF Gateway)

数盾统一运维与测试控制台，提供现代化的 Web UI 交互界面与高性能的 API 代理网关（BFF），用于直观呈现隐私计算、动态分类分级、数据流通调度及合规审计全链路功能。

---

## 1. 双 BFF 架构设计

为了同时兼顾**极速开发适配**与**高并发生产性能**，数盾设计了双 BFF（Backend For Frontend）架构：

```mermaid
graph TD
    Browser[React + TS 前端控制台<br/>:5173 / :80]

    subgraph DualBFF [双 BFF 网关层]
        GoBFF[Go gRPC BFF<br/>:8081<br/>Gin + gRPC-Go<br/>单连接多路复用 / 生产主力]
        PyBFF[Python REST BFF<br/>:8080<br/>FastAPI + httpx<br/>同构无缝 / 开发调试]
    end

    subgraph CoreEngine [PrivShield 核心算力引擎]
        AgentREST[Agent REST :8079]
        AgentGRPC[Agent gRPC :50051]
    end

    Browser -->|/api/* (切换至 8081)| GoBFF
    Browser -->|/api/* (切换至 8080)| PyBFF

    GoBFF -->|gRPC / HTTP/2| AgentGRPC
    PyBFF -->|HTTP/REST| AgentREST
```

* **Go gRPC BFF (`bff-go:8081`)**：
  * 基于 Gin + gRPC-Go 构建；
  * 使用 Protobuf 结构体进行严格类型约束，通过 HTTP/2 多路复用大幅削减通信握手延迟；
  * 生产环境主力推荐，内置 mTLS 双向认证支持与前端静态文件独立托管。
* **Python REST BFF (`bff-py:8080`)**：
  * 基于 FastAPI + httpx 构建；
  * 与 Agent 核心库完全同构，原生支持 Arrow IPC 二进制流反序列化；
  * 开发调试与极简模式首选。

---

## 2. 前端控制台 (Web UI)

* **技术栈**：React 18 + TypeScript + Vite + TailwindCSS + Lucide Icons；
* **极速热更新**：支持 Vite HMR（<50ms 本地热更新）；
* **功能工作台**：
  * 隐私原语交互面板（脱敏、DP 噪声注入、K-匿名分布、查询混淆对比）；
  * 动态分类分级评估面板（三层漏斗命中轨迹可视化、规则/NER/LLM 决策链呈现）；
  * 数据流通调度流水线大屏；
  * 资产目录与敏感字段探查；
  * 不可篡改审计日志流水与哈希完整性核验工具。

---

## 3. 运行指南

```bash
# 启动 Go BFF + Web 前端
bash ./scripts/dev/dev-start-go.sh

# 启动 Python BFF + Web 前端
bash ./scripts/dev/dev-start.sh

# 启动双后端 + Web 前端 (支持 UI 顶部一键切换后端)
bash ./scripts/dev/dev-start-all.sh

# 停止控制台服务
bash ./scripts/dev/dev-stop.sh
```
