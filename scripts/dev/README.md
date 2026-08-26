# 本地开发与测试运维脚本 (scripts/dev)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 在本地开发调试、端到端集成测试、Docker 容器联调以及性能压测阶段所需的自动化脚本集合。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [1. 本地原生开发与控制台启动脚本](#1-本地原生开发与控制台启动脚本)
  - [`dev-bff-agent.sh` / `dev-bff-agent.ps1` (Agent + Go BFF + 前端热更新)](#dev-bff-agentsh--dev-bff-agentps1)
  - [`dev-app-lz.sh` (调度之眼 App-LZ: Go BFF + 前端热更新)](#dev-app-lzsh)
  - [`dev-stop.sh` (停止本地开发服务)](#dev-stopsh)
  - [`stop-app-lz.sh` (停止 App-LZ 控制台服务)](#stop-app-lzsh)
- [2. 中台微服务群管理脚本](#2-中台微服务群管理脚本)
  - [`dev-start-new-modules.sh` (启动 3 大中台微服务)](#dev-start-new-modulessh)
  - [`dev-stop-new-modules.sh` (停止 3 大中台微服务)](#dev-stop-new-modulessh)
  - [`e2e-start-all-services.sh` (启动 Agent + 3 大微服务)](#e2e-start-all-servicessh)
  - [`e2e-stop-all-services.sh` (停止 Agent + 3 大微服务)](#e2e-stop-all-servicessh)
  - [`start_all_services.sh` (启动全量服务群)](#start_all_servicessh)
  - [`stop_all_services.sh` (停止全量服务群)](#stop_all_servicessh)
- [3. Docker 容器化联调脚本](#3-docker-容器化联调脚本)
  - [`docker-start-bff-agent.sh` / `docker-start-bff-agent.ps1` (控制台三件套容器版)](#docker-start-bff-agentsh--docker-start-bff-agentps1)
  - [`docker-start-app-lz.sh` (调度之眼 App-LZ 全栈容器版)](#docker-start-app-lzsh)
  - [`docker-stop-app-lz.sh` (停止 App-LZ 容器集群)](#docker-stop-app-lzsh)
  - [`docker-start-all.sh` (启动全栈 Docker 容器)](#docker-start-allsh)
  - [`docker-start-agent.sh` / `docker-start-agent.ps1` (启动 Agent 容器)](#docker-start-agentsh--docker-start-agentps1)
  - [`docker-stop-agent.sh` / `docker-stop-agent.ps1` (停止 Agent 容器)](#docker-stop-agentsh--docker-stop-agentps1)
  - [`docker-start-llm.sh` / `docker-start-llm.ps1` (启动 vLLM 容器)](#docker-start-llmsh--docker-start-llmps1)
  - [`docker-stop-llm.sh` / `docker-stop-llm.ps1` (停止 vLLM 容器)](#docker-stop-llmsh--docker-stop-llmps1)
  - [`docker-stop.sh` (停止全部 Docker 容器)](#docker-stopsh)
  - [`start-postgres.sh` (独立启动 Phase B PostgreSQL)](#start-postgressh)
- [4. 自动化测试、基准压测与环境工具](#4-自动化测试基准压测与环境工具)
  - [`run_console_e2e_tests.sh` (全套 E2E 自动化测试)](#run_console_e2e_testssh)
  - [`integration-test-new-modules.sh` (微服务集成测试)](#integration-test-new-modulessh)
  - [`benchmark_performance.sh` (原语基准性能压测)](#benchmark_performancesh)
  - [`health_check.sh` (健康状态诊断与探针)](#health_checksh)
  - [`check_metrics_endpoints.sh` (Prometheus 指标探针)](#check_metrics_endpointssh)
  - [`start_monitoring.sh` (启动监控栈)](#start_monitoringsh)
  - [`stop_monitoring.sh` (停止监控栈)](#stop_monitoringsh)
  - [`verify_console_environment.sh` (开发环境巡检)](#verify_console_environmentsh)
  - [`generate_all_test_certs.sh` (一键生成全量 mTLS 测试证书链)](#generate_all_test_certssh)
  - [`clean_privacy_budget_db.sh` (清理隐私预算数据库)](#clean_privacy_budget_dbsh)
  - [`mock_agent_server.py` (Mock Agent 桩服务)](#mock_agent_serverpy)

---

## 1. 本地原生开发与控制台启动脚本

### `dev-bff-agent.sh` / `dev-bff-agent.ps1`
- **作用说明**: 【推荐主力】一键启动 Python 核心算力 Agent（REST `:8079`、gRPC `:50051`）、Go 语言 gRPC/HTTPS 代理网关 BFF (`:8081`)，以及基于 Vite 的 React Web 前端开发服务器 (`:5173`，支持毫秒级 HMR 热更新）。同时支持 `--mtls` 参数以 mTLS 双向认证模式启动。
- **参数选项**:
  - `--force`: 端口被占用时自动释放占用进程。
  - `--mtls`: 启用 mTLS 双向认证模式（自动生成/挂载自签名证书）。
- **执行命令**:
  ```bash
  # Linux / macOS (标准开发模式)
  bash ./scripts/dev/dev-bff-agent.sh
  ```
  
  ```bash
  # Linux / macOS (mTLS 安全模式)
  bash ./scripts/dev/dev-bff-agent.sh --mtls
  ```
  
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\dev-bff-agent.ps1
  ```

---

### `dev-app-lz.sh`
- **作用说明**: 【调度之眼 · 全景测试工作台】一键启动专用于 `services/service-hub` 深度测试与观测的 `console/app-lz` 前后端控制台：
  - App-LZ Go BFF 聚合代理后端（REST `:8085`）
  - App-LZ React Web 前端开发服务器（`:5174`，支持毫秒级 HMR 热更新）
  脚本自动打通 4 大核心服务（`service-hub` `:8082`、`datasource-mgr` `:8083`、`audit-log` `:8084`、`engine` `:8079`），提供 6 阶段流水线动态流转大屏、TS-01~TS-07 一键自动化测试套件、数据源切片探查与 Phase B PostgreSQL 原子租约争抢看板。
- **参数选项**:
  - `--force`: 端口被占用时自动释放占用进程。
- **执行命令**:
  ```bash
  # 启动 App-LZ 开发控制台 (BFF :8085 + Vite :5174)
  bash ./scripts/dev/dev-app-lz.sh --force
  ```
  ```bash
  # 启动 App-LZ 开发控制台 (BFF :8085 + Vite :5174) mtls
  bash ./scripts/dev/dev-app-lz.sh  --mtls --force
  ```
---

### `dev-stop.sh`
- **作用说明**: 一键优雅停止本地由 `dev-bff-agent.sh` 启动的所有进程（Agent、Go BFF、Vite 前端），释放相关端口资源。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-stop.sh
  ```

---

### `stop-app-lz.sh`
- **作用说明**: 一键优雅停止由 `dev-app-lz.sh` 或 `prod-app-lz.sh` 启动的 App-LZ 控制台进程（Go BFF `:8085` 与 Web 前端 `:5174`），清理 PID 文件并释放端口。
- **执行命令**:
  ```bash
  bash ./scripts/dev/stop-app-lz.sh
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
- **作用说明**: 停止由 `dev-start-new-modules.sh` 启动的 3 大微服务进程。
- **执行命令**:
  ```bash
  bash ./scripts/dev/dev-stop-new-modules.sh
  ```

---

### `e2e-start-all-services.sh`
- **作用说明**: 【真实全量环境】一键顺序启动 Python Agent + 3 大 Go 中台微服务，构建真实 E2E 运行环境。
- **执行命令**:
  ```bash
  bash ./scripts/dev/e2e-start-all-services.sh
  ```

---

### `e2e-stop-all-services.sh`
- **作用说明**: 停止由 `e2e-start-all-services.sh` 启动的所有真实服务进程。
- **执行命令**:
  ```bash
  bash ./scripts/dev/e2e-stop-all-services.sh
  ```

---

### `start_all_services.sh`
- **作用说明**: 一键后台启动核心 Agent、Go BFF 以及可选的中台微服务群（支持 `--with-services`）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/start_all_services.sh --with-services
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

### `docker-start-bff-agent.sh` / `docker-start-bff-agent.ps1`
- **作用说明**: 【推荐 Docker 开发】通过 Docker Compose 启动控制台三件套核心容器（`PrivShield` 隐私 Agent + `privacy-console-backend-go` Go BFF 网关 + `privacy-console-web` Nginx 前端）。脚本自动预编译宿主机产物加速构建，并提供**标准非 mTLS**与 **mTLS 双向认证**两个版本，**REST 与 gRPC 双协议均获得完整支持**。
- **模式与协议说明**:
  1. **标准非 mTLS 版本（默认模式 / Standard Non-mTLS）**：
     - **REST 支持**：Agent REST 端点 `http://localhost:8079`（明文 HTTP）；Go BFF 代理接口 `http://localhost:8081`（明文 HTTP）；React Web `http://localhost:5173`。
     - **gRPC 支持**：Agent 监听明文 gRPC `localhost:50051`；Go BFF 通过明文 gRPC (`PrivShield:50051`) 代理通信。
  2. **mTLS 双向安全认证版本 (`--mtls` / Mutual TLS Mode)**：
     - **REST 支持**：Agent REST 端点升级为 HTTPS `https://localhost:8079`（TLS 强加密）；Go BFF 通过 HTTPS 代理上游；React Web `http://localhost:5173`。
     - **gRPC 支持**：Agent 开启 mTLS 双向证书鉴权（端口 `:50051`），严格校验客户端 CN 白名单（`privshield-client` / `privacy-console-go-client`）；Go BFF 自动挂载客户端证书私钥（`/certs/client.crt`）完成安全握手。
     - 证书若缺失将自动调用 `console/bff-go/scripts/gen-certs.sh` 生成自签名根 CA 与带 Docker 容器名 SAN 的证书链。
- **参数选项**:
  - `--mtls`: 以 mTLS 双向认证模式启动（开启 REST HTTPS + gRPC mTLS）。
  - `--no-mtls`: 以标准明文模式启动（默认）。
  - `--no-build`: 跳过构建直接运行已有本地镜像。
  - `--build`: 启动前重新构建本地镜像（默认行为）。
  - `--force`: 端口被占用时自动释放占用进程。
- **执行命令**:
  ```bash
  # 1. 启动标准非 mTLS 版本 (默认，HTTP + 明文 gRPC)
  bash ./scripts/dev/docker-start-bff-agent.sh --force
  ```

  ```bash

  # 2. 启动 mTLS 双向认证版本 (HTTPS + mTLS gRPC)
  bash ./scripts/dev/docker-start-bff-agent.sh --mtls --force
  ```

  ```bash
  # 3. 跳过构建快速拉起
  bash ./scripts/dev/docker-start-bff-agent.sh --mtls --no-build
  ```

  ```powershell
  # Windows (PowerShell 标准非 mTLS)
  .\scripts\dev\docker-start-bff-agent.ps1
  ```
  
  ```powershell
  # Windows (PowerShell mTLS 双向认证)
  .\scripts\dev\docker-start-bff-agent.ps1 -MTLS
  ```

---

### `docker-start-app-lz.sh`
- **作用说明**: 【调度之眼 · Docker 全栈环境】通过 Docker Compose（`deploy/docker-compose/docker-compose.app-lz.yml`）一键拉起 App-LZ 调度之眼专属容器测试集群：
  - `privshield-app-lz-web`: Nginx 托管的 React 前端控制台大屏（`:5174`）
  - `privshield-app-lz-bff`: Go 语言聚合代理后端（`:8085`，gRPC `:50055`）
  - `privshield-service-hub`: 数据流通调度中枢（`:8082`，gRPC `:50052`）
  - `privshield-datasource-mgr`: 数据源资产探查（`:8083`，gRPC `:50053`）
  - `privshield-audit-log`: 脱敏审计日志存证（`:8084`，gRPC `:50054`）
  - `PrivShield`: 核心隐私与动态分类引擎（`:8079`，gRPC `:50051`）
  支持预编译宿主机产物，秒级启动完整的 4 微服务网格与端到端测试链路。
- **参数选项**:
  - `--build`: 启动前重新构建镜像（默认）。
  - `--no-build`: 使用本地已有镜像快速拉起。
  - `--force`: 自动清理占用端口的非容器进程。
- **执行命令**:
  ```bash
  # 构建并启动 App-LZ 全栈容器测试集群
  bash ./scripts/dev/docker-start-app-lz.sh --force
  ```
  
  ```bash
  # 跳过构建快速启动
  bash ./scripts/dev/docker-start-app-lz.sh --no-build
  ```

---

### `docker-stop-app-lz.sh`
- **作用说明**: 一键停止并销毁由 `docker-start-app-lz.sh` 启动的 App-LZ 容器集群及 Docker 网络。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-stop-app-lz.sh
  ```

---

### `docker-start-all.sh`
- **作用说明**: 通过 Docker Compose 一键启动全栈容器集群（Agent + 3 大 Go 中台微服务 + Go BFF + Web 前端）。
- **参数选项**:
  - `--with-llm`: 联动启动本地 vLLM 大语言模型推理容器 (`:8000`)。
  - `--with-postgres`: 启动 Phase B PostgreSQL 多副本 Hub 模式。
  - `--with-monitoring`: 启动 Prometheus + Grafana 监控栈。
  - `--no-build`: 跳过构建直接运行。
- **执行命令**:
  ```bash
  # 标准启动全栈容器
  bash ./scripts/dev/docker-start-all.sh
  ```
  ```bash
  # 带本地 vLLM 大模型容器联动启动
  bash ./scripts/dev/docker-start-all.sh --with-llm
  ```
  
  ```bash

  # 全量启动 (LLM + PostgreSQL + 监控)
  bash ./scripts/dev/docker-start-all.sh --with-llm --with-postgres --with-monitoring
  ```

---

### `docker-start-agent.sh` / `docker-start-agent.ps1`
- **作用说明**: 仅启动核心 Agent 容器，暴露 REST 端口 `:8079` 与 gRPC 端口 `:50051`。
- **参数选项**: `core`（轻量纯 CPU 镜像，默认）或 `ml`（含 PyTorch/Transformers/ONNX/TensorRT 的重型镜像）。
- **执行命令**:
  ```bash
  # Linux / macOS (默认 core 镜像)
  bash ./scripts/dev/docker-start-agent.sh core
  ```
  
  ```bash 

  # Linux / macOS (ml 镜像)
  bash ./scripts/dev/docker-start-agent.sh ml
  ```
  
  ```powershell
  # Windows (PowerShell)
  .\scripts\dev\docker-start-agent.ps1 -Target core
  ```

---

### `docker-stop-agent.sh` / `docker-stop-agent.ps1`
- **作用说明**: 停止由 `docker-start-agent.sh` 启动的 Agent 容器。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-stop-agent.sh
  ```

---

### `docker-start-llm.sh` / `docker-start-llm.ps1`
- **作用说明**: 启动专用的 vLLM 本地大模型推理容器 (`:8000`)，需宿主机具备 NVIDIA GPU 与 Container Toolkit。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-start-llm.sh
  ```

---

### `docker-stop-llm.sh` / `docker-stop-llm.ps1`
- **作用说明**: 停止由 `docker-start-llm.sh` 启动的 vLLM 容器。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-stop-llm.sh
  ```

---

### `docker-stop.sh`
- **作用说明**: 一键停止并清理所有通过 Docker Compose 启动的开发容器及网络（含 llm/monitoring/phase-b 全部 profile）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/docker-stop.sh
  ```

---

### `start-postgres.sh`
- **作用说明**: 独立启动一个 PostgreSQL 16 Docker 容器，供 Phase B LeasedTaskStore 开发调试。支持 `--stop` 停止并移除容器。
- **参数选项**:
  - `--stop`: 停止并移除 PostgreSQL 容器。
- **环境变量**:
  - `PG_PORT`: 宿主机映射端口 (默认: 5432)。
  - `PG_PASSWORD`: 数据库密码 (默认: privshield_dev)。
- **执行命令**:
  ```bash
  # 启动 PostgreSQL
  bash ./scripts/dev/start-postgres.sh
  ```
  
  ```bash

  # 停止并移除
  bash ./scripts/dev/start-postgres.sh --stop
  ```

---

## 4. 自动化测试、基准压测与环境工具

### `run_console_e2e_tests.sh`
- **作用说明**: 自动化启动 Mock Agent + Go BFF + Vite 前端，并执行端到端自动化测试与连通性验证。
- **执行命令**:
  ```bash
  bash ./scripts/dev/run_console_e2e_tests.sh
  ```

---

### `integration-test-new-modules.sh`
- **作用说明**: 对 `service-hub`、`datasource-mgr` 与 `audit-log` 三大微服务执行全流程接口与数据流测试。
- **执行命令**:
  ```bash
  bash ./scripts/dev/integration-test-new-modules.sh
  ```

---

### `benchmark_performance.sh`
- **作用说明**: 对核心脱敏算法、差分隐私加噪、K-Anonymity 等原语进行 CPU/内存吞吐与基准压测。
- **执行命令**:
  ```bash
  bash ./scripts/dev/benchmark_performance.sh
  ```

---

### `health_check.sh`
- **作用说明**: 对所有开发环境微服务（Agent、Go BFF、三大中台服务）进行健康探针巡检。
- **执行命令**:
  ```bash
  bash ./scripts/dev/health_check.sh
  ```

---

### `check_metrics_endpoints.sh`
- **作用说明**: 检查各服务的 `/metrics` Prometheus 指标暴露端点连通性。
- **执行命令**:
  ```bash
  bash ./scripts/dev/check_metrics_endpoints.sh
  ```

---

### `start_monitoring.sh` / `stop_monitoring.sh`
- **作用说明**: 启动/停止 Prometheus (`:9090`) 与 Grafana (`:3000`) 监控大屏。
- **执行命令**:
  ```bash
  bash ./scripts/dev/start_monitoring.sh
  ```

  ```bash
  bash ./scripts/dev/stop_monitoring.sh
  ```

---

### `verify_console_environment.sh`
- **作用说明**: 检查本地 Go、Node.js、Python、pnpm、端口占用等依赖环境完整性。
- **执行命令**:
  ```bash
  bash ./scripts/dev/verify_console_environment.sh
  ```

---

### `generate_all_test_certs.sh`
- **作用说明**: 重新生成全套 mTLS 开发测试证书链（CA、Server、Client 证书及私钥）。
- **执行命令**:
  ```bash
  bash ./scripts/dev/generate_all_test_certs.sh
  ```

---

### `clean_privacy_budget_db.sh`
- **作用说明**: 重置并清理开发阶段生成的 SQLite 隐私预算消费数据库。
- **执行命令**:
  ```bash
  bash ./scripts/dev/clean_privacy_budget_db.sh
  ```

---

### `mock_agent_server.py`
- **作用说明**: 提供轻量级的 Python Mock Agent 服务，用于无 Python ML 依赖环境下的 Go BFF 与前端快速联调。
- **执行命令**:
  ```bash
  python scripts/dev/mock_agent_server.py
  ```
