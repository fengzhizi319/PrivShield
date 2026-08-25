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
  - [`prod-bff-agent.sh` (Agent + Go BFF 生产模式，支持 mTLS)](#prod-bff-agentsh)
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
- **作用说明**: 启动生产加固的独立 PrivShield Agent 容器，挂载数据卷与安全配置。
- **执行命令**:
  ```bash
  bash ./scripts/prod/docker-start-agent.sh core
  ```

---

### `docker-stop-agent.sh` / `docker-stop-agent.ps1`
- **作用说明**: 停止生产环境的 PrivShield Agent 容器。
- **执行命令**:
  ```bash
  bash ./scripts/prod/docker-stop-agent.sh
  ```

---

## 2. 本地单机生产模式 (Native Process Production)

### `prod-bff-agent.sh`
- **作用说明**: 在本地或单机以生产模式启动 Agent 算力层与 Go BFF 服务（Go BFF 直接托管已编译打包的 Web 控制台前端静态资源，端口 `:8081`）。
- **参数选项**:
  - `--rebuild`: 启动前强制重新构建 Web 静态文件与 Go 二进制。
  - `--force`: 端口被占用时自动释放占用进程。
  - `--mtls`: 启用 mTLS 双向认证模式。
- **执行命令**:
  ```bash
  # 标准生产单机模式
  bash ./scripts/prod/prod-bff-agent.sh

  # 强制重新构建并以 mTLS 安全模式启动
  bash ./scripts/prod/prod-bff-agent.sh --rebuild --mtls
  ```

---

### `prod-stop.sh`
- **作用说明**: 优雅停止本地单机生产模式下的 Agent 与 Go BFF 进程。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod-stop.sh
  ```

---

## 3. 数据备份与生产巡检 (Backup & Health Check)

### `prod_health_check.sh`
- **作用说明**: 全链路生产健康巡检脚本，检查 Agent、BFF、微服务群、数据库与 TLS 连通性。
- **执行命令**:
  ```bash
  bash ./scripts/prod/prod_health_check.sh
  ```

---

### `backup-sqlite-databases.sh`
- **作用说明**: 全量 SQLite 数据库冷热备份脚本，生成带时间戳与 SHA-256 校验和的 tar.gz 备份包。
- **执行命令**:
  ```bash
  bash ./scripts/prod/backup-sqlite-databases.sh
  ```

---

### `backup_privacy_budget.sh`
- **作用说明**: 专项备份隐私预算（Privacy Budget）SQLite 数据库。
- **执行命令**:
  ```bash
  bash ./scripts/prod/backup_privacy_budget.sh
  ```
