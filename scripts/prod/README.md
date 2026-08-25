# 生产部署与运维脚本 (scripts/prod)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 在生产环境（Production）下的容器编排、Kubernetes / Helm 部署、mTLS 证书安全模式启动、全量 SQLite 数据库冷热备份以及生产级健康巡检脚本。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [1. 容器与集群部署发布 (Docker / K8s / Helm)](#1-容器与集群部署发布-docker--k8s--helm)
  - [`deploy-docker-compose.sh` / `deploy-docker-compose.ps1` (Docker Compose 生产部署)](#deploy-docker-composesh--deploy-docker-composeps1)
  - [`stop-docker-compose.sh` / `stop-docker-compose.ps1` (停止 Docker Compose)](#stop-docker-composesh--stop-docker-composeps1)
  - [`deploy-helm.sh` (Kubernetes Helm 发布)](#deploy-helmsh)
  - [`uninstall-helm.sh` (卸载 Helm 发布)](#uninstall-helmsh)
  - [`deploy-k8s.sh` (原生 K8s 部署)](#deploy-k8ssh)
  - [`stop-k8s.sh` (卸载原生 K8s 资源)](#stop-k8ssh)
  - [`docker-start-agent.sh` / `docker-start-agent.ps1` (生产 Agent 容器启动)](#docker-start-agentsh--docker-start-agentps1)
  - [`docker-stop-agent.sh` / `docker-stop-agent.ps1` (生产 Agent 容器停止)](#docker-stop-agentsh--docker-stop-agentps1)
- [2. 本地单机生产模式 (Native Process Production)](#2-本地单机生产模式-native-process-production)
  - [`prod-start.sh` / `prod-start-go.sh` (Go BFF 生产模式)](#prod-startsh--prod-start-gosh)
  - [`prod-start-go-mtls.sh` (Go BFF 生产 mTLS 模式)](#prod-start-go-mtlssh)
  - [`prod-start-all.sh` (全量服务生产模式)](#prod-start-allsh)
  - [`prod-stop.sh` (停止生产单机服务)](#prod-stopsh)
- [3. 数据备份与生产巡检 (Backup & Health Check)](#3-数据备份与生产巡检-backup--health-check)
  - [`prod_health_check.sh` (生产全链路健康状态巡检)](#prod_health_checksh)
  - [`backup-sqlite-databases.sh` (全量 SQLite 数据库备份与存证)](#backup-sqlite-databasessh)
  - [`backup_privacy_budget.sh` (隐私预算库专项备份)](#backup_privacy_budgetsh)

---

## 1. 容器与集群部署发布 (Docker / K8s / Helm)

### `deploy-docker-compose.sh` / `deploy-docker-compose.ps1`
- **作用说明**: 使用生产级 Docker Compose 配置启动全栈容器服务（含 Agent 算力层、中台微服务群、BFF 网关及 Nginx 静态托管前端）。
- **参数选项**:
  - `--with-llm`: 联动部署本地 vLLM 大模型推理服务容器 (`:8000`)。
  - `--with-monitoring`: 联动部署 Prometheus 与 Grafana 监控容器。
  - `--agent-only`: 仅部署核心 PrivShield Agent 容器。
- **执行命令**:
  ```bash
  # Linux / macOS: 生产标准全栈部署
  bash ./scripts/prod/deploy-docker-compose.sh

  # Linux / macOS: 带本地大模型与监控部署
  bash ./scripts/prod/deploy-docker-compose.sh --with-llm --with-monitoring
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\prod\deploy-docker-compose.ps1 -WithLLM -WithMonitoring
  ```

---

### `stop-docker-compose.sh` / `stop-docker-compose.ps1`
- **作用说明**: 优雅停止并销毁由生产 Docker Compose 启动的全部容器服务。
- **参数选项**:
  - `--volumes`: 同时清理生产挂载的数据卷。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/prod/stop-docker-compose.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\prod\stop-docker-compose.ps1
  ```

---

### `deploy-helm.sh`
- **作用说明**: 将 PrivShield 部署或升级到 Kubernetes 集群中（基于 `deploy/helm/PrivShield` Chart）。
- **参数选项**: 支持透传所有 `helm upgrade --install` 参数（如 `-f values.yaml`、`--set` 等）。
- **执行命令**:
  ```bash
  bash ./scripts/prod/deploy-helm.sh -f deploy/helm/PrivShield/values-production.yaml
  ```

---

### `uninstall-helm.sh`
- **作用说明**: 从 Kubernetes 集群中安全卸载并清理 PrivShield Helm Release。
- **执行命令**:
  ```bash
  bash ./scripts/prod/uninstall-helm.sh
  ```

---

### `deploy-k8s.sh`
- **作用说明**: 使用原生 Kubernetes 资源清单（基于 Kustomize）发布生产集群服务。
- **执行命令**:
  ```bash
  bash ./scripts/prod/deploy-k8s.sh
  ```

---

### `stop-k8s.sh`
- **作用说明**: 卸载并删除由 `deploy-k8s.sh` 创建的原生 Kubernetes 资源清单。
- **执行命令**:
  ```bash
  bash ./scripts/prod/stop-k8s.sh
  ```

---

### `docker-start-agent.sh` / `docker-start-agent.ps1`
- **作用说明**: 生产环境下单独启动 PrivShield 核心 Agent 容器，映射 REST 端口 `:8079` 与 gRPC 端口 `:50051`。
- **参数选项**: `core`（轻量 CPU 镜像，默认）或 `ml`（包含大模型与深度学习库的镜像）。
- **执行命令**:
  ```bash
  # Linux / macOS (默认 core 镜像)
  bash ./scripts/prod/docker-start-agent.sh core

  # Linux / macOS (ml 镜像)
  bash ./scripts/prod/docker-start-agent.sh ml
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\prod\docker-start-agent.ps1 -Target core
  ```

---

### `docker-stop-agent.sh` / `docker-stop-agent.ps1`
- **作用说明**: 停止并清理生产环境单独运行的 PrivShield Agent 容器。
- **执行命令**:
  ```bash
  # Linux / macOS (Bash)
  bash ./scripts/prod/docker-stop-agent.sh
  ```
  ```powershell
  # Windows (PowerShell)
  .\scripts\prod\docker-stop-agent.ps1
  ```

---

## 2. 本地单机生产模式 (Native Process Production)

### `prod-start.sh` / `prod-start-go.sh`
- **作用说明**: 生产单机模式启动 Agent (`:8079/50051`) 与 Go BFF 网关 (`:8081`)。Go BFF 会自动构建前端 Web 静态产物并独立提供生产 SPA 托管服务。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod-start.sh
  ```

---

### `prod-start-go-mtls.sh`
- **作用说明**: 生产单机模式以严格的 **mTLS 双向 TLS 证书认证** 启动 Go BFF 网关 (`:8443`) 与 Agent，验证生产零信任证书体系。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod-start-go-mtls.sh
  ```

---

### `prod-start-all.sh`
- **作用说明**: 生产单机模式启动全量服务（Agent + Go BFF 静态托管）。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod-start-all.sh
  ```

---

### `prod-stop.sh`
- **作用说明**: 一键优雅停止本地生产模式下的所有服务进程，释放端口资源。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod-stop.sh
  ```

---

## 3. 数据备份与生产巡检 (Backup & Health Check)

### `prod_health_check.sh`
- **作用说明**: 生产级全链路健康状态自动化巡检工具。全面探针核心 Agent、Go BFF、中台微服务群连通性、TLS 证书有效性及持久化存储健康度。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod_health_check.sh
  ```

---

### `backup-sqlite-databases.sh`
- **作用说明**: 全量业务 SQLite 数据库冷热备份脚本。自动扫描并备份隐私预算库、数据源探查库与审计存证库，为每个备份文件计算 SHA-256 校验和防篡改存证，输出到 `backups/sqlite_YYYYMMDD_HHMMSS/`。
- **执行命令**:
  ```bash
  bash ./scripts/prod/backup-sqlite-databases.sh
  ```

---

### `backup_privacy_budget.sh`
- **作用说明**: 差分隐私持久化预算 SQLite 数据库专项热备份工具，执行 `VACUUM INTO` 并生成数据完整性校验摘要。
- **执行命令**:
  ```bash
  bash ./scripts/prod/backup_privacy_budget.sh
  ```
