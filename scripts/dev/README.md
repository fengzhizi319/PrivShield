# 本地开发与测试运维脚本 (scripts/dev)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 在本地开发调试、端到端集成测试、Docker 容器联调以及性能压测阶段所需的自动化脚本集合。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [1. 本地原生开发与控制台启动脚本](#1-本地原生开发与控制台启动脚本)
  - [`dev-start-go.sh` / `dev-start-go.ps1` (Go BFF + 前端热更新)](#dev-start-gosh--dev-start-gops1)
  - [`dev-start.sh` (Python BFF + 前端热更新)](#dev-startsh)
  - [`dev-start-all.sh` (双 BFF + 前端热更新)](#dev-start-allsh)
  - [`dev-start-go-mtls.sh` (Go BFF mTLS 安全模式)](#dev-start-go-mtlssh)
  - [`dev-stop.sh` (停止本地开发服务)](#dev-stopsh)
- [2. 中台微服务群管理脚本](#2-中台微服务群管理脚本)
  - [`dev-start-new-modules.sh` (启动 3 大中台微服务)](#dev-start-new-modulessh)
  - [`dev-stop-new-modules.sh` (停止 3 大中台微服务)](#dev-stop-new-modulessh)
  - [`e2e-start-all-services.sh` (启动 Agent + 3 大微服务)](#e2e-start-all-servicessh)
  - [`e2e-stop-all-services.sh` (停止 Agent + 3 大微服务)](#e2e-stop-all-servicessh)
  - [`start_all_services.sh` (启动全量服务群)](#start_all_servicessh)
  - [`stop_all_services.sh` (停止全量服务群)](#stop_all_servicessh)
- [3. Docker 容器化联调脚本](#3-docker-容器化联调脚本)
  - [`docker-start-all.sh` (启动全栈 Docker 容器)](#docker-start-allsh)
  - [`docker-start-go.sh` (启动 Go + Web 容器栈)](#docker-start-gosh)
  - [`docker-start-python.sh` (启动 Python + Web 容器栈)](#docker-start-pythonsh)
  - [`docker-start-agent.sh` / `docker-start-agent.ps1` (启动 Agent 容器)](#docker-start-agentsh--docker-start-agentps1)
  - [`docker-stop-agent.sh` / `docker-stop-agent.ps1` (停止 Agent 容器)](#docker-stop-agentsh--docker-stop-agentps1)
  - [`docker-start-llm.sh` / `docker-start-llm.ps1` (启动 vLLM 容器)](#docker-start-llmsh--docker-start-llmps1)
  - [`docker-stop-llm.sh` / `docker-stop-llm.ps1` (停止 vLLM 容器)](#docker-stop-llmsh--docker-stop-llmps1)
  - [`docker-stop.sh` (停止全部 Docker 容器)](#docker-stopsh)
- [4. 自动化测试、基准压测与环境工具](#4-自动化测试基准压测与环境工具)
  - [`run_console_e2e_tests.sh` (全套 E2E 自动化测试)](#run_console_e2e_testssh)
  - [`integration-test-new-modules.sh` (微服务集成测试)](#integration-test-new-modulessh)
  - [`benchmark_performance.sh` (原语基准性能压测)](#benchmark_performancesh)
  - [`health_check.sh` (健康状态诊断与探针)](#health_checksh)
  - [`check_metrics_endpoints.sh` (Prometheus 指标探针)](#check_metrics_endpointssh)
  - [`start_monitoring.sh` (启动监控栈)](#start_monitoringsh)
  - [`stop_monitoring.sh` (停止监控栈)](#stop_monitoringsh)
  - [`verify_console_environment.sh` (开发环境巡检)](#verify_console_environmentsh)
  - [`clean_privacy_budget_db.sh` (清理隐私预算数据库)](#clean_privacy_budget_dbsh)
  - [`mock_agent_server.py` (Mock Agent 桩服务)](#mock_agent_serverpy)

---

## 1. 本地原生开发与控制台启动脚本

### `dev-start-go.sh` / `dev-start-go.ps1`
- **作用说明**: 【推荐主力】一键启动 Python 核心算力 Agent（REST `:8079`、gRPC `:50051`）、Go 语言 gRPC 代理网关 BFF (`:8081`)，以及基于 Vite 的 React Web 前端开发服务器 (`:5173`，支持毫秒级 HMR 热更新）。
- **参数选项**: `--force`（端口被占用时自动释放占用进程）。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/dev/dev-start-go.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\dev-start-go.ps1
  ```

---

### `dev-start.sh`
- **作用说明**: 一键启动 Python 核心算力 Agent（REST `:8079`、gRPC `:50051`）、Python REST 代理后端 BFF (`:8080`)，以及前端 Vite 开发服务器 (`:5173`)。
- **参数选项**: `--force`（非交互模式自动释放占用端口）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-start.sh
  ```

---

### `dev-start-all.sh`
- **作用说明**: 【双后端联调】同时启动 Python 核心 Agent、Go gRPC BFF (`:8081`)、Python REST BFF (`:8080`) 和 React Web 前端 (`:5173`)，用于对比双后端代理行为一致性。
- **参数选项**: `--force`（自动释放占用端口）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-start-all.sh
  ```

---

### `dev-start-go-mtls.sh`
- **作用说明**: 以严格的 **mTLS 双向 TLS 证书认证** 模式启动 Go BFF 网关 (`:8443`) 和 Python Agent，用于验证零信任网络与安全通道传输。
- **参数选项**: `--force`（自动释放占用端口）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-start-go-mtls.sh
  ```

---

### `dev-stop.sh`
- **作用说明**: 一键优雅停止本地由上述开发脚本启动的所有进程（Agent、Go/Python BFF、Vite 前端），释放相关端口资源。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-stop.sh
  ```

---

## 2. 中台微服务群管理脚本

### `dev-start-new-modules.sh`
- **作用说明**: 启动 PrivShield 的 3 大 Go 语言中台微服务：
  - `service-hub` 数据流通调度中枢 (`:8082`)
  - `datasource-mgr` 数据源资产管理与探查 (`:8083`, gRPC `:50053`)
  - `audit-log` 脱敏审计日志存证 (`:8084`, gRPC `:50054`)
  *(注：该脚本要求核心 Agent 已在运行中)*。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-start-new-modules.sh
  ```

---

### `dev-stop-new-modules.sh`
- **作用说明**: 一键停止 3 大中台微服务进程（`service-hub`、`datasource-mgr`、`audit-log`），释放端口 `:8082`、`:8083`、`:8084`。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-stop-new-modules.sh
  ```

---

### `e2e-start-all-services.sh`
- **作用说明**: 一键联动启动 **核心算力 Agent (`:8079/50051`)** 以及 **3 大 Go 中台微服务 (`:8082`, `:8083`, `:8084`)**，为端到端全链路业务测试提供基础环境。
- **执行命令**:
  ```bash
  bash ./scripts/dev/e2e-start-all-services.sh
  ```

---

### `e2e-stop-all-services.sh`
- **作用说明**: 一键优雅停止由 `e2e-start-all-services.sh` 启动的 Agent 算力层与 3 大中台微服务所有后台进程。
- **执行命令**:
  ```bash
  bash ./scripts/dev/e2e-stop-all-services.sh
  ```

---

### `start_all_services.sh`
- **作用说明**: 本地全量单机进程启动脚本，一键启动 Agent、3 大中台微服务、双 BFF 网关以及前端 UI 控制台。
- **执行命令**:
  ```bash
  bash ./scripts/dev/start_all_services.sh
  ```

---

### `stop_all_services.sh`
- **作用说明**: 停止本地由 `start_all_services.sh` 启动的全量开发服务群，清理 PID 文件并释放所有相关端口。
- **执行命令**:
  ```bash
  bash ./scripts/dev/stop_all_services.sh
  ```

---

## 3. Docker 容器化联调脚本

### `docker-start-all.sh`
- **作用说明**: 通过 Docker Compose 一键启动全栈容器集群（Agent + Python BFF + Go BFF + Nginx 静态前端 Web）。
- **参数选项**:
  - `--with-llm`: 联动启动本地 vLLM 大语言模型推理容器 (`:8000`)。
- **执行命令**:
  ```bash
  # 标准启动全栈容器
  bash ./scripts/dev/docker-start-all.sh

  # 带本地 vLLM 大模型容器联动启动
  bash ./scripts/dev/docker-start-all.sh --with-llm
  ```

---

### `docker-start-go.sh`
- **作用说明**: 通过 Docker Compose 启动由 **Agent + Go BFF 网关 + Web 控制台** 构成的轻量化生产级镜像组合。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-start-go.sh
  ```

---

### `docker-start-python.sh`
- **作用说明**: 通过 Docker Compose 启动由 **Agent + Python BFF 网关 + Web 控制台** 构成的容器化服务组合。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-start-python.sh
  ```

---

### `docker-start-agent.sh` / `docker-start-agent.ps1`
- **作用说明**: 仅启动核心 Agent 容器，暴露 REST 端口 `:8079` 与 gRPC 端口 `:50051`。
- **参数选项**: `core`（轻量纯 CPU 镜像，默认）或 `ml`（含 PyTorch/Transformers/ONNX/TensorRT 的重型镜像）。
- **执行命令**:
  ```bash
  # Linux / macOS (默认 core 镜像)
  bash ./scripts/dev/docker-start-agent.sh core

  # Linux / macOS (ml 镜像)
  bash ./scripts/dev/docker-start-agent.sh ml
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\docker-start-agent.ps1 -Target core
  ```

---

### `docker-stop-agent.sh` / `docker-stop-agent.ps1`
- **作用说明**: 停止并移除单独运行的核心 Agent Docker 容器。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/dev/docker-stop-agent.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\docker-stop-agent.ps1
  ```

---

### `docker-start-llm.sh` / `docker-start-llm.ps1`
- **作用说明**: 启动独立的本地 vLLM 高性能推理容器（基于 Qwen3.5 模型），对外提供 OpenAI 兼容的 `/v1/chat/completions` API 接口 (`:8000`)。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/dev/docker-start-llm.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\docker-start-llm.ps1
  ```

---

### `docker-stop-llm.sh` / `docker-stop-llm.ps1`
- **作用说明**: 停止并清理运行中的 vLLM 本地大模型推理容器。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/dev/docker-stop-llm.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\docker-stop-llm.ps1
  ```

---

### `docker-stop.sh`
- **作用说明**: 一键停止并彻底清理开发模式下运行的所有 Docker 容器实例、虚拟网络及临时卷。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-stop.sh
  ```

---

## 4. 自动化测试、基准压测与环境工具

### `run_console_e2e_tests.sh`
- **作用说明**: 【CI/回归基准】运行控制台全套端到端 (E2E) 自动化测试。自动拉起 Mock Agent 桩服务，依次执行 Python BFF 冒烟测试、Go BFF 与 Pkg 单元测试、Services 微服务群测试以及 Web 前端 Vitest 组件测试，提供全链路 100% 覆盖校验。
- **执行命令**:
  ```bash
  bash ./scripts/dev/run_console_e2e_tests.sh
  ```

---

### `integration-test-new-modules.sh`
- **作用说明**: 使用 `curl` 针对运行中的 3 大中台微服务（调度中枢、数据源探查、审计日志不可篡改存证）发起全流程业务集成测试。
- **执行命令**:
  ```bash
  bash ./scripts/dev/integration-test-new-modules.sh
  ```

---

### `benchmark_performance.sh`
- **作用说明**: 自动化执行本地脱敏、差分隐私（DP/LDP）、K-匿名、查询混淆等核心隐私原语的高并发与极限吞吐基准压测。
- **执行命令**:
  ```bash
  bash ./scripts/dev/benchmark_performance.sh
  ```

---

### `health_check.sh`
- **作用说明**: 本地基础环境与服务健康状态全面巡检工具。探针 Python 3、CUDA / PyTorch 算力架构、Agent REST/gRPC 端口，以及微服务群存活状态。
- **参数选项**:
  - `--all`: 全面探测包括 BFF 与 3 大中台微服务在内的全量组件。
- **执行命令**:
  ```bash
  # 基础 Agent 健康探针
  bash ./scripts/dev/health_check.sh

  # 全量中台微服务群探针
  bash ./scripts/dev/health_check.sh --all
  ```

---

### `check_metrics_endpoints.sh`
- **作用说明**: 批量扫描并校验 Agent、Go BFF、Service Hub、Datasource Mgr、Audit Log 等所有微服务的 Prometheus `/metrics` 端点是否正常输出指标数据。
- **执行命令**:
  ```bash
  bash ./scripts/dev/check_metrics_endpoints.sh
  ```

---

### `start_monitoring.sh`
- **作用说明**: 启动本地 Prometheus (`:9090`) 与 Grafana (`:3000`) 监控可视化容器栈，自动加载仪表盘。
- **执行命令**:
  ```bash
  bash ./scripts/dev/start_monitoring.sh
  ```

---

### `stop_monitoring.sh`
- **作用说明**: 停止本地 Prometheus 与 Grafana 监控容器栈。
- **执行命令**:
  ```bash
  bash ./scripts/dev/stop_monitoring.sh
  ```

---

### `verify_console_environment.sh`
- **作用说明**: 巡检本地开发工具链依赖是否就绪（Python 3.10+、Node.js、pnpm、Go 1.22+、Web TypeScript 编译检查）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/verify_console_environment.sh
  ```

---

### `clean_privacy_budget_db.sh`
- **作用说明**: 差分隐私持久化预算 SQLite 数据库运维管理工具。支持查询当前各 Namespace 预算消耗、重置指定 Namespace 或清空压缩数据库。
- **参数选项**:
  - `--info-only`: 仅查询并输出各 Namespace 预算消耗，不执行清空。
  - `--reset-all`: 清空并压缩重置所有预算。
  - `-n, --namespace <NAME>`: 指定重置特定命名空间。
- **执行命令**:
  ```bash
  # 仅查看预算消耗情况
  bash ./scripts/dev/clean_privacy_budget_db.sh --info-only

  # 重置所有预算
  bash ./scripts/dev/clean_privacy_budget_db.sh --reset-all
  ```

---

### `mock_agent_server.py`
- **作用说明**: 轻量级 Python Mock Agent 桩服务（监听 `:8079`）。无需加载真实 AI 模型或重型依赖即可响应脱敏、分类、诊断等 REST/JSON 报文，供前端和代理网关快速单元测试与联调。
- **执行命令**:
  ```bash
  python3 ./scripts/dev/mock_agent_server.py --port 8079
  ```
