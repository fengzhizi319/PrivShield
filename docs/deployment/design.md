# 部署设计文档


## 目录 (Table of Contents)

- [1. 概述](#1-概述)
- [2. 设计目标](#2-设计目标)
- [3. 架构选型](#3-架构选型)
  - [3.1 K8s 适用场景与需求说明](#31-k8s-适用场景与需求说明)
- [4. Helm Chart 结构](#4-helm-chart-结构)
  - [4.1 Helm 介绍](#41-helm-介绍)
  - [4.2 关键 values 说明](#42-关键-values-说明)
  - [4.3 Deployment 环境变量](#43-deployment-环境变量)
  - [4.4 values.yaml 与 env 文件的关系](#44-valuesyaml-与-env-文件的关系)
  - [4.5 探针配置](#45-探针配置)
- [5. core/ml 镜像分层](#5-coreml-镜像分层)
- [6. LLM 推理服务部署设计](#6-llm-推理服务部署设计)
  - [6.1 集成模式：进程内本地推理](#61-集成模式进程内本地推理)
  - [6.2 解耦模式：外部独立 vLLM 服务](#62-解耦模式外部独立-vllm-服务)
  - [6.3 并发与资源护栏](#63-并发与资源护栏)
- [7. 安全设计](#7-安全设计)
- [8. 可观测性设计](#8-可观测性设计)
- [9. 部署流程](#9-部署流程)
- [10. 测试策略](#10-测试策略)
- [11. 滚动更新与回滚策略 / Rolling Update & Rollback Strategy](#11-滚动更新与回滚策略-rolling-update-rollback-strategy)
  - [11.1 滚动更新策略](#111-滚动更新策略)
  - [11.2 回滚方案 / Rollback Plan](#112-回滚方案-rollback-plan)
  - [11.3 蓝绿部署（可选）/ Blue-Green Deployment (Optional)](#113-蓝绿部署可选-blue-green-deployment-optional)
  - [11.4 金丝雀发布（可选）/ Canary Release (Optional)](#114-金丝雀发布可选-canary-release-optional)
- [12. 工业化评分 / Industrialization Scorecard](#12-工业化评分-industrialization-scorecard)
  - [12.1 加权评分表](#121-加权评分表)
  - [12.2 结论](#122-结论)
  - [12.3 亮点](#123-亮点)
  - [12.4 改进建议](#124-改进建议)
- [13. 容器化部署实战排坑与经验总结 / Deployment Pitfalls & Best Practices](#13-容器化部署实战排坑与经验总结--deployment-pitfalls--best-practices)
  - [13.1 本地与私有镜像拉取策略（pull_policy: build）](#131-本地与私有镜像拉取策略pull_policy-build)
  - [13.2 容器隔离构建引擎与宿主机 Daemon 代理冲突（Buildx 驱动选择）](#132-容器隔离构建引擎与宿主机-daemon-代理冲突buildx-驱动选择)
  - [13.3 基础镜像 Tag 锁定与语义规范](#133-基础镜像-tag-锁定与语义规范)
  - [13.4 多语言全链路容器构建加速体系（Debian / Alpine / Go / NPM）](#134-多语言全链路容器构建加速体系debian--alpine--go--npm)
  - [13.5 Go 现代化工具链动态下载与版本协同（GOTOOLCHAIN=auto）](#135-go-现代化工具链动态下载与版本协同gotoolchainauto)
  - [13.6 非 Root 容器安全运行与命名卷挂载权限冲突](#136-非-root-容器安全运行与命名卷挂载权限冲突)
  - [13.7 跨组件探针与健康检查端点一致性（/health 与 /api/health）](#137-跨组件探针与健康检查端点一致性health-与-apihealth)
  - [13.8 容器网络隔离下的服务发现与协议回退寻址](#138-容器网络隔离下的服务发现与协议回退寻址)
  - [13.9 测试与 CI 执行环境中的 Linux ARG_MAX 限制规避](#139-测试与-ci-执行环境中的-linux-arg_max-限制规避)
  - [13.10 运行时业务规范文档与 .dockerignore 排除冲突（generate_profile）](#1310-运行时业务规范文档与-dockerignore-排除冲突generate_profile)

---

## 1. 概述

本文档定义 `PrivShield` 的部署架构、交付形式与配置管理策略。通过 Helm Chart、原生 K8s manifests 与 Docker Compose 三种形式，覆盖从本地联调到 Kubernetes 生产部署的完整场景。

## 2. 设计目标

- 提供可配置的 Helm Chart。
- 提供原生 K8s 最小可运行 manifests。
- 提供 Docker Compose 本地多服务编排示例。
- 支持 core/ml 两种镜像选择。
- 敏感配置通过 Secret 注入，不硬编码到镜像。

## 3. 架构选型

| 组件 | 选型 | 说明 |
|---|---|---|
| 容器编排 | Kubernetes / Helm | 生产环境标准方案 |
| 本地开发 | Docker Compose | 快速拉起 worker + gateway |
| 镜像 | single Dockerfile multi-target | `--target core` / `--target ml` |
| 配置 | ConfigMap + Secret | 非敏感配置用 ConfigMap，证书/密钥用 Secret |
| 入口 | ClusterIP Service + Ingress | REST 通过 Ingress 暴露，gRPC 通过 Service 内部调用 |
| 弹性 | HPA v2 | 基于 CPU/内存横向扩展 |
| 隔离 | NetworkPolicy | 可选，限制仅指定 label 的 Pod 可访问 |

### 3.1 K8s 适用场景与需求说明

本项目的 K8s 资产（Helm Chart `deploy/helm/PrivShield/`、原生 manifests `deploy/k8s/`）为**生产级部署形态**，非实验性方案（CI 已有 `helm-lint` job：`ct lint` + kind 集群 `ct install` 验证）。以下场景明确需要 K8s：

#### 3.1.1 多副本高可用生产部署（核心场景）

生产 values（`values-production.yaml`）已启用 `replicaCount: 2`、HPA `minReplicas: 2`、滚动更新 `maxUnavailable: 0`、PDB 等能力，Docker Compose 无法提供：

- **故障自愈**：Pod 崩溃/被杀后自动重建，`/health`（liveness）与 `/readyz`（readiness，额外校验配置解析器与隐私预算 DB 连通性）探针驱动摘流与重启；
- **滚动发布零停机**：`strategy.maxUnavailable: 0 + maxSurge` 配合 PodDisruptionBudget 保证最小可用副本；
- **多实例隐私预算一致性**：`PRIVACY_BUDGET_DB`（SQLite 分布式预算）正是为多实例设计——多副本运行时预算需跨实例共享，单副本/单机场景无此需求。

#### 3.1.2 弹性扩缩容（流量波动）

- HPA v2：CPU 70% / 内存 80% 阈值触发，`minReplicas 2 → maxReplicas 10` 自动横向扩展；
- 项目具备高并发与网关负载均衡设计（`engine/gateway/`、`docs/high_concurrency/`），脱敏/分类请求量不恒定，LLM 层慢请求占用资源，需要按负载弹性伸缩。

#### 3.1.3 生产安全加固

- TLS 证书、API Key 经 **Secret** 注入（`security.tls.existingSecret` / `security.auth.apiKeysSecret`），可独立轮换，优于 Compose 文件挂载；
- **NetworkPolicy**：生产 values 已启用，仅允许 `app.kubernetes.io/part-of: PrivShield` 的 Pod 访问；
- PodSecurityContext：`runAsNonRoot`、`drop: ALL` capabilities、只读根文件系统；
- mTLS CN 白名单 ConfigMap 挂载（`mtls-whitelist.yaml`），支持 per-CN scope 与热重载。

#### 3.1.4 可观测性接入

- `serviceMonitor.enabled: true` 时 Prometheus 自动发现抓取 `/metrics`（`deploy/prometheus/` + `deploy/grafana/` 联动告警）；
- 日志输出 stdout/stderr，由集群日志系统（EFK/Loki）统一采集。

#### 3.1.5 服务暴露与集群内集成

- REST 经 **Ingress**（TLS 终结）暴露给外部调用方（医院/数据局/业务平台）；
- gRPC 走 ClusterIP Service 供集群内调用方访问（如 console-backend-go 代理）；
- 与 vLLM 推理服务同集群部署时，`PRIVACY_LLM_API_BASE` 指向集群 DNS 而非 `127.0.0.1`（`config/env/vllm.env` 的本地默认值不适用）。

#### 3.1.6 不需要 K8s 的场景（对照）

| 场景 | 部署形态 | 说明 |
|---|---|---|
| 本地开发 / 联调 | 本地直跑 | `python -m engine.server`，`.env` + `config/env/<profile>.env` 级联加载 |
| 内部演示 / 单机小规模 | Docker 单容器 | `docker build --target core|ml` + `docker run`，环境变量注入 |
| 本地全栈（agent + console + vLLM + 监控） | Docker Compose | `deploy/docker-compose/docker-compose.yml` 一键拉起，`env_file: ../../.env` 注入配置 |

**决策建议**：单机、演示、开发 → Compose / 直跑；对外提供脱敏服务、需要多副本高可用 + 弹性伸缩 + 安全合规（TLS/网络隔离/审计）→ K8s，使用 `helm install -f values-production.yaml`（配合自管 TLS/API Key Secret）。

> **注意**：K8s 部署时镜像内无 `.env`（`.dockerignore` 排除），配置完全由 values 控制（见 §4.4），LLM 等无 values 字段的配置须经 `extraEnv` 注入——这是生产配置受控的强制要求。

## 4. Helm Chart 结构
Helm 是 Kubernetes 生态中事实标准的**包管理器**，常被称为「Kubernetes 的 apt/yum」。它把部署在 K8s 上的一组相关资源（Deployment、Service、ConfigMap、Ingress 等）打包成一个可复用、可配置、可版本化的单元，大幅简化了复杂应用在容器集群中的安装、升级与回滚。

---
### 4.1 Helm介绍
#### 一、核心概念

| 概念 | 说明 |
|------|------|
| **Chart** | Helm 的包格式，本质是一个包含模板 YAML 文件的目录或压缩包（`.tgz`）。一个 Chart 描述了一组 Kubernetes 资源。 |
| **Release** | Chart 的一个运行实例。同一个 Chart 可以被多次安装到集群中，每次安装产生一个独立的 Release（如 `my-app-v1`、`my-app-v2`）。 |
| **Repository** | Chart 的远程或本地仓库，用于分发和共享 Chart（如官方仓库 Artifact Hub、企业私有仓库）。 |
| **Values** | 用户提供的配置文件（`values.yaml`），用于在部署时向 Chart 模板注入自定义参数（镜像版本、副本数、域名等）。 |

---

#### 二、Helm 解决了什么问题

1. **模板化配置**：不用手写几十行重复的 YAML，通过 Go template 语法将可变部分参数化。
2. **一键部署**：一条命令 `helm install` 即可创建全部 K8s 资源。
3. **版本管理**：每次 `helm upgrade` 都会产生一个新的 Release 版本，支持 `helm rollback` 秒级回滚。
4. **依赖管理**：Chart 可以声明依赖其他 Chart（如你的应用依赖 MySQL、Redis），Helm 会自动拉取并协调安装顺序。
5. **生态共享**：社区提供了大量成熟的 Chart（Nginx、Prometheus、GitLab 等），可直接复用。

---

#### 三、典型工作流

```bash
# 1. 添加仓库
helm repo add bitnami https://charts.bitnami.com/bitnami

# 2. 搜索 Chart
helm search repo bitnami/nginx

# 3. 安装（创建 Release）
helm install my-nginx bitnami/nginx --set replicaCount=3

# 4. 查看运行状态
helm list
helm status my-nginx

# 5. 升级（修改配置或镜像版本）
helm upgrade my-nginx bitnami/nginx --set image.tag=1.25

# 6. 回滚到上一版本
helm rollback my-nginx

# 7. 卸载（清理所有相关 K8s 资源）
helm uninstall my-nginx
```

---

#### 四、Chart 的内部结构

一个标准 Chart 目录如下：

```
my-chart/
├── Chart.yaml          # Chart 元数据（名称、版本、依赖等）
├── values.yaml         # 默认配置值
├── templates/          # K8s 资源模板（使用 Go template 语法）
│   ├── deployment.yaml
│   ├── service.yaml
│   └── ingress.yaml
├── charts/             # 依赖的子 Chart
└── README.md
```

模板示例（`templates/deployment.yaml`）：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-app
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app: {{ .Chart.Name }}
  template:
    metadata:
      labels:
        app: {{ .Chart.Name }}
    spec:
      containers:
        - name: app
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
```

部署时通过 `values.yaml` 或 `--set` 注入实际值，模板渲染后生成标准 Kubernetes YAML 提交给 API Server。

---

#### 五、Helm 3 的关键改进

- **移除 Tiller**：Helm 2 需要在集群中运行一个服务端组件 Tiller，存在权限过大和安全隐患；Helm 3 改为纯客户端架构，直接通过 kubeconfig 与 API Server 交互。
- **Release 信息存储在 Secret 中**：不再依赖 ConfigMap，默认以 Secret 形式存储在 Release 所在的命名空间，实现更好的隔离和 RBAC 控制。
- **库 Chart 支持**：引入 `library` 类型的 Chart，专门用于封装可复用的模板片段（类似编程语言中的函数库）。

---

#### 六、适用场景

| 场景 | 说明 |
|------|------|
| **标准化交付** | 将内部微服务打包成 Chart，通过 CI/CD 自动部署到测试/生产环境。 |
| **多环境管理** | 用不同的 `values-*.yaml`（`values-dev.yaml`、`values-prod.yaml`）管理同一套 Chart 在不同环境的差异。 |
| **第三方软件部署** | 快速部署 Prometheus、Grafana、Kafka 等复杂中间件，无需手动编写数百行资源清单。 |
| **GitOps 工作流** | 结合 ArgoCD、Flux 等工具，将 Helm Chart 作为声明式配置源，实现自动化同步与漂移检测。 |

---

#### 七、与 Kubectl/Kustomize 的对比

| 工具 | 定位 | 核心能力 |
|------|------|----------|
| **kubectl** | K8s 原生 CLI | 直接操作资源对象，适合临时调试和简单管理 |
| **Kustomize** | 原生配置定制 | 通过 overlay 机制对基础 YAML 做环境差异补丁，无模板引擎 |
| **Helm** | 包管理器 | 模板化、版本化、依赖管理、生态共享，适合复杂应用的生命周期管理 |

三者并不互斥，实际生产环境中常常**Helm 负责打包与版本管理，Kustomize 负责环境补丁，kubectl 负责运维操作**。

---

如果你正在学习或准备使用 Helm，建议从 [Artifact Hub](https://artifacthub.io/) 上找一个感兴趣的官方 Chart（如 Nginx、PostgreSQL），尝试 `helm install` 和自定义 `values.yaml`，这是上手最快的方式。
```text
deploy/helm/PrivShield/
├── Chart.yaml
├── values.yaml
├── values-production.yaml
├── values-ml.yaml
└── templates/
    ├── _helpers.tpl
    ├── namespace.yaml
    ├── serviceaccount.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── configmap.yaml
    ├── secret.yaml
    ├── ingress.yaml
    ├── hpa.yaml
    ├── poddisruptionbudget.yaml
    └── networkpolicy.yaml
```

### 4.2 关键 values 说明

```yaml
image:
  repository: PrivShield
  tag: ""  # 默认使用 Chart.appVersion
  pullPolicy: IfNotPresent

flavor: core  # core | ml

service:
  type: ClusterIP
  restPort: 8079
  grpcPort: 50051

security:
  tls:
    enabled: false
    existingSecret: ""
  auth:
    enabled: false
    apiKeysSecret: ""
  rateLimit:
    enabled: false

resources:
  requests:
    cpu: 100m
    memory: 256Mi

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80
```

### 4.3 Deployment 环境变量

| 环境变量 | 来源 | 说明 |
|---|---|---|
| `PRIVACY_PROFILE` | ConfigMap 挂载 | `/etc/PrivShield/privacy-profile.yaml` |
| `PRIVACY_LOG_LEVEL` | values | `INFO` / `DEBUG` |
| `PRIVACY_LOG_FORMAT` | values | `json` / `text` |
| `PRIVACY_TLS_ENABLED` | values | `true` / `false` |
| `PRIVACY_TLS_CERT_FILE` | Secret 挂载 | `/certs/tls.crt` |
| `PRIVACY_TLS_KEY_FILE` | Secret 挂载 | `/certs/tls.key` |
| `PRIVACY_TLS_CA_FILE` | Secret 挂载 | `/certs/ca.crt`（mTLS 模式） |
| `PRIVACY_AUTH_ENABLED` | values | `true` / `false` |
| `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` | Secret | 外部 API Key JSON 字符串 |
| `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED` | values | 内部 mTLS 免 Key 认证 |
| `PRIVACY_RATE_LIMIT_ENABLED` | values | `true` / `false` |

### 4.4 values.yaml 与 env 文件的关系

本项目的配置存在**两类入口**：Helm `values.yaml`（K8s 部署）与 env 文件（根目录 `.env` + `config/env/<profile>.env`，本地开发）。二者不是替代关系，而是同一批 `PRIVACY_*` 环境变量在不同运行环境的投递方式。

#### 一、两类配置入口

| 入口 | 位置 | 加载机制 | 生效场景 |
|---|---|---|---|
| Helm values | `deploy/helm/PrivShield/values.yaml`（另有 `values-production.yaml` / `values-ml.yaml`） | Helm 模板渲染为 Pod 环境变量、ConfigMap、Secret 引用与卷挂载（`templates/deployment.yaml`、`configmap.yaml`、`secret.yaml`） | K8s 部署唯一声明式配置入口 |
| 根目录 `.env` | 项目根目录 `.env`（模板见 `.env.example`） | `engine/env_loader.py` 的 `load_env_file()` 在进程启动时加载（`main.py` / `grpc_server.py` / `server.py` 模块级执行），默认 `override=False` 不覆盖已有环境变量 | 本地直跑（`python -m engine.server` 等） |
| 场景 profile env | `config/env/<profile>.env`（`vllm` / `qwen3` / `mlx` / `openai`） | 按 `PRIVACY_ENV_PROFILE`（默认 `vllm`）级联加载，`override=True` 覆盖基础值；仅用于 LLM 推理后端场景切换 | 本地直跑 |

#### 二、values → 环境变量映射

Helm Chart 通过 `templates/deployment.yaml` 把 values 中的配置渲染为 Pod env（部分经 ConfigMap/Secret 挂载后由 env 指向文件路径），核心映射如下：

| values.yaml 键 | 渲染产物 | 注入方式 / 对应环境变量 |
|---|---|---|
| `agent.profile` | ConfigMap `<fullname>-config` → `privacy-profile.yaml` | 卷挂载到 `/etc/PrivShield/privacy-profile.yaml`（只读），`PRIVACY_PROFILE` 指向该路径；对应本地 `.env` 的 `PRIVACY_PROFILE` |
| `agent.logLevel` / `agent.logFormat` | Pod env | `PRIVACY_LOG_LEVEL`（转大写）/ `PRIVACY_LOG_FORMAT` |
| `service.restPort` / `service.grpcPort` | Pod env | `PRIVACY_REST_PORT` / `PRIVACY_GRPC_PORT`（REST/gRPC Host 固定 `0.0.0.0`） |
| `security.tls.*` | Pod env + Secret 卷 | `PRIVACY_TLS_ENABLED` / `PRIVACY_TLS_CERT_FILE` / `PRIVACY_TLS_KEY_FILE` / `PRIVACY_TLS_CLIENT_AUTH`；`caSecret` → `PRIVACY_TLS_CA_FILE`；`keyPasswordSecret` → `PRIVACY_TLS_KEY_PASSWORD`（secretKeyRef） |
| `security.auth.*` | Pod env + Secret 引用 | `PRIVACY_AUTH_ENABLED`；`apiKeysSecret` / `apiKeys` → `PRIVACY_AUTH_EXTERNAL_KEYS_JSON`（secretKeyRef，键 `api-keys.json`） |
| `security.auth.internalMtls.*` | Pod env + ConfigMap 挂载 | `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED`；`whitelistConfigMap` → `PRIVACY_AUTH_MTLS_WHITELIST_FILE`（挂载 `mtls-whitelist.yaml`）；`allowedCns` → `PRIVACY_AUTH_MTLS_ALLOWED_CNS` |
| `security.rateLimit.*` | Pod env | `PRIVACY_RATE_LIMIT_ENABLED` / `PRIVACY_RATE_LIMIT_DEFAULT_RPS` / `PRIVACY_RATE_LIMIT_DEFAULT_BURST`；`redisUrl` → `PRIVACY_RATE_LIMIT_REDIS_URL`；`perEndpointJson` → `PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON` |
| `extraEnv` | Pod env（原样透传，标准 K8s env 结构） | **任意** `PRIVACY_*` / `OTEL_*`，用于 values 无对应字段的配置（`PRIVACY_LLM_*`、`PRIVACY_BUDGET_*`、`PRIVACY_NER_ENABLE`、`PRIVACY_IMAGE_ALLOWED_DIRS` 等） |

#### 三、优先级规则

运行时生效优先级（从高到低）：

1. **K8s Pod env**——values 渲染 + `extraEnv` 透传 + Secret/ConfigMap 引用注入的环境变量，最高优先级；
2. **`config/env/<profile>.env`**——仅当进程内存在根 `.env` 且设置了 `PRIVACY_ENV_PROFILE` 时加载（`override=True` 覆盖基础值）；
3. **根目录 `.env`**——`load_env_file()` 默认 `override=False`，不覆盖已存在的环境变量。

即：**K8s 中由 values 注入的 env 永远优先**，env 文件只在本地无系统变量的场景兜底。

#### 四、K8s 部署下的实际行为（重要）

- 镜像构建的 `.dockerignore` 排除了 `.env` / `.env.*`（仅保留 `.env.example`），因此**镜像内不存在根 `.env`**；
- `load_env_file()` 找不到主 `.env` 时立即返回，**`config/env/*.env` 不会加载**——K8s 部署中 env 文件完全不参与配置，全部由 values 控制；
- 镜像虽打包了 `config/` 目录（含 `config/env/*.env`），但其中指向 `127.0.0.1`、`.models/` 等本地路径，容器内不适用；
- 因此：**values 无对应字段的配置（LLM 后端 `PRIVACY_LLM_*`、预算库 `PRIVACY_BUDGET_DB`、NER 开关等）一律通过 `extraEnv` 注入**，不要依赖 env 文件；
- 敏感信息（API Key、TLS 证书）只经 Secret 注入，禁止写入 env 文件。

#### 五、Docker Compose 对照

Compose 场景下 env 文件的用法与 K8s 不同：

- `env_file: ../../.env` 把根目录 `.env` **直接注入容器环境变量**（`deploy/docker-compose/docker-compose.yml`），优先级等同 K8s Pod env；容器内无 `.env` 文件，`load_env_file()` 不生效；
- Compose 同时读取根目录 `.env` 进行 `${VAR}` 变量替换（如镜像 tag）；
- 容器内必需的覆盖项（`PRIVACY_REST_HOST=0.0.0.0`、`PRIVACY_LOG_FORMAT=json`、`PRIVACY_PROFILE`、`PRIVACY_BUDGET_DB`）在 compose `environment:` 节显式声明，避免与宿主机开发取值（`127.0.0.1`、`text` 日志）冲突。

### 4.5 探针配置

- **livenessProbe**: `GET /health` 每 10s，失败 3 次重启。
- **readinessProbe**: `GET /readyz` 每 5s，失败 3 次移出流量；额外校验配置解析器与隐私预算 DB 连通性。

## 5. core/ml 镜像分层

| 镜像 | 内容 | 适用场景 |
|---|---|---|
| core | 仅核心依赖 | 脱敏、DP、K-匿名、规则分类 |
| ml | core + torch/transformers/onnxruntime | 完整三层分类 |

分层设计减少默认镜像体积与攻击面，用户按需选择。

## 6. LLM 推理服务部署设计

三层分类漏斗的 Layer-3（LLM 分类/仲裁）提供**集成模式**与**解耦模式**两种部署形态，对应 `PRIVACY_LLM_PROVIDER` 的 `qwen3` 与 `vllm`。二者决定镜像选型、副本策略与 GPU 资源归属，是生产部署的核心决策点。

| 维度 | 集成模式（进程内本地推理） | 解耦模式（外部独立 vLLM 服务） |
|---|---|---|
| 推理引擎 | LlmAdapter 进程内加载 Qwen3.5（transformers PyTorch） | OpenAILlmClassifier 经 HTTP 调用外部 vLLM（OpenAI 兼容 API） |
| `PRIVACY_LLM_PROVIDER` | `qwen3` | `vllm` |
| Agent 镜像 | **ml**（含 torch/transformers） | **core**（无需 ML 依赖） |
| GPU 归属 | Agent Pod（每副本一份模型） | 独立 vLLM Deployment/Pod |
| 多副本扩容 | ❌ 不可（每副本一份模型，OOM 风险） | ✅ core 多副本共享单个 LLM 实例 |
| 适用场景 | 一体式单机/离线交付、测试、无独立 GPU 编排 | 生产多副本高可用、GPU 集中管理、滚动发布不打断模型加载 |

### 6.1 集成模式：进程内本地推理（provider=qwen3）

**原理**：agent 进程内加载 Qwen3.5 分类模型（`PRIVACY_LLM_PROVIDER=qwen3`，transformers PyTorch 后端，`llm_adapter.py` 懒加载单例），LLM 推理与脱敏/分类业务同进程。

**关键约束**：

- 每副本加载一份模型 → N 副本 = N 份显存/内存，**禁止 HPA 多副本扩容**（历史上已发生多实例并发推理叠加导致 OOM、Go 客户端 `connection reset by peer` 的事故）；
- 进程级并发护栏（信号量/内存预检/超时降级）仅保护单进程，跨 Pod 无效（见 §6.3）；
- 滚动更新 `maxSurge` 期间新旧副本并存 = 双份模型，需确保节点资源富余。

**Helm 部署**（Chart 未提供集成模式开关，values 需自行注入）：

- `flavor: ml`（使用 ml 镜像）；
- `extraEnv` 注入 4 个变量：`PRIVACY_LLM_PROVIDER=qwen3`、`PRIVACY_LLM_MODEL_PATH=/models/<model-dir>`、`PRIVACY_LLM_DEVICE=cuda`（或 `cpu`）、`PRIVACY_LLM_ENABLE=true`（需要 Layer-2 时另加 `PRIVACY_NER_ENABLE=true`）；
- `extraVolumes` + `extraVolumeMounts` 把模型目录挂载到 `/models`（`hostPath` 或 PVC/`existingClaim`，只读，换模型零重建）；
- 节点调度：`nodeSelector`/`tolerations` 指向 GPU 节点，`resources.limits` 声明 `nvidia.com/gpu: 1`；
- **勿开 `llm.enabled`**：模板注入的 `PRIVACY_LLM_PROVIDER=vllm` 会覆盖 `qwen3`，两模式不可混用。

### 6.2 解耦模式：外部独立 vLLM 服务（provider=vllm）

**原理**：LLM 推理卸载到独立 vLLM 服务（OpenAI 兼容 `/v1` API），agent 经 `OpenAILlmClassifier` HTTP 调用，进程内无 torch 依赖 → **core 镜像即可**。核心动机：

- 多副本高可用与 LLM 单点资源解耦：core 副本数可随业务负载弹性伸缩（HPA），共享同一个 vLLM 实例；
- GPU 资源集中管理：模型常驻一个（或少量）vLLM Pod，加载/升级不随 agent 发布反复发生；
- 镜像体积与攻击面最小化：agent 不打包重型 ML 依赖。

**配置**（agent 侧 `extraEnv` 注入）：`PRIVACY_LLM_PROVIDER=vllm`、`PRIVACY_LLM_API_BASE=<vLLM 地址>/v1`、`PRIVACY_LLM_MODEL_NAME=<served-model-name>`、`PRIVACY_LLM_API_KEY=EMPTY`（本地无需鉴权时）。

> **易错点**：`PRIVACY_LLM_MODEL_NAME` 必须与 vLLM 启动参数 `--served-model-name` **完全一致**，否则 vLLM 返回 404（模型不存在）。

**部署形态 A：Helm 全托管**（`--set llm.enabled=true`）——Chart 自动渲染：

- `<fullname>-llm` Deployment：`vllm/vllm-openai` 镜像、模型挂载（`llm.storage.hostPath`/`existingClaim`）、GPU 调度（`llm.nodeSelector`/`tolerations`）、`resources.limits."nvidia.com/gpu": 1`、探针（`initialDelaySeconds: 60`，模型加载慢）、`--gpu-memory-utilization`/`--max-model-len` 等启动参数；
- `<fullname>-llm` Service（ClusterIP，端口 8000）；
- 自动向 core 注入 `PRIVACY_LLM_*` env，`PRIVACY_LLM_API_BASE=http://<fullname>-llm:8000/v1`（集群 DNS）。

**部署形态 B：外部已有 vLLM**（独立 namespace / 独立集群 / 宿主机）——`llm.enabled` 保持关闭，core `extraEnv` 指向外部地址：`PRIVACY_LLM_API_BASE=http://llm-svc.llm-ns.svc:8000/v1`（集群内）/ `http://<node-ip>:8000/v1`（跨集群需暴露端口）。Docker Compose 场景已内置 `vllm` 服务（`deploy/docker-compose/docker-compose.yml`），agent 自动注入 `API_BASE=http://vllm:8000/v1`。

### 6.3 并发与资源护栏

LLM 推理进程内置三道护栏（`llm_adapter.py`，模块级全局，所有 adapter 实例共享）：

| 护栏 | 环境变量 | 默认 | 作用 |
|---|---|---|---|
| 并发信号量 | `PRIVACY_LLM_MAX_CONCURRENCY` | 1 | 进程级推理并发上限（防 OOM 的核心闸门） |
| 排队超时 | `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | 30 | 等待推理槽位超时后降级（跳过 LLM 层） |
| 内存预检 | `PRIVACY_LLM_MIN_FREE_MEM_MB` | 512 | 可用内存低于阈值时跳过 LLM 层 |

**作用域边界**：护栏为**进程级**，跨 Pod 无效。因此集成模式必须限制副本数为 1（多副本 = 多份模型，护栏互不可见）；解耦模式下护栏约束的是 agent 侧发往 vLLM 的并发请求数，vLLM 侧的并发由 `--max-num-seqs` 等参数控制。

**决策矩阵**：

| 需求 | 选型 |
|---|---|
| 单机/离线/测试，无 GPU 编排需求 | 集成模式（ml 镜像，副本数=1） |
| 生产多副本 + 弹性伸缩 + GPU 集中管理 | 解耦模式（core 镜像 + 独立 vLLM） |
| 已有多副本 core 集群，想补 LLM 层 | 解耦模式（Helm `llm.enabled=true` 或指向外部 vLLM） |

## 7. 安全设计

- TLS 证书、API Key 均通过 `existingSecret` 注入，Chart 不生成随机密钥。
- NetworkPolicy 默认关闭，生产 values 中启用。
- ServiceAccount 默认创建，RBAC 最小化（无需访问 K8s API）。

## 8. 可观测性设计

- Prometheus 通过 ServiceMonitor 抓取 `/metrics`（values 可选）。
- 日志输出到 stdout/stderr，由集群日志系统采集。

## 9. 部署流程

```bash
# 1. 构建镜像（core）
docker build --target core -t PrivShield:0.1.0 .

# 2. 安装 Helm chart（开发模式）
helm install PrivShield ./deploy/helm/PrivShield

# 3. 生产模式 + 自管证书
helm install PrivShield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=my-tls-secret \
  --set security.auth.apiKeysSecret=my-apikeys-secret

# 4. 原生 K8s
kubectl apply -k ./deploy/k8s/

# 5. Docker Compose
cd deploy/docker-compose && docker compose up -d
```

## 10. 测试策略

- `helm lint` 与 `helm template` 通过。
- 原生 K8s manifests 可 `kubectl apply`。
- Docker Compose 可启动 Agent + Console 后端代理与 Web UI。
- core/ml 镜像构建成功。

## 11. 滚动更新与回滚策略 / Rolling Update & Rollback Strategy

### 11.1 滚动更新策略

Helm Chart 默认使用 Kubernetes 原生滚动更新（RollingUpdate），确保零停机发布：

```yaml
# values.yaml 默认配置
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0      # 更新期间不允许有 Pod 不可用
    maxSurge: 1            # 最多允许 1 个额外 Pod 同时运行
```

**更新流程 / Update Flow:**

1. **新 Pod 创建**: Kubernetes 创建新版本的 Pod（maxSurge=1）。
2. **就绪检查**: 新 Pod 通过 readinessProbe（`GET /readyz`）后加入 Service endpoints。
3. **旧 Pod 终止**: 旧 Pod 收到 SIGTERM，开始优雅关闭（terminationGracePeriodSeconds=30）。
4. **连接排空**: 旧 Pod 在关闭前完成处理中的请求（依赖网关/客户端重试）。
5. **重复**: 直到所有 Pod 更新完成。

**生产环境推荐配置 / Production Recommendations:**

```yaml
# values-production.yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0
    maxSurge: 25%          # 大规模部署时使用百分比

terminationGracePeriodSeconds: 60  # 延长优雅关闭时间

# 启用 PodDisruptionBudget 保证最小可用性
podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

### 11.2 回滚方案 / Rollback Plan

#### 11.2.1 Helm 回滚

```bash
# 查看发布历史 / View release history
helm history PrivShield

# 回滚到上一版本 / Rollback to previous version
helm rollback PrivShield

# 回滚到指定版本 / Rollback to specific revision
helm rollback PrivShield 3

# 回滚并等待完成 / Rollback and wait for completion
helm rollback PrivShield --wait --timeout 5m
```

#### 11.2.2 原生 K8s 回滚

```bash
# 查看 Deployment 历史 / View deployment history
kubectl rollout history deployment/PrivShield

# 回滚到上一版本 / Rollback to previous version
kubectl rollout undo deployment/PrivShield

# 回滚到指定版本 / Rollback to specific revision
kubectl rollout undo deployment/PrivShield --to-revision=3

# 查看回滚状态 / Watch rollback status
kubectl rollout status deployment/PrivShield
```

#### 11.2.3 自动回滚触发条件 / Auto-Rollback Triggers

建议配置以下监控告警作为自动回滚触发条件（结合 Argo Rollouts 或 Flagger）：

| 指标 | 阈值 | 持续时间 | 动作 |
|------|------|----------|------|
| 5xx 错误率 | > 5% | 2 分钟 | 自动回滚 |
| P95 延迟 | > 2s | 5 分钟 | 告警 + 人工确认 |
| Pod 重启次数 | > 3 次 | 5 分钟 | 自动回滚 |
| readinessProbe 失败 | 连续 3 次 | - | Kubernetes 自动处理 |

### 11.3 蓝绿部署（可选）/ Blue-Green Deployment (Optional)

对于关键业务场景，可使用 Argo Rollouts 实现蓝绿部署：

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: PrivShield
spec:
  replicas: 3
  strategy:
    blueGreen:
      activeService: PrivShield-active
      previewService: PrivShield-preview
      autoPromotionEnabled: false  # 手动确认切换
      abortScaleDownDelaySeconds: 30
  selector:
    matchLabels:
      app: PrivShield
  template:
    # ... Pod template spec
```

**切换流程 / Promotion Flow:**

```bash
# 1. 部署新版本（preview 环境）
kubectl apply -f rollout.yaml

# 2. 验证 preview 环境
curl http://PrivShield-preview:8079/health

# 3. 手动确认切换
kubectl argo rollouts promote PrivShield

# 4. 如有问题，快速回滚
kubectl argo rollouts abort PrivShield
```

### 11.4 金丝雀发布（可选）/ Canary Release (Optional)

使用 Argo Rollouts 或 Istio 实现渐进式流量切换：

```yaml
strategy:
  canary:
    steps:
      - setWeight: 10      # 10% 流量到新版本
      - pause: { duration: 5m }  # 观察 5 分钟
      - setWeight: 50      # 50% 流量
      - pause: { duration: 5m }
      - setWeight: 100     # 全量切换
    canaryMetadata:
      labels:
        version: canary
    stableMetadata:
      labels:
        version: stable
```

## 12. 工业化评分 / Industrialization Scorecard

> **工业化软件 = 功能正确 + 性能稳定 + 安全可靠 + 可维护 + 可观测 + 可快速迭代**
>
> 评估框架参考 ISO/IEC 25010 与 Google SRE 实践，采用 6 维度加权评分（1–10 分）。

### 12.1 加权评分表

| 维度 | 权重 | 得分 | 说明 |
|------|------|------|------|
| 功能完整性 | 20% | 8/10 | Helm Chart + K8s manifests + Docker Compose 三种部署形式；core/ml 双镜像；HPA 弹性 |
| 性能 | 15% | 7/10 | HPA 基于 CPU/内存扩展；资源 requests/limits 可配；缺少 VPA 与自定义指标扩展 |
| 可靠性 | 20% | 8/10 | liveness/readiness 探针；多副本支持；滚动更新策略 + 回滚方案 + 蓝绿/金丝雀可选 |
| 安全性 | 15% | 8/10 | Secret 注入（不硬编码）；NetworkPolicy 可选；RBAC 最小化；镜像分层减少攻击面 |
| 可维护性 | 15% | 7/10 | values 分层（default/production/ml）；缺少 Chart 版本变更日志 |
| 工程化 | 15% | 7/10 | CI 验证 Docker build + Helm chart-testing + Trivy 镜像扫描 |
| **总分** | **100%** | **7.55** | |

### 12.2 结论

**通过（Pass）**——满足工业化要求，可进入主线。

### 12.3 亮点

- 三种部署形式覆盖从本地到生产全场景。
- core/ml 镜像分层减少默认体积与攻击面。
- 敏感配置通过 Secret 注入，不硬编码到镜像。
- NetworkPolicy + RBAC 最小权限设计。
- 完整的滚动更新 + 回滚 + 蓝绿/金丝雀发布策略。
- CI 集成 chart-testing 与 Trivy 镜像漏洞扫描。

### 12.4 改进建议

| 优先级 | 建议 | 影响维度 |
|--------|------|----------|
| P2 | 添加自定义指标 HPA（基于 privacy_requests_total） | 性能 +1 |
| P3 | 补充 Chart CHANGELOG 与版本管理策略 | 可维护性 +0.5 |
| P3 | 集成 Argo Rollouts 实现自动化金丝雀发布 | 可靠性 +0.5 |

---

## 13. 容器化部署实战排坑与经验总结 / Deployment Pitfalls & Best Practices

在 `PrivShield` 项目全栈微服务（Python Agent + Go gRPC 代理 + Python REST 代理 + React 前端 + vLLM GPU 推理）的容器化与多环境交付过程中，总结沉淀了以下关键技术陷阱与最佳实践方案。

### 13.1 本地与私有镜像拉取策略（`pull_policy: build`）

#### 陷阱场景
执行 `docker compose up -d` 启动包含本地 Dockerfile 构建的服务时，Docker Compose 遇到未在本地缓存的自定义镜像 Tag（如 `privacy-console-backend-python:0.1.0`），默认策略仍会尝试向 Docker Hub（`docker.io`）发起 HEAD / Manifest 查询请求。在受限网络或无公共推送权限的环境下，会导致：
```text
failed to resolve reference "docker.io/library/privacy-console-backend-python:0.1.0": unexpected status from HEAD request
```

#### 根本原因
Docker Compose 2.x 的 `pull_policy` 默认值为 `missing`。如果服务指定了 `image: xxx:tag` 且本地不存在该 Tag，Compose 倾向于先向远程 Registry 尝试拉取，失败后才尝试本地构建，甚至在部分网络超时场景下直接中断退出。

#### 最佳实践与解法
1. **显式声明拉取策略**：在 `docker-compose.yml` 中为所有本地构建服务显式添加 `pull_policy: build`：
   ```yaml
   console-backend-python:
     build:
       context: ../../console/bff-py
       dockerfile: Dockerfile
     image: privacy-console-backend-python:0.1.0
     pull_policy: build  # 强制本地构建，禁止向远程 Registry 拉取未推送 Tag
   ```
2. **启动脚本默认注入 `--build`**：在 `docker-start-all.sh` / `docker-start-go.sh` 等脚本中，默认追加 `--build` 参数，并提供 `--no-build` 选项供快速重用。

---

### 13.2 容器隔离构建引擎与宿主机 Daemon 代理冲突（Buildx 驱动选择）

#### 陷阱场景
当主机上安装了其他容器化工具（如 Kuscia / Kubernetes In Docker / 独立 BuildKit 容器）时，`docker buildx` 的默认 builder 可能会被切换为 `docker-container` 驱动（如 `kuscia`，网络命名空间绑定在 `172.17.0.2`）。此时执行镜像构建：
```text
failed to resolve source metadata for docker.io/library/python:3.13-slim-bookworm: 
read tcp 172.17.0.2:49146->23.21.28.55:443: read: connection reset by peer
```

#### 根本原因
`docker-container` 驱动运行在隔离的容器网络中，其 BuildKit 守护进程不会继承宿主机 `/etc/docker/daemon.json` 中配置的 `registry-mirrors`（镜像加速器）与系统代理，而是直接向 Docker Hub 官方海外地址建连，极易被国内网络重置。

#### 最佳实践与解法
1. **切换为宿主机默认引擎**：在构建前检查并切换 buildx builder 为宿主机默认 Docker Daemon 驱动：
   ```bash
   # 检查当前构建器
   docker buildx ls
   # 切换回默认宿主机 daemon（继承 daemon.json 中的所有 mirror 与代理配置）
   docker buildx use default
   ```
2. **预拉取基础镜像**：对于大型多阶段基础镜像（如 `golang:1.23.4-alpine3.20`、`python:3.13-slim-bookworm`），利用宿主机 daemon 事先 `docker pull`，利用本地缓存规避编译期拉取抖动。

---

### 13.3 基础镜像 Tag 锁定与语义规范

#### 陷阱场景
在 Dockerfile 中使用了非官方发布的 Patch 版本标签（例如将 Debian 12 Bookworm 基础镜像写作 `FROM python:3.13.13-slim-bookworm`），导致拉取镜像阶段报错：
```text
manifest unknown: manifest unknown
```

#### 根本原因
Docker Hub 官方 Python 镜像的命名规范为 `python:<major>.<minor>-slim-<distro>`（例如 `python:3.13-slim-bookworm`）或具体的已发布微版本 `python:3.13.2-slim-bookworm`，不存在未发布的 `3.13.13`。

#### 最佳实践与解法
- 项目内各服务 Dockerfile 统一基础镜像规范，并在 `AGENTS.md` / `design.md` 中形成清单：
  - Python 运行时：`python:3.13-slim-bookworm`
  - Go 编译阶段：`golang:1.23.4-alpine3.20`
  - Alpine 运行阶段：`alpine:3.20.3`
  - Node 前端构建：`node:20.18.0-alpine3.20`
  - Nginx 前端托管：`nginx:1.26.2-alpine`

---

### 13.4 多语言全链路容器构建加速体系（Debian / Alpine / Go / NPM）

#### 陷阱场景
多阶段构建中，容器内部执行 `apt-get update`、`apk add`、`go mod download`、`pnpm install`、`pip install` 时，由于容器内默认访问海外官方源，构建过程耗时常达 10~30 分钟，且容易遭遇连接重置失败。

#### 最佳实践与解法
在各语言的多阶段 Dockerfile 中，统一注入国内镜像加速源，使全量冷构建缩短至 1~2 分钟内：

1. **Debian APT 加速（兼容 Debian 12 `debian.sources` 与旧 `sources.list`）**：
   ```dockerfile
   RUN (sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources 2>/dev/null \
        || sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list 2>/dev/null || true) \
       && apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
       && rm -rf /var/lib/apt/lists/*
   ```
2. **Alpine APK 加速**：
   ```dockerfile
   RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories 2>/dev/null || true \
       && apk add --no-cache git ca-certificates curl
   ```
3. **Go Module 代理加速**：
   ```dockerfile
   ENV GOPROXY=https://goproxy.cn,direct
   ```
4. **Python Pip 加速**：
   ```dockerfile
   RUN pip install --no-cache-dir -r requirements.txt \
       -i https://mirrors.aliyun.com/pypi/simple/ \
       --extra-index-url https://pypi.tuna.tsinghua.edu.cn/simple
   ```
5. **Node.js / NPM / Pnpm 加速**：
   ```dockerfile
   RUN npm config set registry https://registry.npmmirror.com \
       && npm install -g pnpm \
       && pnpm config set registry https://registry.npmmirror.com \
       && pnpm install --frozen-lockfile
   ```

---

### 13.5 Go 现代化工具链动态下载与版本协同（`GOTOOLCHAIN=auto`）

#### 陷阱场景
当 `go.mod` 声明的依赖库（例如 `github.com/gin-gonic/gin@v1.12.0`）要求 `go >= 1.25.0`，而基础镜像为 `golang:1.23.4-alpine` 时，编译报错：
```text
go: github.com/gin-gonic/gin@v1.12.0 requires go >= 1.25.0 (running go 1.23.4; GOTOOLCHAIN=local)
```

#### 根本原因
Go 1.21+ 引入了 Go Toolchain 机制。在容器环境中，默认环境变量 `GOTOOLCHAIN=local` 禁止 Go 自动下载升级所需的更高版本编译器。

#### 最佳实践与解法
在 Go 编译阶段 Dockerfile 中显式配置：
```dockerfile
ENV GOPROXY=https://goproxy.cn,direct
ENV GOTOOLCHAIN=auto
```
`GOTOOLCHAIN=auto` 配合 `GOPROXY` 可让 Go 编译器在遇到依赖库需要更高版本时，通过国内代理自动拉取目标 toolchain 并无缝完成构建，无需反复更换底层 Docker 基础镜像。

---

### 13.6 非 Root 容器安全运行与命名卷挂载权限冲突

#### 陷阱场景
容器遵循安全最佳实践以非 root 用户（`USER privacy`，UID/GID 系统分配）运行。启动时初始化持久化 SQLite 预算数据库（`PRIVACY_BUDGET_DB=/data/budget/budget.db`）报错：
```text
sqlite3.OperationalError: unable to open database file
```
容器启动失败并陷入 CrashLoopBackOff 或处于 `unhealthy` 状态。

#### 根本原因
1. 当 Docker 首次初始化并挂载命名数据卷（如 `budget-db:/data/budget`）时，若镜像构建阶段未在基础镜像中预先创建该目录并赋予 `privacy` 用户权限，Docker Daemon 会默认以 `root:root` (0755) 属主创建卷目录；
2. 容器以普通用户 `privacy` 启动后，对 `/data/budget` 仅有只读权限，无法在其下创建 SQLite 数据库文件及 WAL 锁日志。

#### 最佳实践与解法
1. **Dockerfile 预建并授权目录**：在切换 `USER privacy` 之前，以 root 身份显式预建全部数据卷挂载点并递归授权：
   ```dockerfile
   # 创建数据与日志持久化挂载目录并授权 privacy 用户
   RUN mkdir -p /data/budget /var/log/privacy \
       && chown -R privacy:privacy /app /data /var/log/privacy
   
   USER privacy
   ```
2. **应用层递归创建父目录兜底**：在 `budget.py` 中初始化 SQLite 连接前增加目录检测与创建：
   ```python
   db_path = os.environ.get("PRIVACY_BUDGET_DB")
   if db_path:
       parent_dir = os.path.dirname(db_path)
       if parent_dir:
           os.makedirs(parent_dir, exist_ok=True)
       conn = sqlite3.connect(db_path, timeout=10.0)
   ```

---

### 13.7 跨组件探针与健康检查端点一致性（`/health` 与 `/api/health`）

#### 陷阱场景
Docker Compose 配置的 `healthcheck` 探测 `http://localhost:8080/health` 或 `http://localhost:8081/health` 时返回 HTTP 404，导致容器一直处于 `(health: starting)` 甚至被标记为 `unhealthy` 触发依赖熔断。

#### 根本原因
控制台代理后端路由设计时将业务健康检查注册在 `/api/health`（带 `/api` 前缀以适配前端路由代理与 API 网关），而标准 Docker HEALTHCHECK 与部分 K8s Liveness 探针习惯探测根路径 `/health`。

#### 最佳实践与解法
在 Python 后端（FastAPI）与 Go 后端（Gin）中**双重注册**健康检查端点，兼顾标准容器探针与 API 代理调用：
- **FastAPI**:
  ```python
  @app.get("/health")
  @app.get("/api/health")
  async def health(): ...
  ```
- **Gin**:
  ```go
  r.GET("/health", s.Health)
  r.GET("/api/health", s.Health)
  ```

---

### 13.8 容器网络隔离下的服务发现与协议回退寻址

#### 陷阱场景
Go gRPC 后端在处理非 gRPC 接口回退（如 `/v1/dynclassification/*` 规则热重载与评估）时，调用上游 REST 服务报错：
```text
Agent REST HTTP error: Post "http://127.0.0.1:8079/v1/...": dial tcp 127.0.0.1:8079: connect: connection refused
```

#### 根本原因
在 Docker Compose 默认的桥接网络（Bridge Network）中，每个容器拥有独立的 Network Namespace。`127.0.0.1` 仅代表当前代理容器自身，跨容器访问必须使用 Docker 内部 DNS 解析的服务名（如 `PrivShield:8079`）。

#### 最佳实践与解法
1. **多层级环境回退解析**：在 Go 后端 `agentRestBaseURL()` 中支持环境变量优先覆盖与智能回退：
   ```go
   func agentRestBaseURL() string {
       if u := os.Getenv("PRIVACY_AGENT_URL"); u != "" {
           return strings.TrimRight(u, "/")
       }
       if u := os.Getenv("PRIVACY_AGENT_REST_URL"); u != "" {
           return strings.TrimRight(u, "/")
       }
       restHost := os.Getenv("PRIVACY_AGENT_REST_HOST")
       if restHost == "" {
           restHost = os.Getenv("PRIVACY_REST_HOST")
       }
       if restHost == "" {
           restHost = os.Getenv("PRIVACY_AGENT_GRPC_HOST") // 自动复用 gRPC 服务名（如 PrivShield）
       }
       if restHost == "" {
           restHost = "127.0.0.1"
       }
       // ... 拼接端口
   }
   ```
2. **Compose 环境变量显式声明**：在 Compose 中为代理容器注入 `PRIVACY_AGENT_URL: "http://PrivShield:8079"` 与 `PRIVACY_AGENT_GRPC_HOST: "PrivShield"`。

---

### 13.9 测试与 CI 执行环境中的 Linux `ARG_MAX` 限制规避

#### 陷阱场景
自动化测试套件（pytest / go test）在通过 `subprocess.run` 调用运维或部署脚本时，抛出系统级错误：
```text
OSError: [Errno 7] Argument list too long
```

#### 根本原因
在现代复杂 CI/CD 或 AI Agent 执行环境中，父进程环境中可能注入了大量超长上下文变量（如数十 KB 的全量 Prompt / Transcript 日志）。当 `subprocess.run` 默认继承 `os.environ` 时，所有环境变量键值对占用的总字节数超过了 Linux 内核由 `MAX_ARG_PAGES` / `ARG_MAX` 设定的内存页限制（通常为 2MB 左右）。

#### 最佳实践与解法
在测试辅助模块中定义环境变量清洗函数 `_clean_env()`，过滤掉超长与无关上下文变量后再传递给子进程：
```python
def _clean_env() -> dict[str, str]:
    """过滤超长环境变量，防止触发 Linux ARG_MAX 限制"""
    clean = {}
    for k, v in os.environ.items():
        # 排除超过 4KB 的大体积变量与特定 agent 调试上下文
        if len(v) < 4096 and not k.startswith(("AGENT_", "CONTEXT_", "PROMPT_")):
            clean[k] = v
    return clean
```

---

### 13.10 运行时业务规范文档与 `.dockerignore` 排除冲突（`generate_profile`）

#### 陷阱场景
调用 `POST /v1/dynclassification/generate_profile` 自动解析标准 Markdown 文档并生成 YAML 分类体系时，在容器中报错：
```text
{"detail": "Failed to generate configuration from document"}
# 容器内部日志：
FileNotFoundError: [Errno 2] No such file or directory: '/app/docs/standard/四川省健康医疗大数据应用指南.md'
```

#### 根本原因
1. 传统镜像打包习惯在 `.dockerignore` 中将 `docs/` 和 `*.md` 全局排除以缩减镜像体积；
2. 动态分类分级模块具备从合规标准文档（`docs/standard/*.md`）一键逆向生成规则配置的核心业务能力（`StandardProfileGenerator`），运行时对标准文档有直接读取依赖。全局排除导致容器内部缺少合规标准文档资产。

#### 最佳实践与解法
1. **`.dockerignore` 精确白名单例外**：使用通配符与负向规则保留标准文档目录：
   ```dockerignore
   docs/*
   !docs/standard/
   !docs/standard/*.md
   site/
   *.md
   !README.md
   !docs/standard/*.md
   ```
2. **Dockerfile 显式打包标准文档**：
   ```dockerfile
   COPY docs/standard/ ./docs/standard/
   ```
3. **Docker Compose 开发期目录挂载**：
   ```yaml
   volumes:
     - ../../docs/standard:/app/docs/standard:ro  # 实时感知宿主机文档更新
   ```