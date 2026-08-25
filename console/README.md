# 数盾控制台 (PrivShield Console & BFF)

数盾统一运维与测试控制台，提供现代化的 Web UI 交互界面与高性能的 API 代理网关（BFF），用于直观呈现隐私计算、动态分类分级、数据流通调度及合规审计全链路功能。

---

## 1. 架构与目录结构

在全平台解耦架构重构后，中台核心微服务已提升至顶层 [services/](file:///home/charles/code/sfwork/PrivShield/services/)，控制台目录聚焦于 **Web 前端交互** 与 **BFF 代理网关** 职责：

```text
console/
├── web/                  # React + TypeScript + Vite 前端控制台 (UI: :5173 / :80)
├── bff-go/               # Go gRPC/HTTPS API 代理网关 / 主力 BFF (API: :8081)
│   ├── cmd/server/       # 网关启动入口 (支持 HTTPS, gRPC, mTLS 双向认证)
│   ├── internal/         # gRPC 转换、模型映射、文件解析与压测
│   └── docs/             # Go BFF 专属设计与接口文档
├── docs/                 # 控制台技术架构、模式与学习指南
└── README.md             # 控制台总览（本文档）
```

> 💡 **中台微服务索引**：企业级数据流通调度微服务群位于根目录 [services/](file:///home/charles/code/sfwork/PrivShield/services/)：
> - [services/service-hub/](file:///home/charles/code/sfwork/PrivShield/services/service-hub/)：数据服务调度中枢（`:8082`）
> - [services/datasource-mgr/](file:///home/charles/code/sfwork/PrivShield/services/datasource-mgr/)：数据源与资产管理（`:8083`）
> - [services/audit-log/](file:///home/charles/code/sfwork/PrivShield/services/audit-log/)：合规存证与审计日志（`:8084`）
> - 共享基础库提升至根目录 [pkg/](file:///home/charles/code/sfwork/PrivShield/pkg/)，根目录统一通过 [go.work](file:///home/charles/code/sfwork/PrivShield/go.work) 管理。

---

## 2. 文档索引

- [docs/modes.md](file:///home/charles/code/sfwork/PrivShield/console/docs/modes.md) — 开发模式 vs 生产模式部署与网络拓扑总览
- [docs/learning/vite.md](file:///home/charles/code/sfwork/PrivShield/docs/learning/vite.md) — 前端 Vite 热重载与构建原理
- **Go BFF 文档**：[design](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/design.md) · [api](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/api.md) · [test](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/test.md) · [ops](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/ops.md) · [reliability](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/reliability.md)
- **调度微服务文档**：[service-hub docs](file:///home/charles/code/sfwork/PrivShield/services/service-hub/docs/design.md) · [datasource-mgr docs](file:///home/charles/code/sfwork/PrivShield/services/datasource-mgr/docs/design.md) · [audit-log docs](file:///home/charles/code/sfwork/PrivShield/services/audit-log/docs/design.md)
- **可靠性能力文档**：[engine](file:///home/charles/code/sfwork/PrivShield/docs/reliability.md) · [service-hub](file:///home/charles/code/sfwork/PrivShield/services/service-hub/docs/reliability.md) · [audit-log](file:///home/charles/code/sfwork/PrivShield/services/audit-log/docs/reliability.md) · [datasource-mgr](file:///home/charles/code/sfwork/PrivShield/services/datasource-mgr/docs/reliability.md) · [gateway](file:///home/charles/code/sfwork/PrivShield/docs/gateway_balancer/reliability.md) · [bff-go](file:///home/charles/code/sfwork/PrivShield/console/bff-go/docs/reliability.md)
- **脚本手册**：[scripts/dev/](file:///home/charles/code/sfwork/PrivShield/scripts/dev) · [scripts/prod/](file:///home/charles/code/sfwork/PrivShield/scripts/prod)

---

## 3. 快速启动指南

### 3.1 一键启动（开发模式）

在仓库根目录下执行：

```bash
# 启动 PrivShield Agent + Go BFF + Web 前端 (Vite HMR: http://localhost:5173)
bash ./scripts/dev/dev-start-go.sh

# 或仅启动 PrivShield Agent + Go BFF(:8081) + Web 前端（与 dev-start-go.sh 等价）
bash ./scripts/dev/dev-start-go.sh

# 停止开发服务
bash ./scripts/dev/dev-stop.sh
```

### 3.2 联动启动中台微服务群

```bash
# 一键启动 Agent + 三大中台微服务群 (service-hub, datasource-mgr, audit-log)
bash ./scripts/dev/e2e-start-all-services.sh

# 或单独启动三大微服务 (需 Agent 已运行)
bash ./scripts/dev/dev-start-new-modules.sh

# 停止微服务群
bash ./scripts/dev/dev-stop-new-modules.sh
```

### 3.3 Docker 容器化启动

```bash
# 启动 Agent + Go BFF + React Web UI
bash ./scripts/dev/docker-start-go.sh

# 启动全栈容器套件（Agent + Go BFF + Web UI + 可选 vLLM）
bash ./scripts/dev/docker-start-all.sh [--with-llm]

# 停止并清理容器服务
bash ./scripts/dev/docker-stop.sh
```

---

## 4. 自动化测试与质量保障

```bash
# 1. 运行全套端到端 E2E 自动化测试（Mock Agent + Go BFF + Services + Web 前端）
bash ./scripts/dev/run_console_e2e_tests.sh

# 2. 运行 Go 全量测试（Pkg + 微服务群 + Go BFF）
make test-go

# 3. 运行 Web 前端 Vitest 测试
cd console/web && corepack pnpm test -- --run

# 4. 真实全链路 E2E 调度测试（需先启动真实服务）
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/
```

---

## 5. 功能矩阵与端点支持

| 功能模块 | 描述 | BFF-Go (gRPC / HTTPS) | 生产就绪度 |
|---|---|:---:|:---:|
| **Masking** | 字段级、整行记录、批量及结构化 DataFrame 敏感信息脱敏 | ✅ | 生产就绪 |
| **DP / 差分隐私** | 噪声统计（Count/Sum/Mean/Histogram）、向量与自适应裁剪 | ✅ | 生产就绪 |
| **K-Anonymity** | Mondrian 算法数据集泛化与单记录启发式 K-匿名 | ✅ | 生产就绪 |
| **LDP / 本地差分隐私** | 二值/类别特征客户端扰动与中心聚合估计 | ✅ | 生产就绪 |
| **动态分类分级** | 三层漏斗（规则 ➔ Small-NER ➔ 本地 LLM 智能裁决） | ✅ | 生产就绪 |
| **查询混淆 (QOL)** | 差分查询与虚假查询注入 | ✅ | 生产就绪 |
| **医疗/医保治理** | 医疗敏感病历与医保结算多阶段治理流水线 | ✅ | 生产就绪 |
| **双向 mTLS 认证** | 支持入站 HTTPS/gRPC 与出站 Agent 的零信任 mTLS | ✅ | 生产就绪 |
| **中台调度流水线** | 数据服务编排调度（ingest→classify→desensitize→audit） | ✅ (`services`) | 生产就绪 |
| **资产与敏感特征发现**| 自动化探测数据源敏感字段并绑定安全级别 | ✅ (`services`) | 生产就绪 |
| **合规与存证审计** | 不可篡改 SHA-256 审计追踪与合规报告导出 | ✅ (`services`) | 生产就绪 |
