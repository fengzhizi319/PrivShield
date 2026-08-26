# Docker 多阶段构建与 Kubernetes/Helm 云原生部署技术指南 / Cloud-Native Deployment, Docker, K8s & Helm Technical Guide

## 1. 技术简介 / Introduction

为了满足不同企业在边缘侧、私有私密专网以及公有云 Kubernetes 集群中部署 `PrivShield` 的差异化需求，本项目构建了现代化的**云原生容器化与编排交付流水线**：

1. **Docker 多阶段分层构建（Multi-stage Build）**：实现轻量核心镜像（`core` ~150MB）与全功能机器学习镜像（`ml` ~3GB）的按需产出；
2. **非 Root 容器最小权限基线（Least Privilege Non-Root User）**：创建系统级无 shell `privacy` 用户，杜绝容器逃逸攻击；
3. **Docker Compose 多场景编排（Profile Orchestration）**：支持 Agent-Only、开发三件套（Agent+BFF+Web）、全栈微服务（带 3 个 Go 中台）以及监控链路的一键拉起；
4. **企业级 Helm Chart 与 K8s 交付（Production Helm Chart）**：提供基于 HPA 弹性伸缩、双协议健康探针（Liveness/Readiness）、TLS/API Key Secret 挂载与拓扑反亲和性（Anti-Affinity）调度。

```text
                                源代码提交 (Git Push)
                                          │
                                          ▼
                ┌──────────────────────────────────────────────────┐
                │ ★ Dockerfile 多阶段构建 (Multi-Stage Pipeline)    │
                │   [base] python:3.13-slim + non-root user        │
                │     ├──► [core] 轻量算力镜像 (仅脱敏/DP/规则)     │
                │     └──► [ml]   全量 ML 镜像 (+ torch/onnx/qwen)  │
                └─────────────────────────┬────────────────────────┘
                                          │
                  ┌───────────────────────┴───────────────────────┐
                  ▼                                               ▼
     ┌────────────────────────┐                      ┌────────────────────────┐
     │ Docker Compose 编排    │                      │ Kubernetes & Helm 编排 │
     │ - 全栈微服务群协同     │                      │ - HPA 弹性伸缩与拓扑亲和 │
     │ - vLLM 独立推理容器    │                      │ - 双协议存活/就绪探针  │
     │ - Prometheus + Grafana │                      │ - Secret / ConfigMap 挂载│
     └────────────────────────┘                      └────────────────────────┘
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 Docker 多阶段构建与镜像轻量化 / Dockerfile Multi-Stage Architecture

文件 / File：[`Dockerfile`](Dockerfile)

```dockerfile
# ==============================================================================
# Stage 1: base —— 公共基础层（共享缓存与非 root 安全基线）
# ==============================================================================
FROM python:3.13-slim-bookworm AS base

WORKDIR /app

# 1. 安装精简系统工具并创建无登录权限的 privacy 运行用户
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy

# 2. 单独复制依赖清单并安装（充分命中 Docker 分层缓存）
COPY requirements-core.txt .
RUN pip install --no-cache-dir -r requirements-core.txt

# ==============================================================================
# Stage 2: core —— 默认轻量运行镜像（~150MB，推荐生产使用）
# ==============================================================================
FROM base AS core

COPY engine/ ./engine/
COPY rules/ ./rules/
COPY config/ ./config/
COPY proto/ ./proto/
COPY scripts/ ./scripts/

# 切换非 root 用户
USER privacy

EXPOSE 8079 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8079/health || exit 1

CMD ["python", "-m", "engine.server"]

# ==============================================================================
# Stage 3: ml —— 完整 ML 镜像（含 PyTorch、Transformers 与 ONNX）
# ==============================================================================
FROM core AS ml

USER root
COPY requirements-ml.txt .
RUN pip install --no-cache-dir -r requirements-ml.txt
USER privacy

CMD ["python", "-m", "engine.server"]
```

#### 构建命令：
```bash
# 默认构建 core 镜像
docker build --target core -t privshield:1.8.0 .

# 按需构建 ml 镜像
docker build --target ml -t privshield:1.8.0-ml .
```

---

### 2.2 Helm Chart 生产级配置与健康探针 / Production Helm Chart

文件 / File：[`deploy/helm/PrivShield/`](deploy/helm/PrivShield/)

#### 探针与优雅停机设计 (Probes & Lifecycle)

```yaml
# deploy/helm/PrivShield/templates/deployment.yaml
containers:
  - name: privshield
    image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
    ports:
      - name: rest
        containerPort: 8079
      - name: grpc
        containerPort: 50051
    # 存活探针：轻量检查 HTTP 服务端口是否连通
    livenessProbe:
      httpGet:
        path: /livez
        port: rest
      initialDelaySeconds: 10
      periodSeconds: 15
    # 就绪探针：检查核心规则加载及可选预算 SQLite 数据库连通性
    readinessProbe:
      httpGet:
        path: /readyz
        port: rest
      initialDelaySeconds: 5
      periodSeconds: 5
    # 优雅停机等待时间（给在途 DP 计算与链路导出器刷新预留时间）
    lifecycle:
      preStop:
        exec:
          command: ["/bin/sh", "-c", "sleep 5"]
```

#### 生产部署命令：
```bash
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=privshield-tls-secret \
  --set security.auth.apiKeysSecret=privshield-apikeys-secret
```

---

### 2.3 Docker Compose 全栈微服务协同编排 / Docker Compose Orchestration

文件 / File：[`deploy/docker-compose/docker-compose.yml`](deploy/docker-compose/docker-compose.yml)

`docker-compose.yml` 统领了 1 个 Python Agent、3 个 Go 中台微服务、1 个 Go BFF、1 个 React 前端控制台以及 Prometheus + Grafana：

```yaml
services:
  agent:
    build:
      context: ../../
      target: core
    ports:
      - "8079:8079"
      - "50051:50051"
    environment:
      - PRIVACY_REST_HOST=0.0.0.0
      - PRIVACY_LOG_LEVEL=INFO

  service-hub:
    build:
      context: ../../
      dockerfile: services/service-hub/Dockerfile
    ports:
      - "8082:8082"
      - "50052:50052"
    depends_on:
      - agent

  prometheus:
    image: prom/prometheus:v2.50.0
    profiles: ["monitoring"]
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
```

#### 场景启动脚本：
```bash
# 启动全栈微服务群
bash ./scripts/dev/e2e-start-all-services.sh

# 带 Prometheus 监控启动
docker compose --profile monitoring up -d
```

---

## 3. Docker 分层缓存与 `.dockerignore` 深度优化 / Layer Caching & Build Context Optimization

### 3.1 Docker 构建缓存命中原理 / Build Cache Invalidation Rules

Docker 镜像由一系列只读层（Layer）堆叠而成。每条 Dockerfile 指令（`FROM`、`RUN`、`COPY`、`ENV` 等）都会生成一个新层。当某条指令的内容与其所有前置指令的内容均未发生变化时，Docker 将直接复用已有缓存层，跳过实际执行，从而将构建时间从数分钟压缩至数秒。

**缓存失效的「雪崩效应」**：一旦某一层被判定为过期（invalidated），其后的**所有层**均会被强制重新构建，即便它们自身的内容并未改变。因此，Dockerfile 的指令顺序直接决定了构建效率。

```text
  优化策略：将「变化频率低」的指令放在前面，「变化频率高」的指令放在后面

  ┌─────────────────────────────────────────────────────┐
  │ FROM python:3.13-slim     ← 几乎不变（系统级缓存）      │ ✅ 命中
  │ RUN apt-get install ...   ← 极少变化                    │ ✅ 命中
  │ COPY requirements*.txt .  ← 依赖变更时才失效             │ ✅ 命中
  │ RUN pip install ...       ← 依赖变更时才失效             │ ✅ 命中
  │ COPY engine/ ./engine/    ← 业务代码频繁变更             │ ❌ 失效
  │ COPY rules/ ./rules/      ← 被连带失效                   │ ⚠️ 重建
  │ CMD ["python", ...]       ← 被连带失效                   │ ⚠️ 重建
  └─────────────────────────────────────────────────────┘
```

### 3.2 `.dockerignore` 排除无关文件 / Excluding Irrelevant Files

构建上下文（Build Context）是 Docker 客户端发送给 Docker Daemon 的完整目录快照。如果不过滤，`.git/`、`node_modules/`、`.venv/`、`__pycache__/` 等巨型目录会被全量上传，导致：
1. 构建上下文体积膨胀（可达数 GB），网络传输耗时；
2. 缓存命中率下降（文件指纹变化导致 COPY 层失效）；
3. 敏感文件（`.env`、私钥）意外打入镜像。

项目根目录的 [`Dockerfile`](Dockerfile) 配套 [`Dockerfile`](Dockerfile) 使用了以下 `.dockerignore`：

```text
# .dockerignore — 构建上下文过滤规则
.git/                    # Git 元数据（可达数百 MB）
.venv/                   # Python 虚拟环境
__pycache__/             # Python 字节码缓存
*.pyc                    # 编译后字节码
node_modules/            # 前端依赖（前端独立构建）
.logs/                   # 运行时日志
.pytest_cache/           # 测试缓存
.mypy_cache/             # 类型检查缓存
*.egg-info/              # Python 包元数据
dist/                    # 前端构建产物
console/web/dist/        # 前端构建产物（由 Nginx 镜像独立处理）
deploy/                  # K8s/Helm 部署清单（运行时不需要）
docs/                    # 文档（运行时不需要）
tests/                   # 测试代码（运行时不需要）
.env*                    # 环境变量文件（防止敏感信息泄露）
*.key                    # TLS 私钥
*.pem                    # 证书文件
```

### 3.3 `requirements.txt` 前置复制模式 / Dependency-First COPY Pattern

这是 Python Docker 构建中最经典的缓存优化技巧：先单独复制依赖清单文件并安装，再复制业务代码。这样，只要 `requirements-core.txt` 未变更，即便业务代码每天都在改，`pip install` 层也能稳定命中缓存。

```dockerfile
# ✅ 推荐：依赖前置复制模式
COPY requirements-core.txt .          # 仅复制依赖清单
RUN pip install --no-cache-dir -r requirements-core.txt  # 缓存友好
COPY engine/ ./engine/                # 业务代码最后复制
```

```dockerfile
# ❌ 反模式：一次性复制全部源码
COPY . .                              # 任何文件变化都导致此层失效
RUN pip install -r requirements.txt   # 连带 pip install 也被迫重建
```

### 3.4 `--no-cache-dir` 与镜像体积优化

`pip install --no-cache-dir` 禁止 pip 将下载的 `.whl` 包缓存到 `~/.cache/pip/`。在 Docker 构建中，这些缓存毫无用处（因为镜像层本身就是只读的），却会白白增加 50~200MB 的镜像体积。

```bash
# 镜像内 pip 缓存占用空间对比
# 使用 --no-cache-dir：~150MB
docker images privshield:1.8.0

# 不使用 --no-cache-dir：~350MB（缓存白白浪费 200MB）
```

---

## 4. 非 Root 容器安全基线深入 / Non-Root Container Security Deep Dive

### 4.1 为什么必须使用非 Root 用户运行容器？

Docker 容器默认以 Root (UID 0) 身份运行进程。虽然容器通过 Namespace 和 Cgroup 实现了进程隔离，但历史上 Linux 内核多次曝出容器逃逸漏洞（如 CVE-2019-5736 runC 覆写漏洞、CVE-2020-15257 containerd shim 漏洞）。如果容器以 Root 运行，一旦被攻破，攻击者可直接获得宿主机 Root 权限。

**最小权限原则 (Principle of Least Privilege)**：生产容器应使用专用的无登录权限系统用户运行，即便发生容器逃逸，攻击者也只能获得一个无 shell、无 home 目录的空壳账户。

### 4.2 `privacy` 用户创建详解

```dockerfile
# 创建系统级无登录权限用户
RUN groupadd -r privacy \
    && useradd -r -g privacy -d /app -s /sbin/nologin privacy
```

各参数含义：

| 参数 | 含义 | 安全目的 |
|---|---|---|
| `-r` (groupadd) | 创建系统组（GID < 1000） | 与普通用户组区分，便于审计 |
| `-r` (useradd) | 创建系统用户（UID < 1000） | 不分配用户家目录创建等额外开销 |
| `-g privacy` | 指定主组为 `privacy` | 文件权限按组控制 |
| `-d /app` | 家目录设为 `/app`（工作目录） | 不创建额外的 `/home/privacy` |
| `-s /sbin/nologin` | 登录 Shell 设为「禁止登录」 | 即便拿到凭据也无法 `ssh` 或 `su` |

### 4.3 `USER` 指令与文件权限对齐

```dockerfile
# 确保工作目录归 privacy 用户所有
WORKDIR /app
RUN chown -R privacy:privacy /app

# 切换到非 root 用户（后续所有指令均以此身份执行）
USER privacy

# 暴露端口（仅为文档目的，不实际打开端口）
EXPOSE 8079 50051
```

**常见陷阱**：如果 `COPY` 指令在 `USER privacy` 之后执行，复制的文件默认归 Root 所有，privacy 用户可能无权读取。解决方案是在 `USER` 切换之前完成所有 `COPY` 操作，或显式 `chown`。

### 4.4 K8s `securityContext` 双重保险

在 Kubernetes 中，除了 Dockerfile 层面的 `USER` 指令，还应在 Pod Spec 中设置 `securityContext` 作为双重保险：

```yaml
# deploy/helm/PrivShield/templates/deployment.yaml
spec:
  containers:
    - name: privshield
      securityContext:
        runAsNonRoot: true          # K8s 准入控制器强制校验
        runAsUser: 1000             # 明确指定 UID
        readOnlyRootFilesystem: true # 根文件系统只读（防止攻击者写入恶意脚本）
        allowPrivilegeEscalation: false  # 禁止 setuid 提权
        capabilities:
          drop: ["ALL"]             # 丢弃所有 Linux Capabilities
```

---

## 5. Docker Compose Profile 编排模式详解 / Docker Compose Profile Orchestration

### 5.1 Profile 机制简介

Docker Compose 的 `profiles` 字段允许按场景条件性激活服务组。未指定 `profiles` 的服务始终启动；指定了 `profiles: ["monitoring"]` 的服务仅在显式请求时启动。

```yaml
# 服务始终启动（无 profiles 约束）
services:
  agent:
    build: { context: ../../, target: core }
    ports: ["8079:8079", "50051:50051"]

  service-hub:
    build: { context: ../../, dockerfile: services/service-hub/Dockerfile }
    depends_on: [agent]

# 仅在请求 monitoring profile 时启动
  prometheus:
    image: prom/prometheus:v2.50.0
    profiles: ["monitoring"]
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana:10.4.0
    profiles: ["monitoring"]
    ports: ["3000:3000"]
```

### 5.2 场景矩阵与启动命令

| 场景 | Profile 组合 | 启动命令 | 服务数量 |
|---|---|---|---|
| **Agent Only** | 默认 | `docker compose up -d agent` | 1 |
| **开发三件套** | 默认 | `docker compose up -d agent bff web` | 3 |
| **全栈微服务** | 默认 | `docker compose up -d` | 6 (Agent + 3 Go + BFF + Web) |
| **全栈 + 监控** | `monitoring` | `docker compose --profile monitoring up -d` | 8 (+ Prometheus + Grafana) |
| **全栈 + PostgreSQL** | `postgres` | `docker compose --profile postgres up -d` | 7 (+ PostgreSQL) |

### 5.3 `depends_on` 与健康检查联动

Compose 的 `depends_on` 仅控制启动顺序，不等待上游服务就绪。配合 `condition: service_healthy` 可实现真正的就绪等待：

```yaml
services:
  service-hub:
    depends_on:
      agent:
        condition: service_healthy    # 等待 agent 的 HEALTHCHECK 通过
    environment:
      - AGENT_URL=http://agent:8079

  agent:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8079/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s              # 宽限期：ML 模型加载可能需要较长时间
```

---

## 6. Helm Chart 模板引擎与 Values 参数体系 / Helm Template Engine & Values Architecture

### 6.1 Chart 目录结构

```text
deploy/helm/PrivShield/
├── Chart.yaml               # Chart 元数据（名称、版本、App 版本）
├── values.yaml              # 默认参数值（开发/测试环境）
├── values-production.yaml   # 生产环境参数覆盖
├── values-ml.yaml           # ML 镜像专用参数
└── templates/
    ├── _helpers.tpl          # 模板辅助函数（标签、名称、选择器）
    ├── deployment.yaml       # Deployment 资源定义
    ├── service.yaml          # Service 资源定义（双端口：REST + gRPC）
    ├── configmap.yaml        # ConfigMap（隐私配置 Profile 挂载）
    ├── secret.yaml           # Secret（TLS 证书 + API Key 挂载）
    ├── hpa.yaml              # HPA 弹性伸缩策略
    ├── serviceaccount.yaml   # ServiceAccount（RBAC 绑定）
    ├── ingress.yaml          # Ingress（可选，Nginx/Traefik 入口）
    └── NOTES.txt             # helm install 后的提示信息
```

### 6.2 核心 Values 参数详解

```yaml
# values.yaml — 关键参数节选

# 镜像配置
image:
  repository: privshield       # 镜像仓库名
  tag: "1.8.0"                 # 镜像标签（对应 App 版本）
  pullPolicy: IfNotPresent     # 拉取策略：Always / IfNotPresent / Never
  target: core                 # Docker 构建目标：core（轻量）或 ml（全量）

# 副本与弹性伸缩
replicaCount: 2                # 默认副本数
autoscaling:
  enabled: true                # 启用 HPA
  minReplicas: 2               # 最小副本数
  maxReplicas: 10              # 最大副本数
  targetCPUUtilization: 70     # CPU 利用率目标阈值
  targetMemoryUtilization: 80  # 内存利用率目标阈值

# 资源限制（生产环境必须设置）
resources:
  requests:
    cpu: "500m"                # 请求 0.5 核 CPU（调度保证）
    memory: "512Mi"            # 请求 512MB 内存
  limits:
    cpu: "2000m"               # 上限 2 核 CPU
    memory: "2Gi"              # 上限 2GB 内存（超过则 OOMKilled）

# 安全配置
security:
  tls:
    enabled: false
    existingSecret: ""         # TLS Secret 名称（含 tls.crt + tls.key）
  auth:
    enabled: false
    apiKeysSecret: ""          # API Key Secret 名称

# 隐私引擎配置
privacy:
  logLevel: "INFO"
  logFormat: "json"            # text 或 json（生产推荐 json）
  budgetDb: ""                 # SQLite 预算数据库路径（留空则内存模式）
  warmupLlm: false             # 启动时预热 LLM（生产建议 true）
```

### 6.3 `_helpers.tpl` 模板函数复用

Helm 模板使用 Go Template 语法。`_helpers.tpl` 定义可复用的命名与标签函数，确保所有资源的命名和标签保持一致：

```gotemplate
{{/*
生成完整的资源名称（Release 名 + Chart 名）
*/}}
{{- define "privshield.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
通用标签（所有资源统一附加）
*/}}
{{- define "privshield.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}
```

---

## 7. K8s 生产部署模式 / Kubernetes Production Deployment Patterns

### 7.1 双协议健康探针设计 / Dual-Protocol Health Probes

PrivShield 同时暴露 REST (:8079) 和 gRPC (:50051) 两个端口。K8s 探针需要分别覆盖：

```yaml
# livenessProbe — 存活探针（失败则重启容器）
livenessProbe:
  httpGet:
    path: /livez          # 轻量检查：进程是否存活
    port: rest
  initialDelaySeconds: 10  # 给 ML 模型加载预留启动时间
  periodSeconds: 15        # 每 15 秒检查一次
  failureThreshold: 3      # 连续 3 次失败才判定为不存活

# readinessProbe — 就绪探针（失败则从 Service 摘除流量）
readinessProbe:
  httpGet:
    path: /readyz          # 深度检查：规则库是否加载、预算 DB 是否可达
    port: rest
  initialDelaySeconds: 5
  periodSeconds: 5         # 每 5 秒检查（快速感知状态变化）
  failureThreshold: 2
```

**`/livez` vs `/readyz` 的设计区别**：
- `/livez` 仅检查进程是否响应 HTTP 请求，即便规则库尚未加载完毕也返回 200。目的是判断进程是否「假死」。
- `/readyz` 检查核心依赖是否就绪（规则引擎初始化、SQLite 数据库连通、可选的 LLM 模型加载状态）。未就绪时返回 503，K8s 会暂停向该 Pod 分发流量。

### 7.2 HPA 弹性伸缩策略 / Horizontal Pod Autoscaler

```yaml
# templates/hpa.yaml
{{- if .Values.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "privshield.fullname" . }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "privshield.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetCPUUtilization }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetMemoryUtilization }}
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300   # 缩容冷却 5 分钟，防止抖动
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60             # 每分钟最多缩容 1 个 Pod
{{- end }}
```

### 7.3 拓扑反亲和性调度 / Topology Anti-Affinity

在多副本部署中，应确保 Pod 分散调度到不同节点，避免单点故障：

```yaml
# templates/deployment.yaml — Pod 反亲和性
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: privshield
          topologyKey: kubernetes.io/hostname  # 尽量不在同一 Node 上
```

### 7.4 Secret 与 ConfigMap 安全挂载

生产环境中，TLS 证书和 API Key 不应硬编码在镜像或 Values 中，而应通过 K8s Secret 安全挂载：

```bash
# 创建 TLS Secret
kubectl create secret tls privshield-tls-secret \
  --cert=config/certs/server.crt \
  --key=config/certs/server.key

# 创建 API Key Secret
kubectl create secret generic privshield-apikeys-secret \
  --from-literal=api-keys='{"key-abc123": {"scopes": ["*"]}}'

# Helm 部署时引用
helm install privshield ./deploy/helm/PrivShield \
  --set security.tls.enabled=true \
  --set security.tls.existingSecret=privshield-tls-secret \
  --set security.auth.enabled=true \
  --set security.auth.apiKeysSecret=privshield-apikeys-secret
```

---

## 8. CI/CD 流水线集成 / CI/CD Pipeline Integration

### 8.1 GitHub Actions 示例

```yaml
# .github/workflows/ci.yml
ci:
  steps:
    - name: Build Docker Image (Core)
      run: |
        docker build --target core -t privshield:ci-${{ github.sha }} .
    
    - name: Run Tests in Container
      run: |
        docker run --rm privshield:ci-${{ github.sha }} \
          python -m pytest tests/ -q --tb=short
    
    - name: Push to Registry
      if: github.ref == 'refs/heads/main'
      run: |
        docker tag privshield:ci-${{ github.sha }} registry.example.com/privshield:${{ github.sha }}
        docker push registry.example.com/privshield:${{ github.sha }}
```

### 8.2 镜像标签命名规范

| 场景 | 标签格式 | 示例 |
|---|---|---|
| CI 临时构建 | `ci-<commit-sha>` | `ci-a1b2c3d4` |
| 开发版本 | `<version>-dev.<build>` | `1.8.0-dev.42` |
| 正式发布 | `<version>` | `1.8.0` |
| ML 镜像 | `<version>-ml` | `1.8.0-ml` |
| 最新稳定版 | `latest` | `latest` |

---

## 9. 常见问题排查与运维命令速查 / Troubleshooting & Operations Runbook

### 9.1 常用运维命令

```bash
# 查看 Pod 状态与事件
kubectl get pods -l app.kubernetes.io/name=privshield -o wide
kubectl describe pod <pod-name>

# 查看容器日志（含前一个崩溃容器的日志）
kubectl logs <pod-name> -c privshield --previous

# 进入容器调试（临时 Pod）
kubectl debug -it <pod-name> --image=busybox --target=privshield

# 检查健康探针状态
kubectl get pod <pod-name> -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'

# 查看 HPA 伸缩状态
kubectl get hpa privshield -o yaml

# 端口转发到本地调试
kubectl port-forward svc/privshield 8079:8079 50051:50051
```

### 9.2 常见故障排查

| 现象 | 可能原因 | 排查命令 |
|---|---|---|
| Pod 反复 CrashLoopBackOff | OOM（内存超限） | `kubectl describe pod` 查看 Last State: OOMKilled |
| Pod 一直 Pending | 资源不足或亲和性冲突 | `kubectl describe pod` 查看 Events |
| Readiness 探针持续失败 | 规则库未加载或 DB 不可达 | `kubectl logs` 查看启动日志 |
| Liveness 探针超时 | 进程假死或 GIL 阻塞 | 检查 `PRIVACY_LLM_MAX_CONCURRENCY` 设置 |
| 镜像构建极慢 | 缓存失效或上下文过大 | 检查 `.dockerignore` 和 COPY 顺序 |
| Helm 升级后 Pod 未更新 | 镜像标签未变化 | `kubectl rollout restart deployment/privshield` |

### 9.3 镜像体积优化清单

1. 使用 `python:3.13-slim` 而非 `python:3.13`（从 ~900MB 降至 ~150MB）
2. `pip install --no-cache-dir` 去除 pip 下载缓存
3. `apt-get install --no-install-recommends` 仅安装必要依赖
4. `rm -rf /var/lib/apt/lists/*` 清理 apt 缓存
5. 多阶段构建：ML 依赖仅在 `ml` 阶段安装，`core` 镜像不含 PyTorch
6. `.dockerignore` 排除测试、文档、前端依赖等运行时不需要的文件
