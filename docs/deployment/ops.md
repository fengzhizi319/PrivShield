# privacy-local-agent 部署运维手册

## 目录

- [1. 环境准备](#1-环境准备)
- [2. 镜像构建](#2-镜像构建)
  - [2.1 多阶段构建架构](#21-多阶段构建架构)
  - [2.2 构建命令](#22-构建命令)
  - [2.3 依赖清单](#23-依赖清单)
  - [2.4 镜像备份与分享（Backup & Share）](#24-backup-share)
- [3. Helm 部署](#3-helm-部署)
  - [3.1 Chart 结构](#31-chart-结构)
  - [3.2 默认安装（开发/测试）](#32-默认安装开发测试)
  - [3.3 生产安装（TLS + 认证 + HPA）](#33-生产安装tls--认证--hpa)
  - [3.4 ML 镜像部署](#34-ml-镜像部署)
  - [3.5 升级与回滚](#35-升级与回滚)
  - [3.6 卸载](#36-卸载)
- [4. 原生 K8s 部署](#4-原生-k8s-部署)
  - [4.1 资源清单](#41-资源清单)
  - [4.2 部署步骤](#42-部署步骤)
- [5. Docker Compose 部署](#5-docker-compose-部署)
  - [5.0 小白快速上手（前置条件 + 名词简释）](#50-小白快速上手前置条件--名词简释)
  - [5.1 Docker Compose 全栈服务编排](#51-docker-compose-全栈服务编排)
  - [5.2 服务启动流程详解](#52-服务启动流程详解)
    - [5.2.1 命令拆解](#521-命令拆解)
    - [5.2.2 完整执行流程](#522-完整执行流程)
    - [5.2.3 实际解析结果](#523-实际解析结果)
    - [5.2.4 环境变量三来源机制](#524-环境变量三来源机制)
    - [5.2.5 多环境修改环境变量的建议](#525-多环境修改环境变量的建议)
    - [5.2.6 vllm 容器启动细节](#526-vllm-容器启动细节)
    - [5.2.7 启动后的 core ↔ vllm 联动](#527-启动后的-core--vllm-联动)
    - [5.2.8 compose 命令的实现位置与 restart/up 区别](#528-compose-命令的实现位置与-restartup-区别)
    - [5.2.9 --profile 从哪里找服务（小白澄清）](#529---profile-从哪里找服务小白澄清)
    - [5.2.10 镜像构建策略：不是每次 up 都构建（小白澄清）](#5210-镜像构建策略不是每次-up-都构建小白澄清)
    - [5.2.11 llm 网络在哪里配置、由谁实现（小白澄清）](#5211-llm-网络在哪里配置由谁实现小白澄清)
  - [5.3 自动化 Docker 脚本运行集](#53-自动化-docker-脚本运行集)
  - [5.4 Docker Compose 常用命令速查](#54-docker-compose-常用命令速查)
  - [5.5 常见配置需求速查（想改 X → 改哪里）](#55-常见配置需求速查想改-x--改哪里)
- [6. LLM 推理服务部署（集成模式 / 解耦模式）](#6-llm-推理服务部署集成模式--解耦模式)
- [7. 安全配置](#7-安全配置)
  - [7.1 TLS / mTLS](#71-tls--mtls)
  - [7.2 API Key 认证](#72-api-key-认证)
  - [7.3 速率限制](#73-速率限制)
- [8. 服务启动与优雅关闭](#8-服务启动与优雅关闭)
- [9. 健康检查与探针](#9-健康检查与探针)
- [10. 监控与告警](#10-监控与告警)
  - [10.1 Prometheus 指标](#101-prometheus-指标)
  - [10.2 告警规则](#102-告警规则)
  - [10.3 Grafana 仪表盘](#103-grafana-仪表盘)
  - [10.4 ServiceMonitor（Prometheus Operator）](#104-servicemonitorprometheus-operator)
- [11. 自动伸缩（HPA）](#11-自动伸缩hpa)
- [12. 网络策略（NetworkPolicy）](#12-网络策略networkpolicy)
- [13. 配置体系与环境变量参考](#13-配置体系与环境变量参考)
  - [13.1 配置体系总览](#131-配置体系总览)
  - [13.2 环境变量参考](#132-环境变量参考)
- [14. 验证与冒烟测试](#14-验证与冒烟测试)
- [15. 故障排查](#15-故障排查)
  - [15.1 Docker Compose 常见故障](#151-docker-compose)
    - [15.1.1 vLLM Docker 服务测试失败排查案例](#1511-vllm-docker-服务测试失败排查案例)
  - [15.2 WSL 中 Docker GPU 失效（真实排查案例）](#152-wsl-docker-gpu)
- [16. 日常运维操作](#16-日常运维操作)

---

## 1. 环境准备

| 组件 | 最低版本 | 用途 |
|---|---|---|
| Kubernetes | >= 1.25 | 容器编排（HPA v2、NetworkPolicy v1） |
| Helm | >= 3.12 | Chart 安装与管理 |
| Docker | >= 20.10 | 镜像构建（BuildKit 多阶段） |
| Docker Compose | >= 2.0 | 本地联调 |
| Prometheus Operator | 可选 | ServiceMonitor 自动发现 |
| Grafana | >= 9.0 | 可视化仪表盘 |

**Python 运行时**（仅本地开发需要）：Python >= 3.10。

---

## 2. 镜像构建

### 2.1 多阶段构建架构

Dockerfile 采用三阶段构建，通过 `--target` 选择最终镜像：

```text
base (python:3.13.13-slim-bookworm)
 ├── 安装 curl / ca-certificates（K8s 探针依赖）
 ├── 安装 requirements-core.txt（核心运行时依赖）
 │
 ├──► core 目标
 │     ├── COPY 全部源码
 │     ├── EXPOSE 8079 50051
 │     ├── ENV PRIVACY_REST_HOST=0.0.0.0 / PRIVACY_GRPC_HOST=0.0.0.0
 │     └── CMD python -m privacy_local_agent.server
 │
 └──► ml 目标（继承 core）
       ├── 安装 requirements-ml.txt（torch/transformers/onnxruntime 等）
       └── CMD python -m privacy_local_agent.server
```

- **core 镜像**（~350 MB）：仅含隐私原语（DP / K-匿名 / 脱敏 / 规则分类），适合绝大多数生产场景。
- **ml 镜像**（~4 GB）：额外包含 PyTorch / Transformers / ONNX Runtime，用于本地 NER（Layer-2）和 VLM/LLM（Layer-3）分类。

### 2.2 构建命令

```bash
# 在仓库根目录执行
# core 镜像（推荐生产默认）
docker build --target core -t PrivShield:0.1.0 .

# ml 镜像（含 torch/transformers/onnxruntime，用于完整三层分类）
docker build --target ml -t PrivShield:0.1.0-ml .

# 也可使用 Makefile 快捷命令
make docker-core          # 等价于 --target core
make docker-ml            # 等价于 --target ml
```

> 自定义版本号：`make docker-core VERSION=0.2.0`

### 2.3 依赖清单

**core 运行时依赖**（`requirements-core.txt`）：

| 包 | 版本约束 | 用途 |
|---|---|---|
| fastapi | >= 0.110.0 | REST 框架 |
| uvicorn[standard] | >= 0.27.0 | ASGI 服务器 |
| pydantic | >= 2.6.0 | 数据校验 |
| grpcio | >= 1.62.0, < 2.0.0 | gRPC 通信 |
| protobuf | >= 4.25.0, < 8.0.0 | 序列化 |
| pyyaml | >= 6.0.1 | Profile 配置解析 |
| httpx | >= 0.27.0 | 网关代理 HTTP 客户端 |
| limits | >= 3.10.0 | 速率限制 |
| cryptography | >= 42.0.0 | TLS / 加密 |
| python-json-logger | >= 2.0.7 | 结构化日志 |
| prometheus-client | >= 0.20.0 | 指标暴露 |
| numpy / pandas / pyarrow | — | 向量化规则引擎 |

**ml 扩展依赖**（`requirements-ml.txt`，在 core 基础上追加）：

| 包 | 版本约束 | 用途 |
|---|---|---|
| torch | >= 2.2.0 | 深度学习推理 |
| transformers | >= 4.45.0 | 模型加载 |
| accelerate | >= 0.30.0 | 模型加速 |
| onnxruntime | >= 1.17.0 | NER ONNX 推理 |
| modelscope | >= 1.20.0 | ModelScope NER 管道 |
| datasets | >= 4.0.0, <= 4.8.4 | ModelScope 运行时依赖 |

### 2.4 镜像备份与分享（Backup & Share）

**核心概念**：镜像的"真名"是 `IMAGE ID`（sha256），`tag` 只是指针/别名，**一个镜像可以同时有多个 tag**。`latest` 是官方仓库的滚动标签，会随官方更新变化。

**命名规范**：保留官方 tag 不动（`docker-compose.yml`、`docker-start-llm.sh`、测试都引用它），另加自定义 tag 记录存档/交付版本，格式 `<项目>-<日期>` 或 `<版本号>`：

```bash
# 两个 tag 指向同一个 IMAGE ID，docker images 会显示两行
docker tag vllm/vllm-openai:latest vllm/vllm-openai:pla-20260813
```

**方式一：离线导出 / 导入**（U 盘、网盘、内网传输）

```bash
# 导出（本机实测：vllm 镜像 8.5G，保存约 1 分钟）
docker save vllm/vllm-openai:latest -o ~/vllm-image.tar

# 导入（目标机器上，8.5G 加载约 1~2 分钟）
docker load -i ~/vllm-image.tar
```

> **踩坑**：保存路径必须选磁盘空间充足的分区。`/tmp` 通常是 tmpfs（内存盘），本机仅 7.7G < 镜像 8.5G，`docker save` 直接报 `no space left on device`；保存到家目录（磁盘分区）即可。

**方式二：推送到镜像仓库**（多人/多机协作）

```bash
# 1. 打上仓库前缀 tag（myregistry 换成 Docker Hub 用户名或私有仓库地址）
docker tag PrivShield:0.1.0 myregistry/PrivShield:0.1.0

# 2. 登录并推送
docker login
docker push myregistry/PrivShield:0.1.0

# 3. 目标机器拉取
docker pull myregistry/PrivShield:0.1.0
```

> 国内网络 pull 官方镜像慢时，可用 daemon.json 里配置的 registry-mirrors（如 `docker.m.daocloud.io`）加速，或从镜像站/私有仓库导入。

**本项目注意**：

- `docker-compose.yml`、`docker-start-llm.sh`、集成测试都引用 `vllm/vllm-openai:latest`，改 tag 名需同步修改所有引用处。
- 换 daemon（如 snap docker → docker.io）后原镜像不互通，必须 `docker save` + `docker load` 迁移（详见 15.2 节）。

---

## 3. Helm 部署

### 3.1 Chart 结构

```text
deploy/helm/privacy-local-agent/
├── Chart.yaml                  # Chart 元数据（version: 0.1.0, appVersion: 0.1.0）
├── values.yaml                 # 默认 values（开发模式，TLS/Auth 关闭）
├── values-production.yaml      # 生产覆盖值（TLS/Auth/HPA/NetworkPolicy 开启）
├── values-ml.yaml              # ML 镜像覆盖值（资源上限提升）
└── templates/
    ├── configmap.yaml          # privacy-profile.yaml 配置注入
    ├── deployment.yaml         # 主 Deployment（含探针、安全上下文、TLS 挂载）
    ├── hpa.yaml                # HorizontalPodAutoscaler（autoscaling/v2）
    ├── ingress.yaml            # 可选 Ingress
    ├── namespace.yaml          # 可选 Namespace 创建
    ├── networkpolicy.yaml      # 可选 NetworkPolicy
    ├── poddisruptionbudget.yaml # 可选 PodDisruptionBudget
    ├── secret.yaml             # 内置 Secret（TLS 证书 / API Key）
    ├── service.yaml            # ClusterIP Service（REST 8079 + gRPC 50051）
    ├── serviceaccount.yaml     # ServiceAccount
    └── servicemonitor.yaml     # Prometheus Operator ServiceMonitor
```

### 3.2 默认安装（开发/测试）

```bash
helm install pla ./deploy/helm/PrivShield
```

默认配置要点：
- `replicaCount: 1`，单副本
- `flavor: core`，使用轻量镜像
- TLS / Auth / RateLimit 均关闭
- 资源：requests 100m CPU / 256Mi，limits 1000m CPU / 1Gi
- 探针：liveness `/health`（10s 间隔），readiness `/readyz`（5s 间隔，额外校验配置解析器与预算 DB 连通性）
- HPA / Ingress / NetworkPolicy / ServiceMonitor 均关闭

### 3.3 生产安装（TLS + 认证 + HPA）

```bash
# 1. 准备 TLS Secret（包含 tls.crt 和 tls.key）
kubectl create secret tls pla-tls \
  --cert=path/to/tls.crt --key=path/to/tls.key \
  -n PrivShield

# 2. 准备 API Key Secret（包含 api-keys.json 文件）
# api-keys.json 格式示例：
# {
#   "my-api-key": { "name": "gateway", "scopes": ["*"] },
#   "readonly-key": { "name": "auditor", "scopes": ["read"] }
# }
kubectl create secret generic pla-apikeys \
  --from-file=api-keys.json=path/to/api-keys.json \
  -n PrivShield

# 3. 安装（使用生产 values 覆盖）
helm install pla ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=pla-tls \
  --set security.auth.apiKeysSecret=pla-apikeys \
  --set image.repository=myregistry/PrivShield \
  --set image.tag=0.1.0
```

**生产 values 关键差异**（`values-production.yaml`）：

| 配置项 | 默认值 | 生产值 |
|---|---|---|
| replicaCount | 1 | 2 |
| agent.logFormat | text | json |
| security.tls.enabled | false | true |
| security.auth.enabled | false | true |
| security.rateLimit.enabled | false | true |
| resources.requests | 100m / 256Mi | 500m / 512Mi |
| resources.limits | 1000m / 1Gi | 2000m / 2Gi |
| autoscaling.enabled | false | true（2~10 副本） |
| networkPolicy.enabled | false | true |
| serviceMonitor.enabled | false | true |

### 3.4 ML 镜像部署

```bash
helm install pla-ml ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-ml.yaml \
  --set image.repository=myregistry/PrivShield \
  --set image.tag=0.1.0-ml
```

**ML values 关键差异**（`values-ml.yaml`）：

| 配置项 | 默认值 | ML 值 |
|---|---|---|
| flavor | core | ml |
| resources.requests | 100m / 256Mi | 1000m / 2Gi |
| resources.limits | 1000m / 1Gi | 4000m / 8Gi |
| autoscaling | 关闭 | 开启（1~3 副本） |

> ML 镜像包含 PyTorch + Transformers，内存占用显著增大，建议节点至少 16Gi 可用内存。

### 3.5 升级与回滚

```bash
# 升级（修改 values 或镜像版本后）
helm upgrade pla ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set image.tag=0.2.0

# 查看历史版本
helm history pla

# 回滚到上一版本
helm rollback pla

# 回滚到指定版本
helm rollback pla 2
```

### 3.6 卸载

```bash
helm uninstall pla

# 如需同时清理 PVC / Secret 等手动创建的资源
kubectl delete secret pla-tls pla-apikeys -n PrivShield
```

---

## 4. 原生 K8s 部署

### 4.1 资源清单

`deploy/k8s/` 目录包含以下资源（通过 Kustomize 管理）：

| 文件 | Kind | 说明 |
|---|---|---|
| `namespace.yaml` | Namespace | 创建 `PrivShield` 命名空间 |
| `configmap.yaml` | ConfigMap | 注入 `privacy-profile.yaml` 配置 |
| `deployment.yaml` | Deployment | 主工作负载（含探针、资源限制） |
| `service.yaml` | Service | ClusterIP，暴露 8079（REST）+ 50051（gRPC） |
| `secret.example.yaml` | Secret | TLS 证书 + API Key 示例（需复制修改） |
| `kustomization.yaml` | Kustomization | 资源编排入口 |

### 4.2 部署步骤

```bash
# 1. 准备 Secret（复制示例并填入真实证书/密钥）
cp deploy/k8s/secret.example.yaml deploy/k8s/secret.yaml
# 编辑 secret.yaml，替换 REPLACE_WITH_YOUR_CERT / REPLACE_WITH_YOUR_KEY

# 2. 如需启用 Secret，取消 kustomization.yaml 中的注释：
#    resources:
#      - secret.yaml

# 3. 一键部署
kubectl apply -k deploy/k8s/

# 4. 验证
kubectl get pods -n PrivShield
kubectl get svc -n PrivShield
```

**Deployment 关键配置说明**：

```yaml
# 容器端口
ports:
  - name: http
    containerPort: 8079    # REST API
  - name: grpc
    containerPort: 50051   # gRPC

# 探针配置
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /readyz
    port: http
  initialDelaySeconds: 5
  periodSeconds: 5

# 资源限制
resources:
  requests: { cpu: 100m, memory: 256Mi }
  limits:   { cpu: 1000m, memory: 1Gi }

# 配置挂载
volumeMounts:
  - name: config
    mountPath: /etc/PrivShield
    readOnly: true
```

---

## 5. Docker Compose 与 Docker 自动化部署

适用于本地快速联调、完整容器化测试及单机一键部署。

### 5.0 小白快速上手：第一次跑通全栈

> 本节给第一次接触 Docker 的读者一条完整路径，每步的命令与"为什么"都给出说明。已熟悉的读者可跳过。

**第 0 步：前置条件检查**（首次使用前）：

| 项 | 要求 | 如何验证 |
|---|---|---|
| Docker | Docker Desktop（Windows/Mac）或 Docker Engine（Linux） | `docker --version` |
| Docker Compose | compose 插件或独立二进制二选一 | `docker compose version` 或 `docker-compose version` |
| 国内网络（可选） | 配置镜像加速或代理 | 拉镜像超时见 [6.2](#62-解耦模式外部独立-vllm-服务) 的备选方案 |
| NVIDIA GPU + nvidia-container-toolkit | 仅跑 vllm 需要；无 GPU 则跳过 `--profile llm` | `nvidia-smi` |

**第 1 步：启动核心服务**

```bash
cd deploy/docker-compose
docker compose up -d
```

首次运行会自动**构建** agent/console 镜像并**拉取**基础镜像，需要几分钟属正常现象。

**第 2 步：确认服务就绪**

```bash
docker compose ps              # 状态应为 Up (healthy)
curl http://localhost:8079/health   # 期望：{"status":"ok",...}
```

**第 3 步：验证功能**

浏览器打开 `http://localhost:5173`（Web 控制台）；或用 [14.3](#143-docker-compose-验证) 的 curl 冒烟命令直接测脱敏/DP 接口。

**第 4 步：停止服务**

```bash
docker compose down        # 停止并移除容器/网络（数据卷保留，预算/日志还在）
docker compose down -v     # 连数据卷一起删（慎用！预算与日志会丢失）
```

**名词简释**（第一次接触时先看这个）：

| 名词 | 一句话解释 | 本项目的例子 |
|---|---|---|
| 镜像 image | 打包好的"安装包"（只读模板），由 Dockerfile 构建 | `privacy-local-agent:0.1.0`、`vllm/vllm-openai:latest` |
| 容器 container | 镜像的运行实例，一个隔离的进程环境 | 7 个服务对应 7 个容器 |
| 服务 service | compose 文件里对一组容器的声明 | `PrivShield`、`vllm` |
| 网络 network | 容器间的虚拟局域网，内部按服务名 DNS 互访 | `backend` / `llm` / `frontend` |
| 卷 volume | 容器外的持久化存储（容器删除数据仍在） | `budget-db`、`audit-logs` |
| profile | compose 的"开关"，按需启停可选服务 | `--profile llm` 才启动 vllm |
| healthcheck | 容器自检命令，供依赖方等待就绪 | agent 探测 `/readyz` |
| build | 从源码构建镜像的配置（有 build 段则本地构建而非拉取） | agent/console 三个服务 |

### 5.1 Docker Compose 全栈服务编排

`deploy/docker-compose/docker-compose.yml` 提供了涵盖 Agent、双 Console 代理后端、React Web UI 及 vLLM 大模型的完整服务编排：

| 服务组件 | 镜像 / 构建目标 | 容器端口 | 功能说明 |
|---|---|---|---|
| `PrivShield` | `Dockerfile` (`target: core`) | 8079 (REST) / 50051 (gRPC) | 隐私 Agent 核心 Sidecar 服务 |
| `console-backend-go` | `console/backend-go/Dockerfile` | 8081 | Go gRPC 高性能代理后端 |
| `console-backend-python` | `console/backend/Dockerfile` | 8080 | Python FastAPI REST 代理后端 |
| `console-web` | `console/web/Dockerfile` | 5173 | React 单页控制台 Nginx 静态服务 |
| `vllm` | `vllm/vllm-openai:${VLLM_IMAGE_TAG:-latest}` (profile: `llm`) | 8000 | vLLM Layer-3 本地大模型推理（GPU；Qwen3.5 混合注意力架构需 vLLM 0.26+） |

```bash
cd deploy/docker-compose

# 启动核心服务套件 (Agent + Go/Python 后端 + Web UI)
docker compose up -d

# 启动包含 vLLM 大模型 GPU 推理容器的全栈服务
docker compose --profile llm up -d
```

### 5.2 服务启动流程详解（以 `docker compose --profile llm up -d` 为例）

本节面向不熟悉 Compose 的读者，完整讲解一条启动命令从输入到服务就绪的全过程。

#### 5.2.1 命令拆解

| 命令片段 | 作用 |
|---|---|
| `docker` | Docker CLI 入口 |
| `compose` | 进入 Compose 子命令（若 docker 未安装 compose 插件，可用独立二进制 `docker-compose` 等价替代） |
| `--profile llm` | 启用 `llm` profile：仅启动带 `profiles: ["llm"]` 的服务（vllm）+ 所有不带 profiles 的服务 |
| `up` | 创建并启动服务（自动创建网络/卷/镜像） |
| `-d` | detached：容器后台运行，命令完成后即返回 |

> 需在 `deploy/docker-compose/` 目录执行（compose 默认在此目录查找 `docker-compose.yml`）；也可用 `-f deploy/docker-compose/docker-compose.yml` 从任意目录指定。

#### 5.2.2 完整执行流程

```text
┌───────────────────────── 解析阶段（生成最终配置） ─────────────────────────┐
│ ① 定位文件      CWD 查找 docker-compose.yml（即 deploy/docker-compose/）    │
│ ② 变量替换      ${VLLM_IMAGE_TAG:-latest} → latest（CWD 无 .env → 用默认值）│
│ ③ YAML 解析     校验 services/networks/volumes 结构                        │
│ ④ Profile 过滤  启用 llm → 5 个服务（无 profile 的 4 个 + vllm）            │
│ ⑤ 依赖图构建    console-* 依赖 privacy-local-agent（condition: healthy）    │
│ ⑥ 路径规范化    build context、bind mount 源转为绝对路径                    │
│ ⑦ env_file 合并 agent 的 ../../.env 展开合并进容器 environment             │
└───────────────────────────────────────────────────────────────────────────┘
┌───────────────────────── 执行阶段（up -d） ───────────────────────────────┐
│ ⑧ 创建网络/卷   3 网络 + 2 卷（名称加项目前缀，如 docker-compose_llm）      │
│ ⑨ 镜像准备      vllm 从 Docker Hub 拉取；agent/console 有 build 段→本地构建 │
│ ⑩ 启动容器      按依赖拓扑：agent 先启动，vllm 无依赖可并行                 │
│ ⑪ 健康检查      agent /readyz 通过后，console-* 才启动（service_healthy）   │
│ ⑫ DNS 就绪      网络内以服务名互访（vllm:8000、privacy-local-agent:8079）  │
└───────────────────────────────────────────────────────────────────────────┘
```

#### 5.2.3 实际解析结果（`docker-compose --profile llm config` 实测）

| 验证点 | 结果 |
|---|---|
| 无 profile 的服务 | `PrivShield`、`console-backend-go`、`console-backend-python`、`console-web`（4 个） |
| `--profile llm` 追加 | + `vllm`（共 5 个） |
| vllm 镜像 | `vllm/vllm-openai:latest`（`${VLLM_IMAGE_TAG:-latest}` 替换生效） |
| env_file 合并 | agent 容器环境含根目录 `.env` 的值：`PRIVACY_ENV_PROFILE=qwen3`、`PRIVACY_LOG_LEVEL=INFO`、`PRIVACY_REST_PORT=8079` 等 |
| 网络 | `docker-compose_backend`（internal）、`docker-compose_llm`（非 internal，支持宿主机调试端口映射）、`docker-compose_frontend` |
| 挂载 | 根目录 `.models` → `/models`（只读）；`privacy-profile.yaml`（只读） |

#### 5.2.4 环境变量三来源机制（易混淆，重点）

`docker-compose.yml` 中所有环境变量来自三个独立机制，理解它们才能知道"改哪里生效"：

| 机制 | 作用对象 | 是否依赖 `.env` | 生效时机 |
|---|---|---|---|
| `environment` 段硬编码 | agent 8 个核心变量（监听地址、LLM provider/模型名/Key）、双 console 后端全部变量、grafana 的 GF_* | ❌ 不依赖（写死在 yml） | 容器创建时 |
| `env_file` 注入 | 仅 `PrivShield`（`../../.env` → 根目录 `.env`） | ✅ 唯一依赖点（`required: false`） | 容器创建时 |
| `${VAR:-default}` 变量替换 | `vllm` 镜像 tag、`grafana` 密码、agent 的 `LLM_API_BASE` 与 `LLM_API_KEY`（默认 `http://vllm:8000/v1` 和 `EMPTY`，跨主机部署时覆盖为 GPU 主机端点与鉴权密钥） | ⚠️ 解析时查 **CWD**（`deploy/docker-compose/.env`），与根目录 `.env` 无关 | 解析阶段 |

**优先级**：`environment` > `env_file` > 镜像内默认值（Dockerfile ENV/CMD）。

按服务归类：

| 服务 | 环境变量来源 |
|---|---|
| `PrivShield` | env_file（根目录 `.env` 其余项）+ environment 覆盖 8 项（容器必须 0.0.0.0、LLM provider 等）+ `${LLM_API_BASE}` / `${LLM_API_KEY}` 变量替换（跨主机部署用） |
| `console-backend-go` / `console-backend-python` / `console-web` | 全部 environment 硬编码（compose 内部 DNS 服务名，跨环境不变） |
| `vllm` | 无环境变量；仅 `image` 的 `${VLLM_IMAGE_TAG:-latest}` 编排替换 |
| `grafana` | GF_* 硬编码 + `${GRAFANA_ADMIN_PASSWORD:-changeme}` 替换 |

> **结论**：删掉根目录 `.env`，`docker compose up -d` 依然能启动全部服务（硬编码值 + 镜像默认值兜底）。`.env` 不是编排必需，而是 agent **业务参数**的运维主控入口（LLM provider、预算、TLS/Auth 等 8 大类 230 项）。

> **运维提示**：`deploy/docker-compose/` 目录下没有 `.env`，所以 `${VLLM_IMAGE_TAG:-latest}` 等替换总是用默认值。想固定 vLLM 镜像版本，需在 `deploy/docker-compose/.env` 写入 `VLLM_IMAGE_TAG=v0.26.x`，或执行时临时指定：`VLLM_IMAGE_TAG=v0.26.x docker compose --profile llm up -d`。**只修改根目录 `.env` 不会影响变量替换**——根目录 `.env` 仅经 `env_file` 注入容器运行时。

#### 5.2.5 多环境修改环境变量的建议（不用每个文件都改）

**结论先行：不麻烦**。环境差异项其实收敛在极少数文件，compose 里"写死的"多是环境无关的固定值。

1. **分清"固定值"与"环境差异值"**

   - compose 硬编码项（监听地址 `0.0.0.0`、`PrivShield`/`vllm` 服务名、端口）是**环境无关的编排固定值**——本地/测试/生产容器里都相同，无需改动；只有跨主机部署（agent 在别的主机、LLM 用云 API）才需要动
   - 真正因环境而异的项：LLM provider、日志级别、预算窗口、TLS/Auth 开关、镜像 tag、Grafana 密码等

2. **90% 的场景只改一个文件：根目录 `.env`**

   - agent 容器运行时行为（`PRIVACY_ENV_PROFILE`、`PRIVACY_LOG_LEVEL`、`PRIVACY_BUDGET_*`、TLS/Auth…）→ 改根目录 `.env` 即可，经 `env_file` 自动注入
   - 编排值（vLLM tag、Grafana 密码）→ 改 `deploy/docker-compose/.env`（仅 2 处，均有默认值兜底，不写也不影响启动）

3. **多套环境并存的三种方案**（按复杂度递增）

   | 方案 | 做法 | 适用场景 |
   |---|---|---|
   | A. 多 `.env` 文件切换 | 复制根目录 `.env` 为 `.env.dev` / `.env.prod`，用 `docker compose --env-file .env.prod up -d` 指定 | 环境数量少（2~3 套） |
   | B. 变量替换 + 单 `.env` | compose 中可变项写成 `${VAR:-default}`，各环境在 `deploy/docker-compose/.env` 覆盖 | 差异项集中在编排层 |
   | C. compose override | 新增 `docker-compose.override.yml` 只写差异项，compose 自动合并；生产用 `-f` 显式指定多文件 | 差异项多、需复用同一份主文件 |

   方案 C 示例（`docker-compose.override.yml` 与 `docker-compose.yml` 同目录，`up` 时自动合并，无需改主文件）：

   ```yaml
   services:
     privacy-local-agent:
       environment:
         PRIVACY_LOG_LEVEL: "DEBUG"
   ```

4. **真正需要"多处同步改"的场景**：跨主机部署（agent/后端/vLLM 分处不同机器或云 API）——需同步改 compose 的地址项 + `.env` 业务项。此类多环境场景建议直接走 K8s/Helm（`deploy/k8s/`、`deploy/helm/`），环境差异收敛到 `values.yaml` 一个文件（见第 3、4 章）。

#### 5.2.6 vllm 容器启动细节

1. **GPU 注入**：`deploy.resources.reservations.devices`（driver: nvidia、count: 1、capabilities: [gpu]）——要求宿主机安装 NVIDIA 驱动 + `nvidia-container-toolkit`
2. **启动参数**（`command` 覆盖镜像默认 CMD）：`--model /models/Qwen3.5-0.8B-Privacy-Classifier-Smoother` → `--served-model-name`（须与 agent 的 `PRIVACY_LLM_MODEL_NAME` 一致）→ `--trust-remote-code` → `--gpu-memory-utilization 0.85` → `--max-model-len 4096` → `--host 0.0.0.0 --port 8000`
3. **模型加载**：从只读挂载的根目录 `.models/` 读取权重（Qwen3.5 为混合注意力架构，需 vLLM 0.26+，故默认 `latest`）
4. **健康检查**：容器内 `python3` 探测 `http://127.0.0.1:8000/health`，`start_period: 60s`（首次加载模型约 22s+，期间失败不计入 retries）。**注意**：vllm 官方镜像基于 Ubuntu 24.04，只有 `python3` 没有 `python`，健康检查命令务必写成 `python3`，否则容器会一直处于 `unhealthy`。
5. **端口映射**：vllm 容器存在**两条互不干扰的端口通道**，理解它们的区别是改端口的前提——

   | 通道 | 定义位置 | 访问方 | 端口值 |
   |---|---|---|---|
   | ① 容器内监听 | `command:` 段的 `--host 0.0.0.0 --port 8000` | 同网络内其他容器（agent） | 容器内 8000 |
   | ② 宿主机映射 | `ports:` 段的 `"127.0.0.1:8000:8000"` | 宿主机调试 / 压测 | 宿主 8000 → 容器 8000 |

   - agent 容器经内部 `llm` 网络以服务名直连 `http://vllm:8000/v1`（即 `PRIVACY_LLM_API_BASE`），**走的是通道①，不经过宿主机端口映射**——通道②仅为宿主机直接访问 vLLM（`curl localhost:8000`、压测）而设
   - **关键坑点**：在 Docker Compose 中，若服务只 attached 到 `internal: true` 的网络，即使写了 `ports:` 也不会创建宿主机端口映射，`docker inspect` 会显示 `"8000/tcp": null`，宿主机 `curl 127.0.0.1:8000` 会无响应。因此 `llm` 网络必须设为 `internal: false`（或让 vllm 同时属于一个非 internal 网络），才能通过 `ports:` 暴露调试端口。
   - 映射格式 `"宿主机IP:宿主机端口:容器端口"`（省略 IP 表示绑定宿主机所有网卡；如 `"127.0.0.1:8000:8000"` 仅回环可访问，更安全）

   两条通道的拓扑关系（core 与 vllm 是**两个独立容器**，互连不依赖端口映射）：

   ```text
   ┌─────────────────────────── llm 网络（bridge） ────────────────────────────┐
   │ 通道① 容器间直连（不走 ports）：                                          │
   │   privacy-local-agent ──DNS: "vllm" → 容器IP──▶ vllm:8000（容器内监听）   │
   │                                                                          │
   │ 通道② 宿主机访问（走 ports）：                                             │
   │   宿主机 curl 127.0.0.1:8000 ──端口映射──▶ vllm 容器:8000                │
   └──────────────────────────────────────────────────────────────────────────┘
   ```

   **容器间互连成立的三前提**（缺一不可）：

   | 前提 | 配置位置 | 说明 |
   |---|---|---|
   | ① core 与 vllm 在**同一网络** | 两服务 `networks:` 均含 `llm` | 不在同网络则 core 内 DNS 解析不到 `vllm` |
   | ② vllm **容器内监听 0.0.0.0** | vllm `command:` 的 `--host 0.0.0.0` | 若监听 127.0.0.1 只接受自身回环请求，其他容器一律连不上 |
   | ③ core 用**服务名**访问 | `PRIVACY_LLM_API_BASE: http://vllm:8000/v1` | 容器 IP 会变，服务名由 Docker DNS 自动解析 |

   **只改对外端口（推荐做法）**：宿主机 8000 被占用或想换对外端口时，只改 `ports:` 左侧即可，agent 内部调用完全不受影响：

   ```yaml
   ports:
     - "9000:8000"   # 宿主机 9000 → 容器 8000（右侧容器端口不能动，动了要同步三处，见下）
   ```

   改完必须 `docker compose up -d vllm` **重建容器**才生效（`docker compose restart` 不重新解析 yml，配置变更不生效，见 5.2.8）。验证：

   ```bash
   docker compose port vllm 8000          # → 输出 0.0.0.0:9000，即映射生效
   curl http://localhost:9000/v1/models   # → 返回模型列表（含 Qwen3.5-...-Smoother）
   ```

   **如需连容器端口一起改**（极端场景，如内部端口也想换），必须同步以下 **3 处**，漏一处就断连：

   | 同步位置 | 示例改动 |
   |---|---|
   | ① `command:` 段 `--port` | `--port 8000` → `--port 9000`（vLLM 进程监听新端口） |
   | ② `ports:` 段右侧 | `"8000:9000"`（宿主端口 → 新容器端口） |
   | ③ agent 的 `PRIVACY_LLM_API_BASE` | `http://vllm:8000/v1` → `http://vllm:9000/v1`（**不改成 agent 连不上 vLLM**） |

   > **踩坑**：daemon 29.x 的 `docker inspect` 中 PortBindings 可能显示 `{invalid IP 8000}`，这是**显示层 bug**，不代表端口发布失败——以 `docker port <容器名>` / `ss -ltn | grep 8000` 为准。WSL2 下 WSL 内端口未监听时，`curl localhost:<port>` 会被转发到 Windows 侧返回 502（连不上而非拒绝），排查时先确认 `ss` 有监听再 curl。

#### 5.2.7 启动后的 core ↔ vllm 联动

- agent 容器内 `PRIVACY_LLM_PROVIDER=vllm` + `PRIVACY_LLM_API_BASE=http://vllm:8000/v1`（compose `${LLM_API_BASE:-http://vllm:8000/v1}` 默认值，跨主机部署可经 `deploy/docker-compose/.env` 的 `LLM_API_BASE` 覆盖，见 5.2.11 ④），经 `llm` 网络（专供 core ↔ vllm，现已设为非 internal 以支持宿主机调试端口映射）以服务名 `vllm` 解析
- 分类请求进入 Layer-3 时，agent 以 OpenAI 兼容 HTTP 调用 vllm
- vllm 挂掉/OOM → agent 自动降级 Layer-1 规则 + Layer-2 NER，REST/gRPC 不受影响（运行时解耦）
- 升级 vllm 只需 `docker compose up -d vllm`，core 容器无需重建

**core 与 vllm 不在同一网络（独立部署）时的替代方案**：

| 场景 | 方案 | 配置要点 |
|---|---|---|
| 同机、不同部署（无共享网络） | A. 建共享网络（推荐，等同 compose 效果） | `docker network create llm-net`；vllm 启动加 `--network llm-net --network-alias vllm`；core 加 `--network llm-net`，`PRIVACY_LLM_API_BASE=http://vllm:8000/v1` 不变，仍走内部直连 |
| 同机、不同部署 | B. 走宿主机端口映射 | vllm `ports:` 绑定 `0.0.0.0:8000:8000`（**不能绑 127.0.0.1**，否则其他容器访问不到）；core 容器内写 `http://host.docker.internal:8000/v1` + `extra_hosts: ["host.docker.internal:host-gateway"]` |
| 跨主机部署 | 只能走映射 + 实际 IP（多方案对比见 5.2.11 ④） | vllm 映射 `0.0.0.0:8000:8000`；core 写 `http://<vllm主机IP>:8000/v1`（`host.docker.internal` 跨主机失效，需写实际 IP） |

> **坑点**：core 容器内 `127.0.0.1` 指的是 **core 容器自己**，不是宿主机；容器内访问宿主机必须用 `host.docker.internal`（Linux 需 `extra_hosts` 或 `--network host`）或宿主机实际 IP。

#### 5.2.8 compose 命令的实现位置与 `restart`/`up` 区别

**关键认知**：`docker compose restart / up / down` 等命令的**代码不在项目仓库中**——它们由 Docker Compose 程序本身内置（本机为 snap 安装的独立二进制 `/snap/bin/docker-compose`，等价于 `docker compose` 插件；开源实现见 GitHub `docker/compose`，每个子命令对应 `cmd/compose/` 下一个源码文件）。`docker-compose.yml` 只**声明**“服务长什么样”（声明式），不含任何命令行为逻辑（命令式）。

| 层次 | 代码位置 | 职责 |
|---|---|---|
| 命令实现 | compose CLI 内置（`/snap/bin/docker-compose`） | `restart`/`up`/`down` 等子命令的调度逻辑 |
| 服务定义 | 项目的 `docker-compose.yml` | 声明镜像、环境变量、挂载、健康检查等 |
| 底层执行 | Docker daemon（dockerd） | 真正停/启容器（Engine API `POST /containers/{id}/restart`） |

**两个易混淆的 “restart”**：

| 形式 | 类型 | 触发方式 | 行为 |
|---|---|---|---|
| `restart: unless-stopped`（yml 字段，见 5.2.6） | 重启策略 | 容器异常退出时由 daemon **自动**重启 | 无人值守，按策略拉起 |
| `docker compose restart vllm`（命令） | 手动操作 | 运维执行 | stop + start **现有容器**；不重建、不重新解析 yml |

> **运维要点**：修改 `docker-compose.yml` 或 `.env` 后，必须 `docker compose up -d`（对比并重建有变更的容器）才生效；`restart` 只重启现有容器，**配置变更不会生效**。`console/scripts/docker-*.sh` 只是命令的封装，不包含实现。

#### 5.2.9 `--profile llm` 从哪里找服务（小白澄清）

**结论先行**：`--profile llm` **不是镜像的启动参数**，也**不进入容器内部**——它是 **Compose CLI 的命令行参数**，只在宿主机解析阶段起作用：从 `docker-compose.yml` 的服务定义里，把声明了 `profiles: ["llm"]` 的服务“放行”进本次启动的服务集合。

**数据来源 = `docker-compose.yml` 中的 `profiles:` 字段**（每个服务的归属标记）：

```yaml
vllm:
  profiles: ["llm"]   # ← 归属标记：这个服务属于 llm 组
  image: vllm/vllm-openai:${VLLM_IMAGE_TAG:-latest}
```

**匹配流程**（在 compose CLI 进程内完成，容器感知不到）：

```text
命令行 --profile llm（宿主机，compose CLI 解析）
        │
        ▼
读取 docker-compose.yml 的 7 个服务，逐个检查 profiles 字段
  无 profiles            → 无条件启用（agent / console 共 4 个）
  profiles: ["llm"]     → 与 --profile llm 匹配 → 启用（vllm）
  profiles: ["monitoring"] → 不匹配 → 过滤（prometheus / grafana）
        │
        ▼
生成候选集合（5 个）→ up vllm 从其中只选 vllm 执行
        │
        ▼
容器启动（vllm 进程） ← 完全感知不到 profile 的存在
```

**实测证据**（本机 compose v5.3.1）：

```bash
docker compose config --services             # → 4 个（无 vllm）
docker compose --profile llm config --services  # → 5 个（含 vllm）
```

**与“镜像启动参数”的区别**（小白最容易混淆的点）：

| 项 | `--profile llm` | vllm 的 `command:` 段（如 `--model ... --port 8000`） |
|---|---|---|
| 是什么 | compose CLI 命令行参数 | 镜像/容器的启动参数 |
| 由谁处理 | compose CLI（宿主机解析阶段） | dockerd → 容器内主进程（vllm） |
| 作用 | 决定“哪些服务被启动” | 决定“容器进程怎么运行” |
| 是否进入容器 | ❌ 不进（容器内无感知） | ✅ 进（容器主进程的命令行） |
| 写在哪里 | 命令行 / shell 脚本 | docker-compose.yml 的 `command:` 段 |

> **一句话总结**：`--profile` 是“选服务”的开关（compose 层，从 yml 的服务定义匹配）；`command:` 是“容器跑什么”的参数（镜像层，传给容器内进程）。两者互不干扰。

#### 5.2.10 镜像构建策略：不是每次 up 都构建（小白澄清）

**结论先行**：`docker compose up -d` **不是每次都会执行 build**。compose 采用“镜像优先”策略——本地已有 `privacy-local-agent:0.1.0` 就直接复用（**哪怕你刚改了源码/Dockerfile，也不会自动重建**）；只有镜像不存在时才尝试拉取/构建。

**决策链**（`PrivShield` 服务同时声明了 `build:` 与 `image:`，见 5.1）：

```text
docker compose up -d privacy-local-agent
        │
        ▼
① 本地已有 privacy-local-agent:0.1.0 ？
   ├─ 是 → 直接使用，跳过构建和拉取（最快路径）
   └─ 否 ↓
② 尝试从远端仓库 pull 该 tag
   ├─ 成功 → 使用拉取的镜像
   └─ 失败（无网络 / 私有仓库不可达）↓
③ 回退用 build: 段本地构建
```

**关键认知：compose 从不对比 Dockerfile/源码是否修改**，它只认“镜像名:tag 是否存在”。所以**改了代码后直接 `up -d` 不会重建**，容器里跑的还是旧代码！

**实测证据**（本机 compose v5.3.1，本地无该镜像时的 dry-run 输出）：

```text
Image privacy-local-agent:0.1.0 Pulling            ← ② 先尝试拉取
Image privacy-local-agent:0.1.0 connection refused ← 无远端仓库，失败
Image privacy-local-agent:0.1.0 Building           ← ③ 自动回退本地构建
writing image dryRun-... 
naming to privacy-local-agent:0.1.0                ← 构建出同名镜像
```

**改了代码后，三种强制重建方式**：

| 方式 | 命令 | 适用场景 |
|---|---|---|
| ① 两步走（推荐） | `docker compose build privacy-local-agent` → `docker compose up -d` | 步骤清晰，可先看构建结果 |
| ② 一步到位 | `docker compose up -d --build privacy-local-agent` | 构建+重建容器一条命令 |
| ③ 完全不用缓存 | `docker compose build --no-cache privacy-local-agent` | 改了基础镜像/依赖源，需彻底重装 |

> ①/② 构建后镜像 ID 变化，`up -d` 检测到差异会**自动重建容器**，无需手动删容器。

**与项目脚本的差异**：`scripts/dev/docker-start-agent.sh` 走裸 `docker build`（每次运行都重新构建，适合开发期频繁改代码）；compose `up -d` 是“有镜像就复用”（适合稳定期省时间）。两者行为不同，按场景选用。

**按改动类型选命令（心法表）**：

| 我改了什么 | 用哪条命令 | 原因 |
|---|---|---|
| Python 源码 / Dockerfile | `docker compose up -d --build` | 必须重建镜像才包含新代码 |
| 只改 `.env` / `privacy-profile.yaml` 等配置 | `docker compose up -d` | 无需构建；配置变化会重建容器 |
| 只是服务卡死想重启 | `docker compose restart` | 镜像、配置都不变，最轻量 |

#### 5.2.11 `llm` 网络在哪里配置、由谁实现（小白澄清）

**结论先行**：`llm` 网络是**在 `docker-compose.yml` 里声明**的（配置位置），但**创建与实现是 Docker daemon（dockerd）的 bridge 网络驱动**完成的（实现位置）。compose CLI 只负责把 yml 的声明翻译成对 Docker Engine API 的调用；项目代码中不存在任何网络实现逻辑。

**① 配置位置：`deploy/docker-compose/docker-compose.yml` 顶层 `networks:` 段**

```yaml
networks:
  llm:
    driver: bridge      # 桥接驱动：创建独立虚拟二层交换网络
    internal: false     # 非 internal：允许 ports 端口映射到宿主机（供 vllm 调试端口用）
```

- `driver: bridge` 是核心声明——告诉 daemon「用 Linux 网桥创建一个独立的虚拟二层网络」
- 服务侧通过 `networks:` 字段“接线”，**两个服务都挂载 `llm` 才互通**：
  - `PrivShield` → `networks: [backend, llm]`
  - `vllm` → `networks: [llm]`
- 未显式声明 `networks:` 的服务会自动加入 compose 默认网络（`<项目名>_default`），与 `llm` 网络互不相通；因此 `http://vllm:8000/v1` 能解析的前提，正是两个服务都显式挂载了 `llm` 网络

**② 实现位置：dockerd 的 bridge 网络驱动（libnetwork）**

compose 把 yml 翻译成 Engine API 调用后，真正“施工”的是 dockerd，实际发生的事：

```text
compose CLI（声明解析）                       dockerd（真正实现）
──────────────────────────────   ────────────────────────────────────────────
读取 yml 的 networks.llm         →  ① 创建 Linux 网桥 br-<hash>（虚拟交换机）
  driver: bridge                 →  ② IPAM 为该网络分配私有子网（如 172.20.0.0/16）
服务挂载 networks: [llm]         →  ③ 为每个容器创建 veth pair 并接入网桥
                                 →  ④ 启动内嵌 DNS（127.0.0.11）注册服务名
```

| 步骤 | 实现者 | 产物 / 效果 |
|---|---|---|
| ① 建网桥 | dockerd bridge driver | 宿主机出现 `br-<hash>` 网桥设备，相当于一台虚拟二层交换机，构成独立广播域（即“虚拟二层网段”） |
| ② 分配子网/IP | dockerd IPAM | 该网络独占一个私有子网（`docker network inspect` 的 `IPAM.Config` 可查），容器自动获得同网段动态 IP |
| ③ 接线 veth pair | dockerd（容器网络命名空间） | 每个容器一对虚拟“网线”：容器内一端是 `eth0`，另一端插到网桥上 → 同网桥容器二层互通 |
| ④ 内嵌 DNS | dockerd（DNS 代理） | 容器内 `vllm` 被解析为该容器的动态 IP（服务发现）；**IP 随容器重建而变化，服务名恒定**，故跨容器访问始终成立 |
| ⑤ 网络隔离 | 不同网桥 | 不同网络 = 不同网桥，默认互不相通（`llm` 与 `backend`/`frontend` 各自独立），网络同时是安全边界 |

**③ 在宿主机上可观察到的证据**：

```bash
docker network ls                            # 看到 docker-compose_llm（compose 网络名带项目前缀）
docker network inspect docker-compose_llm    # Driver: bridge；IPAM 子网；Containers 列出 agent 与 vllm 的 IP
docker exec PrivShield getent hosts vllm   # 容器内解析服务名 → 返回 vllm 容器 IP（如 172.20.0.5）
```

> **一句话总结**：yml 的 `networks:` 段是“图纸”，dockerd 的 bridge 驱动是“施工队”——它建网桥（虚拟交换机）、发 IP、接线（veth pair）、跑 DNS，最终让 agent 与 vllm 在同一个虚拟二层网段里用服务名互访。

**④ 跨主机部署：`llm` 网络为何失效与替代方案（按部署拓扑选型）**

实际部署中 vllm 需要 **GPU 主机**，而 core 是无 torch 的轻量 Sidecar、任何主机都能跑，因此存在两种典型拓扑，`llm` 网络的可用性完全取决于此：

**场景 A：同机部署（core 与 vllm 在同一台主机）——`llm` 网络直接可用**

- 即默认 compose 编排：GPU 主机同时跑 core + vllm，走 ①② 的 bridge 网络，容器名直连 `http://vllm:8000/v1`，**零额外配置**
- core 镜像不含 torch、内存占用小（deploy 上限 2G），与 vllm 同机几乎不互相影响；无 GPU 的主机则跳过 `--profile llm` 纯跑 core

**场景 B：跨主机部署（core 在普通主机、vllm 在 GPU 主机）——`llm` 网络失效**

失效原因即 ③ 所述：bridge 网络是本机概念——两台机器的网桥互不相通，且容器名 DNS 只在**本机 daemon** 内注册，core 主机上解析不到 GPU 主机上的 `vllm`。此时无论怎么配置 `networks: llm` 都不跨机器生效，必须选用替代方案：

| 方案 | 配置要点 | 优点 | 风险 / 成本 | 适用场景 |
|---|---|---|---|---|
| B1 端口映射 + 实际 IP | GPU 主机：vllm 映射 `0.0.0.0:8000:8000`；core 主机：`deploy/docker-compose/.env` 写 `LLM_API_BASE=http://<vllm主机IP>:8000/v1` 与 `LLM_API_KEY=<key>`（未设置时默认 `http://vllm:8000/v1` 与 `EMPTY`，零配置） | 零改造、最快打通 | 端口直裸公网有安全风险（vllm 默认无鉴权、API Key 为 `EMPTY`）；IP 变化需改配置 | 内网互通 / 快速验证 |
| B2 组网（WireGuard / Tailscale） | 两台主机接入同一虚拟内网（Tailscale 分配 100.x 地址）；vllm 监听 `0.0.0.0:8000`；core 写 `http://<vllm虚拟内网IP>:8000/v1` | 不暴露公网端口、虚拟 IP 稳定，安全 | 需安装维护组网工具 | 生产推荐（公网环境安全访问） |
| B3 Swarm overlay 网络 | 两台主机 `docker swarm join` 加入同一集群，`docker network create -d overlay llm`，服务名跨节点由 swarm 内置 DNS 解析 | 服务名直连体验与 bridge 一致 | 需将 compose 迁移为 `docker stack deploy`（服务需补 `deploy:` 段） | 已采用 Swarm 的团队 |
| B4 K8s（本项目正式形态） | 见 §6.2：Helm `llm.enabled=true` 自动创建 `-llm` Deployment + Service，core 用 `http://<fullname>-llm:8000/v1` 跨节点访问 | 跨节点 DNS、多副本、自动恢复、GPU 调度 | 需要 K8s 集群运维能力 | 生产多副本 / 大规模 |

**选型速查**：

| 我的部署形态 | 直接选 |
|---|---|
| 单台 GPU 主机，core + vllm 同机 | 默认 `networks: llm`（零改动） |
| 两台主机内网互通（如同机房） | B1 端口映射（配合防火墙限制来源 IP） |
| 两台主机公网隔离（不同公网 IP） | B2 组网（推荐）或 B4 K8s |
| 已上容器集群 | B3 Swarm 或 B4 K8s |

> **安全与网络调优提醒**：
> 1. **鉴权安全**：B1 若必须暴露公网，vllm 默认无鉴权（`PRIVACY_LLM_API_KEY=EMPTY`），务必用防火墙/安全组把 8000 端口限制为仅 core 主机 IP 可访问，若通过网关加了 Token 鉴权，可在 `deploy/docker-compose/.env` 配置 `LLM_API_KEY=sk-xxxx`；跨公网方案应优先考虑 B2 组网，避免端口直裸。
> 2. **网络延迟与超时**：跨主机/跨云网络延迟（10~50ms）高于本机网桥（<0.5ms）。在大流量或长文本生成场景下，若网络波动引起偶发超时，可在根目录 `.env` 中按需调大 `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` 或配置专线加速。

### 5.3 自动化 Docker 脚本运行集

为简化容器化运维与测试，项目在 `scripts/dev/`（侧边栏/LLM 单组分）与 `console/scripts/`（全栈/控制台）中内置了一套便捷的 Docker 脚本：

```bash
# 1. 独立运行/停止 Privacy Agent 容器 (支持 core / ml 目标)
./scripts/dev/docker-start-agent.sh [core|ml]
./scripts/dev/docker-stop-agent.sh

# 2. 启动/停止 vLLM 大模型推理服务容器 (GPU 加速)
./scripts/dev/docker-start-llm.sh
./scripts/dev/docker-stop-llm.sh

# 3. 启动 Agent + Go 代理后端 + Web UI 容器套件
./console/scripts/docker-start-go.sh

# 4. 启动 Agent + Python 代理后端 + Web UI 容器套件
./console/scripts/docker-start-python.sh

# 5. 启动全栈 Docker 容器套件 (Agent + 双后端 + Web UI + 可选 vLLM)
./console/scripts/docker-start-all.sh [--with-llm]

# 6. 一键停止并清理全栈 Docker 容器与 Compose 栈
./console/scripts/docker-stop.sh
```

| 脚本文件 | 适用场景与功能 | 默认端口 / 网络 | 对应自动化测试套件 |
|---|---|---|---|
| `scripts/dev/docker-start-agent.sh` | 单独启动 Agent 容器（支持 `core` / `ml` 镜像） | REST: 8079, gRPC: 50051 | `tests/scripts/test_docker_start_agent.py` |
| `scripts/dev/docker-stop-agent.sh` | 停止并删除 Agent 单容器 | — | `tests/scripts/test_docker_start_agent.py` |
| `scripts/dev/docker-start-llm.sh` | 启动独立 vLLM 大模型 GPU 推理容器 | HTTP: 8000 (OpenAI 兼容) | `tests/scripts/test_docker_start_llm.py` |
| `scripts/dev/docker-stop-llm.sh` | 停止并删除 vLLM 大模型容器 | — | `tests/scripts/test_docker_start_llm.py` |
| `console/scripts/docker-start-all.sh` | 一键拉起 Agent + 双代理后端 + Web UI（可选 `--with-llm`） | Web: 5173, Go: 8081, Py: 8080 | Docker Compose 全栈编排 |
| `console/scripts/docker-stop.sh` | 一键停止并清理所有全栈 Compose 容器 | — | 执行 `docker compose down` |

> **自动化测试建议**：
> 运行以下命令可自动验证启动脚本、网络拓扑与真实容器的生命周期：
> ```bash
> # 运行 Agent 脚本、Compose 拓扑与 Docker 容器全套测试（30 用例）
> .venv/bin/pytest tests/scripts/test_docker_start_agent.py -v -s
>
> # 运行 vLLM 大模型脚本与 OpenAI 兼容推理测试
> .venv/bin/pytest tests/scripts/test_docker_start_llm.py -v -s
> ```

### 5.4 Docker Compose 常用命令速查（小白学习用）

| 命令 | 作用 | 常用场景 |
|---|---|---|
| `docker compose up -d` | 创建+启动全部服务（后台） | 第一次部署 / 修改配置或源码后重建 |
| `docker compose --profile llm up -d` | 启动含 vLLM 的全栈 | 需要大模型推理时 |
| `docker compose ps` | 查看服务状态与健康检查 | 确认是否启动成功 |
| `docker compose logs -f <服务名>` | 查看服务日志（`-f` 实时跟随） | 排查启动失败/报错 |
| `docker compose restart <服务名>` | 重启现有容器（**不重建、不重新解析配置**） | 服务卡死时的临时恢复 |
| `docker compose stop` / `start` | 暂停 / 恢复全部服务（保留容器） | 暂时不用但不想删 |
| `docker compose down` | 停止并删除容器+网络（**保留数据卷**） | 结束一次部署 |
| `docker compose down -v` | 连数据卷一起删除 | 想从零开始（**数据会丢！**） |
| `docker compose exec <服务名> sh` | 进入容器内部命令行 | 容器内排查问题 |
| `docker compose build <服务名>` | 重新构建镜像 | 改了源码后重建 |
| `docker compose up -d --build <服务名>` | 重新构建镜像并启动（一步到位） | 改源码后重建，最常用 |
| `docker compose build --no-cache <服务名>` | 不用缓存层彻底重建 | 改了基础镜像/依赖源 |
| `docker compose pull` | 只拉取镜像不启动 | 预下载镜像 |
| `docker compose config` | 查看解析合并后的最终配置 | 检查变量替换/合并是否正确 |
| `docker compose up -d <服务名>` | 只启动/升级某个服务 | 单独升级 vllm（core 无感知） |
| `docker compose top` | 查看容器内运行的进程 | 确认进程是否存活 |

### 5.5 常见配置需求速查（想改 X → 改哪里）

| 我的需求 | 修改位置 | 生效方式 |
|---|---|---|
| 换 LLM 推理后端（vllm/qwen3/mlx/openai） | 根目录 `.env` 的 `PRIVACY_ENV_PROFILE` | 本地重启进程；容器 `docker compose up -d` |
| core 与 vllm 跨主机部署（分机/跨云） | `deploy/docker-compose/.env` 写 `LLM_API_BASE=http://<vllm主机IP>:8000/v1` 与 `LLM_API_KEY=sk-xxx`（详见 5.2.11 ④） | `docker compose up -d` |
| 改监听端口 8079 / 50051 | 根目录 `.env`（agent 行为）+ compose `ports:`（映射） | `docker compose up -d` |
| 改 vLLM 对外端口 | compose 的 vllm `ports:`（如 `"9000:8000"`，只改左侧宿主端口） | `docker compose up -d vllm`（详见 5.2.6） |
| 开 TLS / API Key 认证 | 根目录 `.env` 取消注释（本地）；compose `environment:` 取消注释（容器） | 重建容器 |
| 固定 vLLM 镜像版本 | `deploy/docker-compose/.env` 写 `VLLM_IMAGE_TAG=v0.26.x` | `docker compose --profile llm up -d vllm` |
| 改 Grafana 密码 | `deploy/docker-compose/.env` 写 `GRAFANA_ADMIN_PASSWORD=...` | `docker compose --profile monitoring up -d grafana` |
| 调脱敏/DP/K匿名参数 | 容器：`deploy/docker-compose/privacy-profile.yaml`；本地：`config/sample-privacy-profile.yaml` | 重建容器 / 重启进程 |
| 看更详细的日志 | 根目录 `.env` 的 `PRIVACY_LOG_LEVEL=DEBUG` | 重启生效 |
| 升级 agent 版本 | 改 compose `image:` tag 或重新 `build` | `docker compose up -d` |
| 改 agent 源码（如 `privacy_local_agent/*.py`） | 无需改配置，直接重新构建镜像 | `docker compose up -d --build privacy-local-agent` |
| 清理容器日志占用的磁盘 | compose 已有 max-size 自动轮转；想手动清可 `docker compose logs --tail=0` | 即时 |
| 想用监控栈（Prometheus+Grafana） | 无需改配置，加 `--profile monitoring` 即可 | `docker compose --profile monitoring up -d` |

---

## 6. LLM 推理服务部署（集成模式 / 解耦模式）

Layer-3 LLM 深度分类支持两种部署模式，核心差异是 **LLM 推理在哪里运行**：

| 维度 | 集成模式（进程内本地推理） | 解耦模式（外部独立 vLLM 服务） |
|---|---|---|
| LLM 运行位置 | Agent 进程内（PyTorch/Transformers） | 独立 vLLM 容器/服务（OpenAI 兼容 HTTP） |
| Agent 镜像 | **ml**（含 torch/transformers） | **core**（无需 ML 依赖） |
| `PRIVACY_LLM_PROVIDER` | `qwen3` | `vllm` |
| 模型来源 | 本地模型目录（需挂载进容器/Pod） | vLLM 侧管理（挂载 `.models`） |
| 适用场景 | 无 GPU / 单机私有化交付 / 测试 | 生产 GPU / 高并发 / 多副本 |
| 多副本扩容 | ❌ 不可（每副本加载一份模型，OOM 风险） | ✅ core 多副本共享一个 LLM 实例 |

### 6.1 集成模式（进程内本地 LLM）

**本地直跑**：`.env` 或 `PRIVACY_ENV_PROFILE=qwen3`（加载 `config/env/qwen3.env`）：

```ini
PRIVACY_LLM_PROVIDER=qwen3
PRIVACY_LLM_MODEL_PATH=.models/Qwen3.5-0.8B-Privacy-Classifier-Smoother
PRIVACY_LLM_DEVICE=cuda          # 无 GPU 改 cpu
PRIVACY_LLM_ENABLE=true          # 可选：显式启用 Layer-3（默认按置信度触发）
PRIVACY_NER_ENABLE=true          # Layer-2 Small-NER
```

**Helm 部署**（`llm.enabled` 保持 false，**勿开启**——那是解耦模式）：

```bash
helm install pla-ml ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-ml.yaml \
  --set image.tag=0.1.0-ml
```

```yaml
# 自定义 values（集成模式关键配置）
flavor: ml                          # 必须 ml 镜像（含 torch）
extraEnv:
  - name: PRIVACY_LLM_PROVIDER
    value: "qwen3"
  - name: PRIVACY_LLM_MODEL_PATH
    value: "/models/Qwen3.5-0.8B-Privacy-Classifier-Smoother"
  - name: PRIVACY_LLM_DEVICE
    value: "cuda"
  - name: PRIVACY_LLM_ENABLE
    value: "true"
extraVolumes:                       # 模型权重不在镜像内，必须挂载
  - name: models
    hostPath: { path: /data/models }    # 生产推荐 PVC
extraVolumeMounts:
  - name: models
    mountPath: /models
```

### 6.2 解耦模式（外部独立 vLLM 服务）

**本地直跑**：`.env` 或 `PRIVACY_ENV_PROFILE=vllm`（加载 `config/env/vllm.env`），并先启动 vLLM 服务：

```ini
PRIVACY_LLM_PROVIDER=vllm
PRIVACY_LLM_API_BASE=http://127.0.0.1:8000/v1
PRIVACY_LLM_MODEL_NAME=Qwen3.5-0.8B-Privacy-Classifier-Smoother
PRIVACY_LLM_API_KEY=EMPTY
```

```bash
python run_vllm_server.py          # 宿主机方式启动 vLLM
# 或 Docker 方式（官方镜像，零构建）：
docker compose --profile llm up -d vllm
```

**国内网络拉取 `vllm/vllm-openai` 镜像超时的备选方案**（`docker pull` 报 `i/o timeout` / 连接 `registry-1.docker.io` 失败时）：

Docker 守护进程不继承 shell 的 HTTP(S)_PROXY 代理环境变量，且默认未配置国内镜像加速器时直连 Docker Hub 会超时。可先从 DaoCloud 镜像源拉取，再打回官方 tag（compose 仍按 `vllm/vllm-openai:${VLLM_IMAGE_TAG:-latest}` 解析，无需改动编排）：

```bash
# 从 DaoCloud 镜像源拉取（其他可选源：docker.1ms.run / dockerproxy.net / hub.rat.dev，前缀替换同上）
docker pull docker.m.daocloud.io/vllm/vllm-openai:latest
# 打回官方 tag，使 compose 与 docker-start-llm.sh 无需修改即可命中本地镜像
docker tag docker.m.daocloud.io/vllm/vllm-openai:latest vllm/vllm-openai:latest
```

> 长期方案：配置 `/etc/docker/daemon.json` 的 `registry-mirrors` 国内镜像加速器，或为 docker 服务（systemd drop-in）显式配置 `HTTP_PROXY`/`HTTPS_PROXY` 代理；生产环境建议固定镜像 tag（如 `VLLM_IMAGE_TAG` 指定 v0.26.x）保证可复现。

**Helm 部署** —— 方式一：**Helm 全托管**（一条命令同时创建 core + LLM Deployment）：

```bash
helm install pla ./deploy/helm/PrivShield --set llm.enabled=true
```

- 自动创建 `-llm` Deployment（vLLM + GPU 预留）+ ClusterIP Service
- core 自动注入 4 个连接 env，`PRIVACY_LLM_API_BASE` 指向 `http://<fullname>-llm:8000/v1`
- 模型来源：`llm.storage.hostPath`（单节点）或 `llm.storage.existingClaim`（PVC，生产推荐）
- GPU 调度：`llm.nodeSelector` / `llm.tolerations`（需集群安装 NVIDIA device plugin）

方式二：**外部已有 vLLM 服务**（LLM 不由 Helm 管理，core 只连出去）：

```yaml
extraEnv:
  - name: PRIVACY_LLM_PROVIDER
    value: "vllm"
  - name: PRIVACY_LLM_API_BASE
    value: "http://llm-svc.llm-ns.svc:8000/v1"   # 外部 vLLM 服务地址
  - name: PRIVACY_LLM_MODEL_NAME
    value: "Qwen3.5-0.8B-Privacy-Classifier-Smoother"
  - name: PRIVACY_LLM_API_KEY
    value: "EMPTY"
```

**Docker Compose**：已内置解耦配置（core 服务 4 个 `PRIVACY_LLM_*` env + vllm 服务），无需改动：

```bash
docker compose up -d                # 纯 core（无 LLM）
docker compose --profile llm up -d  # 解耦模式（core + 独立 vLLM）
```

> **易错点**：解耦模式下 `PRIVACY_LLM_MODEL_NAME` 必须与 vLLM 启动参数 `--served-model-name` **完全一致**，否则返回 404；两种模式**二选一**——集成模式误开 `llm.enabled=true` 会导致模板注入的 `PRIVACY_LLM_PROVIDER=vllm` 覆盖 `qwen3` 配置。

### 6.3 并发与内存护栏（两种模式通用）

进程级防护，防止并发推理叠加导致 OOM（历史上曾表现为 Go 客户端 `connection reset by peer`）：

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` | 进程级 LLM 推理并发上限（信号量，所有适配器共享） |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | `30` | 信号量排队超时，超时降级跳过 LLM 层 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | 推理前内存预检阈值，低于则降级 |

> 护栏为**进程级**，跨 Pod 无效：集成模式多副本部署时每副本各加载一份模型，内存/显存线性增长，**禁止多副本扩容**；需要并发扩展请改用解耦模式（core 多副本 + 单个 vLLM 实例）。

---

## 7. 安全配置

所有安全开关默认关闭，生产环境通过环境变量显式启用。配置由 `privacy_local_agent/security/config.py` 中的 `SecuritySettings` 统一解析。

### 7.1 TLS / mTLS

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_TLS_ENABLED` | `false` | 启用 TLS |
| `PRIVACY_TLS_CERT_FILE` | — | 服务端证书路径（必填） |
| `PRIVACY_TLS_KEY_FILE` | — | 服务端私钥路径（必填） |
| `PRIVACY_TLS_CA_FILE` | — | CA 证书路径（mTLS 时必填） |
| `PRIVACY_TLS_CLIENT_AUTH` | `none` | 客户端认证模式：`none` / `optional` / `require` |
| `PRIVACY_TLS_KEY_PASSWORD` | — | 私钥密码（可选） |

**实现细节**：
- REST 端：通过 `uvicorn_ssl_kwargs()` 构造 SSL 参数传递给 Uvicorn。
- gRPC 端：通过 `grpc_server_credentials()` 构造 `grpc.ServerCredentials`。
- 当 `tls_client_auth=require` 时启用双向 mTLS，客户端必须出示受信任证书。

**Helm 部署时**：TLS 证书通过 K8s Secret 挂载到 `/certs/` 目录，Deployment 模板自动配置探针使用 `curl -k https://...` 方式探测。

### 7.2 API Key 认证

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_AUTH_ENABLED` | `false` | 启用 API Key 认证 |
| `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` | `{}` | 外部 API Key JSON 映射 |
| `PRIVACY_AUTH_INTERNAL_KEYS_JSON` | `{}` | 内部 API Key JSON 映射 |
| `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED` | `false` | 内部 mTLS 免 Key 认证 |

**API Key JSON 格式**：

```json
{
  "your-api-key-string": {
    "name": "gateway-service",
    "scopes": ["*"]
  }
}
```

- `name`：人类可读标识，用于日志和速率限制键。
- `scopes`：权限列表，`["*"]` 表示完全访问。

**请求携带方式**：HTTP Header `X-API-Key: your-api-key-string`。

### 7.3 速率限制

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_RATE_LIMIT_ENABLED` | `false` | 启用速率限制 |
| `PRIVACY_RATE_LIMIT_DEFAULT_RPS` | `10` | 默认每秒请求数 |
| `PRIVACY_RATE_LIMIT_DEFAULT_BURST` | `20` | 默认突发上限 |
| `PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON` | `{}` | 按端点覆盖限制 |
| `PRIVACY_RATE_LIMIT_REDIS_URL` | — | Redis URL（多实例共享限流状态） |

> 健康检查端点默认跳过认证和限速（`PRIVACY_HEALTH_NO_AUTH=true`、`PRIVACY_HEALTH_NO_RATE_LIMIT=true`）。

---

## 8. 服务启动与优雅关闭

容器入口为 `python -m privacy_local_agent.server`，该模块实现 REST + gRPC 双协议统一启动：

```text
启动流程：
1. 解析命令行参数（--rest-host/--rest-port/--grpc-host/--grpc-port），优先级高于环境变量
2. 构造 Uvicorn SSL 参数（若 TLS 启用）
3. 在非守护线程中启动 REST 服务（uvicorn.Server）
4. 启动 gRPC 服务（非阻塞模式）
5. 注册 SIGTERM / SIGINT 信号处理器
6. 主线程等待终止信号

优雅关闭流程：
1. 捕获 SIGTERM/SIGINT 信号
2. 停止 gRPC 服务（保留 5 秒在途请求处理时间）
3. 设置 REST 服务 should_exit = True
4. 等待 REST 线程退出（超时 10 秒）
5. 进程安全退出（exit code 0）
```

**REST 应用生命周期**（`main.py` lifespan）：
1. 初始化结构化日志（`configure_logging`）
2. 初始化 OpenTelemetry Tracing（若配置了 OTLP endpoint）
3. 异步预热 LLM 模型（若 `PRIVACY_WARMUP_LLM=true`）
4. 注册可观测性中间件 + 挂载 `/metrics`
5. 挂载所有业务路由

---

## 9. 健康检查与探针

| 端点 | 用途 | 返回 |
|---|---|---|
| `GET /health` | 通用健康检查（K8s liveness 默认；readiness 默认使用 `/readyz`） | `{"status": "ok", "namespace": "..."}` |
| `GET /livez` | 存活探针 | `{"status": "alive"}` |
| `GET /readyz` | 就绪探针（检查配置解析器 + 预算 DB 连通性） | `{"status": "ready"}` |

**就绪探针检查逻辑**（`/readyz`）：
1. 验证 `Configuration resolver` 已初始化，否则返回 503。
2. 若配置了 `PRIVACY_BUDGET_DB`，尝试 SQLite 连接（2s 超时），失败返回 503。
3. 返回 `{"status": "ready"}`。

**TLS 模式下的探针**：Helm 模板自动切换为 `exec` 方式（`curl -fsS -k https://...`），避免 httpGet 无法处理自签证书。

---

## 10. 监控与告警

### 10.1 Prometheus 指标

指标通过 `/metrics` 端点暴露（`prometheus-client` ASGI app），所有指标以 `privacy_` 为前缀。

**核心请求指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_requests_total` | Counter | method, path, status | REST/gRPC 请求总数 |
| `privacy_request_duration_seconds` | Histogram | method, path | 请求延迟分布 |
| `privacy_traffic_bytes_total` | Counter | method, path, direction | 请求/响应流量（字节） |

**隐私原语指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_dp_queries_total` | Counter | mechanism, aggregation | DP 查询次数 |
| `privacy_dp_duration_seconds` | Histogram | aggregation, mechanism | DP 查询延迟 |
| `privacy_budget_remaining` | Gauge | namespace, budget_type | 剩余隐私预算 |
| `privacy_masking_operations_total` | Counter | operation | 脱敏操作次数 |
| `privacy_masking_duration_seconds` | Histogram | operation | 脱敏延迟 |
| `privacy_kano_operations_total` | Counter | operation | K-匿名操作次数 |
| `privacy_kano_duration_seconds` | Histogram | operation | K-匿名延迟 |
| `privacy_qol_operations_total` | Counter | domain | 查询混淆次数 |
| `privacy_qol_duration_seconds` | Histogram | domain | 查询混淆延迟 |

**分类指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_classification_total` | Counter | final_level, layer | 分类结果统计 |
| `privacy_classification_duration_seconds` | Histogram | operation | 分类操作延迟 |
| `privacy_classification_ner_total` | Counter | status | NER 引擎调用次数 |
| `privacy_classification_ner_duration_seconds` | Histogram | engine | NER 推理延迟 |
| `privacy_classification_llm_total` | Counter | status | LLM 引擎调用次数 |
| `privacy_classification_llm_duration_seconds` | Histogram | engine | LLM 推理延迟 |
| `privacy_classification_rule_hits_total` | Counter | rule_id | Layer-1 规则命中 |
| `privacy_classification_composite_hits_total` | Counter | rule_id | 组合规则命中 |
| `privacy_classification_jobs_total` | Counter | status | 异步分类任务 |
| `privacy_classification_jobs_duration_seconds` | Histogram | status | 异步任务延迟 |

**安全指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_auth_denials_total` | Counter | reason | 认证/授权/限速拒绝次数 |
| `privacy_auth_duration_seconds` | Histogram | result | 认证检查延迟 |

**网关指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_gateway_requests_total` | Counter | protocol, method, status | 网关代理请求 |
| `privacy_gateway_latency_seconds` | Histogram | protocol | 网关代理延迟 |
| `privacy_gateway_healthy_nodes` | Gauge | — | 健康后端节点数 |
| `privacy_gateway_retries_total` | Counter | protocol, reason | 网关重试次数 |

**其他指标**：

| 指标名 | 类型 | Labels | 说明 |
|---|---|---|---|
| `privacy_profile_resolve_total` | Counter | primitive, status | 参数解析操作 |
| `privacy_data_extraction_total` | Counter | format, status | 数据提取操作 |

### 10.2 告警规则

告警规则文件位于 `deploy/prometheus/alerts.yml`，挂载到 Prometheus rules 目录使用：

```yaml
# prometheus.yml
rule_files:
  - /etc/prometheus/rules/PrivShield-alerts.yml
```

**告警规则汇总**：

| 告警名 | 组 | 严重级别 | 触发条件 | 持续时间 |
|---|---|---|---|---|
| GatewayNoHealthyNodes | availability | critical | `privacy_gateway_healthy_nodes == 0` | 1m |
| GatewayDegradedCapacity | availability | warning | `privacy_gateway_healthy_nodes < 2` | 5m |
| HighRequestLatencyP95 | latency | warning | P95 > 1s | 5m |
| HighGatewayLatencyP95 | latency | warning | 网关 P95 > 2s | 5m |
| HighClassificationLatency | latency | warning | 分类 P95 > 5s | 5m |
| HighGatewayErrorRate | errors | critical | 5xx 错误率 > 5% | 5m |
| HighAuthDenialRate | errors | warning | 认证拒绝率 > 10% | 5m |
| HighGatewayRetryRate | errors | warning | 重试率 > 10% | 5m |
| PrivacyBudgetNearlyExhausted | privacy | warning | 预算剩余 < 0.1 | 1m |
| PrivacyBudgetExhausted | privacy | critical | 预算耗尽 ≤ 0 | 1m |
| HighLLMClassifierErrorRate | classification | warning | LLM 错误率 > 10% | 5m |

### 10.3 Grafana 仪表盘

预置仪表盘 JSON 位于 `deploy/grafana/dashboard.json`，包含以下面板：

| 面板 | PromQL | 说明 |
|---|---|---|
| Request Rate (by method) | `sum(rate(privacy_requests_total[5m])) by (method)` | 请求速率 |
| Request Latency (p50/p95) | `histogram_quantile(0.95/0.50, ...)` | 延迟分位数 |
| Gateway Request Rate | `sum(rate(privacy_gateway_requests_total[5m])) by (protocol, status)` | 网关流量 |
| Gateway Healthy Nodes | `privacy_gateway_healthy_nodes` | 健康节点数（Stat） |
| Gateway Latency (p95) | `histogram_quantile(0.95, ...)` | 网关延迟 |
| Classification Results | `sum(rate(privacy_classification_total[5m])) by (final_level, layer)` | 分类结果分布 |
| Auth Denials (by reason) | `sum(rate(privacy_auth_denials_total[5m])) by (reason)` | 认证拒绝 |
| Privacy Budget Remaining | `privacy_budget_remaining` | 预算余量 |
| Privacy Primitives Operations | masking / kano / dp 速率 | 原语操作速率 |

**导入方式**：Grafana → Dashboards → Import → Upload JSON file → 选择 `deploy/grafana/dashboard.json`。

### 10.4 ServiceMonitor（Prometheus Operator）

Helm 安装时设置 `serviceMonitor.enabled=true` 即可自动创建 ServiceMonitor：

```yaml
# 生成的 ServiceMonitor 规格
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: PrivShield
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
      scrapeTimeout: 10s
```

---

## 11. 自动伸缩（HPA）

Helm 模板使用 `autoscaling/v2` API，支持 CPU 和内存双指标：

```yaml
# values-production.yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80
```

**注意事项**：
- 启用 HPA 后，`replicaCount` 字段被忽略（Deployment 模板中有条件判断）。
- 可通过 `autoscaling.behavior` 自定义缩容策略（如冷却窗口）。
- ML 镜像建议 `maxReplicas` 不宜过大（单 Pod 内存占用高）。

---

## 12. 网络策略（NetworkPolicy）

生产环境启用 NetworkPolicy 限制入站流量：

```yaml
# values-production.yaml
networkPolicy:
  enabled: true
  ingress:
    from:
      - podSelector:
          matchLabels:
            app.kubernetes.io/part-of: PrivShield
```

**生成的 NetworkPolicy 行为**：
- 仅允许同命名空间中带有 `app.kubernetes.io/part-of: privacy-local-agent` 标签的 Pod 访问。
- 开放端口：REST（8079）+ gRPC（50051）。
- 如需允许 Ingress Controller 访问，需额外添加对应的 `from` 规则。

---

## 13. 配置体系与环境变量参考

### 13.1 配置体系总览

项目运行时配置采用**「分层级联」**机制：**运行时参数统一收口在根目录 `.env`（本地开发）或 Deployment/values（容器环境），编排差异由 Docker Compose / K8s / Helm 三套独立文件承载**。

#### 13.1.1 本地开发环境配置

本地运行时配置以根目录 `.env` 为**运维主控入口**（由 `env_loader.py` 启动时自动加载），共 8 大类（场景 Profile、网络监听、日志、安全、隐私预算、分类漏斗、LLM 资源保护、图片打码/网关），每项均有中英文注释与生产推荐值。

加载顺序（优先级递增）：

```text
1. .env                                 ← 基础配置（根目录，运维主控入口）
2. config/env/<PRIVACY_ENV_PROFILE>.env ← 场景覆盖（vllm / qwen3 / mlx / openai）
3. 系统环境变量 / Docker / K8s 注入      ← 最高优先级
```

配套配置文件：

| 文件 | 作用 |
|---|---|
| `.env` | 基础环境变量主控入口（网络监听 / 日志 / 安全 / 预算 / 分类 / LLM 等 8 大类） |
| `config/env/<profile>.env` | 按 `PRIVACY_ENV_PROFILE` 级联加载的场景覆盖（vllm / qwen3 / mlx / openai 四套） |
| `config/sample-privacy-profile.yaml` | 参数 Profile 模板（脱敏 / DP / K匿名 等参数），通过 `PRIVACY_PROFILE` 变量引用 |
| `config/personalized-profiles.yaml` | 个性化推荐参数文件（`PRIVACY_PERSONALIZED_PROFILE`） |

> **切换 LLM 推理后端只需修改 `PRIVACY_ENV_PROFILE` 一行**，自动级联加载对应场景覆盖文件。

#### 13.1.2 Docker Compose 配置

`deploy/docker-compose/docker-compose.yml` 负责**服务编排**（7 个服务、网络、卷、健康检查、资源限制），但它与根目录 `.env` 是**联动关系**，而非独立配置源：

- compose 自动读取根目录 `.env` 进行**变量替换**（如 `${VLLM_IMAGE_TAG:-latest}`、`${GRAFANA_ADMIN_PASSWORD:-changeme}`）；
- agent 服务通过 `env_file: ../../.env` **显式引入**根目录 `.env`；
- `environment:` 段仅做**容器化必要覆盖**（如 `PRIVACY_REST_HOST: "0.0.0.0"` 覆盖本地的 `127.0.0.1`）。

配套文件：

| 文件 | 作用 |
|---|---|
| `deploy/docker-compose/docker-compose.yml` | 全栈服务编排（Agent / 双 Console 后端 / Web / vLLM / 监控） |
| `deploy/docker-compose/privacy-profile.yaml` | 容器内参数 Profile（只读挂载到 `/etc/privacy-local-agent/`） |
| `deploy/prometheus/`、`deploy/grafana/` | 监控栈配置（Prometheus 抓取/告警规则、Grafana provisioning） |

#### 13.1.3 Kubernetes 配置

K8s 配置独立存放于 `deploy/k8s/`（原生 YAML）与 `deploy/helm/`（Helm Chart），**不依赖根目录 `.env`**，环境变量直接内联在 Deployment 或 values 中：

| 文件 | 职责 |
|---|---|
| `deploy/k8s/deployment.yaml` | 环境变量直接内联在 `spec.containers[].env`；TLS / Auth / LLM 以注释形式给出开启模板 |
| `deploy/k8s/configmap.yaml` | 参数 Profile（`privacy-profile.yaml`）→ ConfigMap，挂载到 `/etc/privacy-local-agent` |
| `deploy/k8s/llm-deployment.yaml` + `llm-service.yaml` | 独立 vLLM 推理服务（运行时解耦，LLM 挂掉 core 自动降级） |
| `deploy/k8s/secret.example.yaml` | API Key / TLS 证书 Secret 示例 |
| `deploy/k8s/kustomization.yaml` | `kubectl apply -k` 一键部署入口 |
| `deploy/helm/privacy-local-agent/` | Helm Chart（`values.yaml` / `values-production.yaml` / `values-ml.yaml`，生产推荐） |

#### 13.1.4 三环境配置差异速览

| 维度 | 本地开发 | Docker Compose | Kubernetes |
|---|---|---|---|
| 编排文件 | — | `deploy/docker-compose/docker-compose.yml` | `deploy/k8s/*.yaml` 或 `deploy/helm/privacy-local-agent/` |
| 环境变量来源 | 根目录 `.env` + `config/env/<profile>.env` | 根目录 `.env`（env_file + 变量替换）+ `environment:` 覆盖 | Deployment `env` 段内联 / Helm `extraEnv` + `values.yaml` |
| 参数 Profile | `PRIVACY_PROFILE` 指向本地 YAML | `privacy-profile.yaml` 只读挂载 | ConfigMap 挂载 `/etc/privacy-local-agent` |
| 监听地址 | 默认 `127.0.0.1`（仅本机） | 必须 `0.0.0.0`（容器内） | 必须 `0.0.0.0`（容器内） |
| 安全配置 | 默认关闭 | 取消注释或经 `.env` 注入 | values 开关 + Secret |

---

### 13.2 环境变量参考

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_REST_HOST` | `0.0.0.0`（容器）/ `127.0.0.1`（本地） | REST 监听地址 |
| `PRIVACY_REST_PORT` | `8079` | REST 监听端口 |
| `PRIVACY_GRPC_HOST` | `0.0.0.0`（容器）/ `127.0.0.1`（本地） | gRPC 监听地址 |
| `PRIVACY_GRPC_PORT` | `50051` | gRPC 监听端口 |
| `PRIVACY_PROFILE` | — | YAML 参数 Profile 路径 |
| `PRIVACY_NAMESPACE` | `default` | 预算命名空间 |
| `PRIVACY_BUDGET_DB` | — | SQLite 预算持久化路径（多实例必配） |
| `PRIVACY_BUDGET_WINDOW_SECONDS` | — | 预算自动重置时间窗口 |
| `PRIVACY_LOG_LEVEL` | `INFO` | 日志级别 |
| `PRIVACY_LOG_FORMAT` | `text` | 日志格式：`text` / `json` |
| `PRIVACY_SERVICE_NAME` | `PrivShield` | 服务名（日志/Tracing） |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | OpenTelemetry OTLP 端点 |
| `OTEL_SERVICE_NAME` | — | Tracing 服务名（覆盖 PRIVACY_SERVICE_NAME） |
| `PRIVACY_TLS_ENABLED` | `false` | 启用 TLS |
| `PRIVACY_TLS_CERT_FILE` | — | 证书路径 |
| `PRIVACY_TLS_KEY_FILE` | — | 私钥路径 |
| `PRIVACY_TLS_CA_FILE` | — | CA 证书路径（mTLS） |
| `PRIVACY_TLS_CLIENT_AUTH` | `none` | 客户端认证：none/optional/require |
| `PRIVACY_TLS_KEY_PASSWORD` | — | 私钥密码 |
| `PRIVACY_AUTH_ENABLED` | `false` | 启用 API Key 认证 |
| `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` | `{}` | 外部 Key JSON |
| `PRIVACY_AUTH_INTERNAL_KEYS_JSON` | `{}` | 内部 Key JSON |
| `PRIVACY_RATE_LIMIT_ENABLED` | `false` | 启用速率限制 |
| `PRIVACY_RATE_LIMIT_DEFAULT_RPS` | `10` | 默认 RPS |
| `PRIVACY_RATE_LIMIT_DEFAULT_BURST` | `20` | 默认突发 |
| `PRIVACY_RATE_LIMIT_REDIS_URL` | — | Redis 限流后端 |
| `PRIVACY_LLM_PROVIDER` | `auto` | LLM 后端：`qwen3`（集成模式本地推理）/ `vllm` / `openai`（解耦模式 HTTP）/ `mlx` |
| `PRIVACY_LLM_API_BASE` | — | 外部 vLLM/OpenAI 端点（解耦模式必配，如 `http://vllm:8000/v1`） |
| `PRIVACY_LLM_MODEL_NAME` | — | 模型对外标识，须与 vLLM `--served-model-name` 一致 |
| `PRIVACY_LLM_API_KEY` | `EMPTY` | 外部服务 API Key |
| `PRIVACY_LLM_MODEL_PATH` | `.models/Qwen3.5-...` | 本地模型路径（集成模式） |
| `PRIVACY_LLM_DEVICE` | `auto` | 本地推理设备：`cuda` / `cpu` / `mps` |
| `PRIVACY_LLM_ENABLE` | `false` | 显式启用 Layer-3 LLM 层（默认按置信度触发） |
| `PRIVACY_NER_ENABLE` | `false` | 启用 Layer-2 Small-NER 实体抽取 |
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` | 进程级 LLM 推理并发上限（信号量） |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | `30` | 信号量排队超时（秒），超时降级 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | 推理前可用内存阈值（MB），低于则降级 |
| `PRIVACY_WARMUP_LLM` | `false` | 启动时异步预热 LLM |
| `PRIVACY_VLM_TIMEOUT` | `180` | VLM 推理超时（秒） |
| `PRIVACY_HEALTH_NO_AUTH` | `true` | 健康检查跳过认证 |
| `PRIVACY_HEALTH_NO_RATE_LIMIT` | `true` | 健康检查跳过限速 |

---

## 14. 验证与冒烟测试

### 14.1 K8s 环境验证

```bash
# 查看 Pod 状态
kubectl get pods -n PrivShield -l app=PrivShield

# 查看启动日志
kubectl logs -n PrivShield deploy/PrivShield -f

# 端口转发
kubectl port-forward -n PrivShield svc/PrivShield 8079:8079 50051:50051

# 健康检查
curl http://localhost:8079/health
# 期望：{"status":"ok","namespace":"default"}

# 就绪探针
curl http://localhost:8079/readyz
# 期望：{"status":"ready"}

# 存活探针
curl http://localhost:8079/livez
# 期望：{"status":"alive"}

# Prometheus 指标
curl http://localhost:8079/metrics | head -20

# 脱敏接口冒烟
curl -X POST http://localhost:8079/v1/privacy/mask \
  -H "Content-Type: application/json" \
  -d '{"field_name": "mobile", "value": "13800138000", "context": "medical"}'

# DP 查询冒烟
curl -X POST http://localhost:8079/v1/privacy/dp/count \
  -H "Content-Type: application/json" \
  -d '{"values": [1, 0, 1, 1, 0], "params": {"epsilon": 1.0}}'
```

### 14.2 TLS 环境验证

```bash
# 跳过证书验证
curl -k https://localhost:8079/health

# 指定 CA 证书
curl --cacert ca.crt https://localhost:8079/health

# 携带 API Key
curl -k -H "X-API-Key: your-api-key" https://localhost:8079/health
```

### 14.3 Docker Compose 验证

```bash
cd deploy/docker-compose

# 查看容器状态与健康检查
docker compose ps

# 查看日志
docker compose logs -f PrivShield

# 冒烟测试
curl http://localhost:8079/health
```

---

## 15. 故障排查

| 现象 | 可能原因 | 排查步骤 |
|---|---|---|
| Pod CrashLoopBackOff | TLS 证书路径错误或 Profile YAML 语法错误 | `kubectl logs deploy/privacy-local-agent` 查看启动异常 |
| Pod Pending | 资源不足 / 节点亲和不满足 | `kubectl describe pod <name>` 查看 Events |
| 健康检查失败（liveness） | 端口未监听 / 安全中间件拦截 | 确认 `/health` 在 `publicPaths` 白名单中；检查 `PRIVACY_HEALTH_NO_AUTH=true` |
| 就绪探针 503 | 配置解析器未初始化 / SQLite DB 不可达 | 检查 `PRIVACY_PROFILE` 路径是否正确挂载；检查 `PRIVACY_BUDGET_DB` 文件权限 |
| gRPC 调用失败 | Service 端口 / TLS 设置不一致 | 确认 Service 暴露 50051；TLS 模式下客户端需使用 TLS channel |
| 认证 401 | API Key 未配置或格式错误 | 检查 `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` 是否为合法 JSON；Header 使用 `X-API-Key` |
| 速率限制 429 | RPS 超限 | 调大 `PRIVACY_RATE_LIMIT_DEFAULT_RPS` 或配置 `PER_ENDPOINT` 覆盖 |
| 隐私预算拒绝 | 预算耗尽 | 查看 `privacy_budget_remaining` 指标；配置 `PRIVACY_BUDGET_WINDOW_SECONDS` 自动重置 |
| OOMKilled | 内存不足（尤其 ML 镜像） | 调大 `resources.limits.memory`；ML 建议至少 8Gi |
| 分类延迟过高 | LLM 推理慢 / 模型未预热 | 启用 `PRIVACY_WARMUP_LLM=true`；查看 `privacy_classification_llm_duration_seconds` |
| 多实例预算不一致 | 使用内存预算后端 | 配置 `PRIVACY_BUDGET_DB` 使用 SQLite 持久化 |

### 15.1 Docker Compose 常见故障（本地/单机场景）

| 现象 | 可能原因 | 排查/解决 |
|---|---|---|
| `docker compose up` 报 `no such file` | 当前目录不在 `deploy/docker-compose/` | `cd deploy/docker-compose`，或加 `-f deploy/docker-compose/docker-compose.yml` |
| 拉镜像超时 `i/o timeout` | 国内网络直连 Docker Hub 失败 | 见 [6.2](#62-解耦模式外部独立-vllm-服务) 的 DaoCloud 拉取方案或配置镜像加速器 |
| 容器反复重启（Restarting 状态） | 启动参数错误 / 端口被占 / 健康检查失败 | `docker compose logs <服务名>` 看报错；`docker compose ps` 看状态 |
| 端口被占 `address already in use` | 宿主机已有进程占用 8079 等端口 | `ss -tlnp | grep 8079` 找占用进程；或改 compose `ports:` 映射 |
| vllm 启动失败 `CUDA error` / `no GPU` | 未装 NVIDIA 驱动 / nvidia-container-toolkit / 无 GPU | `nvidia-smi` 验证；无 GPU 时不要加 `--profile llm` |
| vllm 返回 404 `model not found` | `--served-model-name` 与 `PRIVACY_LLM_MODEL_NAME` 不一致 | 两处改为完全一致后 `docker compose up -d vllm` |
| agent 健康检查一直失败 | `/readyz` 503：`PRIVACY_PROFILE` 路径未正确挂载 | `docker compose exec privacy-local-agent ls /etc/privacy-local-agent` |
| 改了 `.env` 但不生效 | 只 restart 未重建容器 | `docker compose up -d`（restart 不会重新注入环境变量） |
| Web 控制台 502 | agent 未就绪或后端代理未启动 | 等 healthcheck 通过；`docker compose ps` 看全部状态 |
| 磁盘被日志/卷占满 | 日志轮转 max-size 偏大或卷增长 | 日志：调小 `logging.options.max-size`；卷：确认数据可备份后 `down -v` |
| `curl 127.0.0.1:8000` 无响应 / 连接代理后 502 | ① `llm` 网络为 `internal: true` 导致 `ports:` 未映射；② 宿主机 `http_proxy` 把 localhost 请求转发到代理 | ① `llm` 网络改为 `internal: false`，`ports` 改为 `127.0.0.1:8000:8000`；② 访问本地地址时绕过代理（如 `no_proxy=127.0.0.1,localhost` 或代码显式禁用代理） |
| vllm 容器状态 `unhealthy` | 健康检查命令用了 `python` 而非 `python3` | 检查 `docker-compose.yml` healthcheck 的 `test` 数组，确保使用 `python3`；`docker inspect <容器> --format '{{.State.Health}}'` 查看失败日志 |

#### 15.1.1 vLLM Docker 服务测试失败排查案例

> 适用场景：运行 `tests/scripts/test_docker_start_llm.py` 的 integration 测试时，vLLM 容器虽已启动，但 `_wait_vllm_ready()` 在 600s 内始终未等到 `/v1/models` 响应；或测试通过容器启动阶段，却在真实 chat/classify 任务中失败。

**现象**：

- `docker ps` 显示 vLLM 容器 `Up ... (unhealthy)`，且 `docker inspect` 中 `State.Health.Status` 为 `unhealthy`，失败日志反复出现 `exec: "python": executable file not found in $PATH`
- `docker inspect <容器> --format '{{json .NetworkSettings.Ports}}'` 输出 `"8000/tcp": null`，宿主机 `curl 127.0.0.1:8000/v1/models` 无响应或经过本地代理返回 502
- 集成测试在 `TestVllmServiceIntegration` 阶段报 `vLLM 服务未在 600s 内就绪`，或 LLM 定级结果不符合预期（如缺少 `confidence`、HIV 文本被定级为 L3、公开统计被定级为 L4）

**根因分析**（按发现顺序）：

| 序号 | 根因 | 说明 |
|---|---|---|
| 1 | `llm` 网络设为 `internal: true` | Docker Compose 中，服务若**只** attached 到 `internal: true` 的网络，即使写了 `ports:` 也不会创建宿主机端口映射，导致 `127.0.0.1:8000` 无法访问 |
| 2 | healthcheck 使用 `python` | `vllm/vllm-openai:latest` 基于 Ubuntu 24.04，镜像内只有 `python3`，没有 `python` 命令；错误的 healthcheck 让容器永远 `unhealthy` |
| 3 | 宿主机 `http_proxy` 劫持 localhost | 测试脚本用 `urllib.request` 访问 `127.0.0.1:8000` 时，如果环境变量 `http_proxy` 已设置，请求会被转发到本地代理（如 127.0.0.1:7897），代理再连 127.0.0.1:8000 失败返回 502 |
| 4 | 测试 fixture 清理不够健壮 | 原 fixture 在 `_wait_vllm_ready()` 失败后虽尝试 `docker rm -f`，但未检查返回值，失败时容器残留，可能污染后续测试 |
| 5 | `OpenAILlmClassifier` prompt 与微调模型不一致 | 通过 HTTP 调用 vLLM 时默认使用通用 prompt，而项目微调的 `Qwen3.5-0.8B-Privacy-Classifier-Smoother` 需要与训练侧一致的 system prompt 和裸用户文本，否则输出 JSON 字段缺失或定级漂移 |

**排查过程**（可复现）：

```bash
# 1. 确认端口是否真的映射到了宿主机
 docker inspect --format='{{json .NetworkSettings.Ports}}' PrivShield-vllm
 # 正常应显示：{"8000/tcp":[{"HostIp":"127.0.0.1","HostPort":"8000"}]}
 # 若为 null，说明 internal 网络阻止了映射

# 2. 确认网络是否为 internal
 cd deploy/docker-compose
 docker compose --profile llm config | grep -A3 'llm:'

# 3. 确认健康检查命令
 docker inspect --format='{{json .State.Health}}' PrivShield-vllm | python3 -m json.tool
 # 若日志里有 "python": executable file not found → 需改成 python3

# 4. 确认本地代理是否干扰
 env | grep -i proxy
 curl -v --max-time 5 http://127.0.0.1:8000/v1/models
 # 若看到 Trying 127.0.0.1:7897 或 502 Bad Gateway → 代理在转发 localhost

# 5. 直接测试 vLLM 容器内服务是否已就绪
 docker exec PrivShield-vllm python3 -c \
   "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=5).status)"
```

**解决方案**（对应上述根因）：

| 序号 | 修改文件 | 修改内容 |
|---|---|---|
| 1 | `deploy/docker-compose/docker-compose.yml` | 将 `llm` 网络 `internal: true` 改为 `internal: false`；将 vllm `ports` 从 `"8000:8000"` 改为 `"127.0.0.1:8000:8000"` |
| 2 | `deploy/docker-compose/docker-compose.yml` | healthcheck `test` 数组中 `python` 改为 `python3` |
| 3 | `tests/scripts/test_docker_start_llm.py` | `_http_get_json` / `_http_post_json` 使用 `urllib.request.build_opener(urllib.request.ProxyHandler({}))` 显式禁用代理 |
| 4 | `tests/scripts/test_docker_start_llm.py` | 重构 `vllm_service` fixture：使用 `try/finally`、检查容器删除结果、端口映射失败时附加日志诊断 |
| 5 | `privacy_local_agent/dynclassification/llm_engines.py` | `OpenAILlmClassifier` 新增 `_is_finetuned_model()`，匹配微调模型名时复用 `_FINETUNED_SYSTEM_PROMPT` 和裸用户文本 |

**验证命令**：

```bash
# 非集成测试（无 Docker/GPU 依赖）
PYTHONPATH=. pytest tests/scripts/test_docker_start_llm.py -m "not integration" -v

# 全量测试（含 Docker + GPU + 真实 vLLM 推理）
PYTHONPATH=. pytest tests/scripts/test_docker_start_llm.py -v
# 预期结果：28 passed
```

**要点**：

- Docker 的 `internal` 网络是“禁止容器主动访问外部”的语义，但它同时会阻止 Docker 为该服务创建**宿主机端口映射**；如果业务上需要宿主机调试端口，必须让服务所在网络为非 internal，或让服务同时属于一个非 internal 网络。
- 容器镜像的 `/usr/bin/python` 软链接在新版基础镜像中可能被移除，任何进入容器执行的命令（healthcheck、自定义入口、测试探测）都应显式使用 `python3`。
- 宿主机代理环境变量对 `127.0.0.1` / `localhost` 的处理因客户端而异：curl 可能绕过，Python `urllib.request` 可能不绕过；访问本地容器端口时应在代码中显式禁用代理，或设置 `no_proxy=127.0.0.1,localhost`。
- 0.8B 微调模型对 prompt 分布非常敏感，HTTP 调用时应使用与训练样本一致的 system prompt，否则会出现 JSON 字段缺失或定级漂移。

**常用诊断命令**：

```bash
# 查看 Pod 事件
kubectl describe pod -n PrivShield -l app=PrivShield

# 进入容器调试
kubectl exec -it -n PrivShield deploy/PrivShield -- /bin/sh

# 容器内测试端口
curl http://localhost:8079/health
curl http://localhost:8079/metrics | grep privacy_budget

# 查看 ConfigMap 内容
kubectl get configmap PrivShield-config -n PrivShield -o yaml

# 查看 Secret（base64 编码）
kubectl get secret pla-tls -n PrivShield -o yaml
```

### 15.2 WSL 中 Docker GPU 失效（真实排查案例）

> 适用场景：WSL2 + Ubuntu，`docker run --gpus 1 ...` 报错，但宿主机 `nvidia-smi` 完全正常。

**现象**：

- `docker run --gpus 1 ...` 报错：`nvidia-container-cli: initialization error: load library failed: libnvidia-ml.so.1: cannot open shared object file`
- 带 GPU 前置条件的测试自动 skip（例如 `tests/scripts/test_docker_start_llm.py` 的集成测试）
- 宿主机 `nvidia-smi` 正常、`ldconfig -p | grep libnvidia-ml` 能找到驱动库 → 驱动本身没坏

**根因**：Docker 是通过 **snap** 安装的（`snap list docker`）。snap 严格沙箱内自带的 `nvidia-container-cli`（位于 `/snap/docker/<rev>/usr/bin/`）**无法读取 WSL 的 GPU 驱动目录 `/usr/lib/wsl/lib`**——`snap connections docker` 显示 `gpu-2404` / `graphics-core22` 接口的 slot 均为空（WSL 下没有 GPU provider snap）。与容器镜像无关，换任何镜像都一样报错。

**排查命令**（按序执行可复现定位）：

```bash
# 1. Docker 是否注册了 nvidia runtime？→ 只有 runc，没有 nvidia
docker info | grep -A3 Runtimes

# 2. Docker 是不是 snap 装的？
snap list docker

# 3. snap 的 GPU 接口是否可用？→ slot 为空 = snap 无法接触 GPU
snap connections docker

# 4. nvidia-container-cli 在哪？→ 只存在于 snap 沙箱目录内
find /usr -name "nvidia-container-cli*"

# 5. 宿主驱动是否正常？→ 正常，说明问题在 Docker 沙箱而不是驱动
nvidia-smi
ls /usr/lib/wsl/lib/          # WSL 驱动库存在（libnvidia-ml.so.1 等）
ldconfig -p | grep libnvidia-ml
```

**解决**（snap docker → docker.io daemon + nvidia-container-toolkit）：

| 步骤 | 命令 | 说明 |
|---|---|---|
| 1 | `docker save vllm/vllm-openai:latest -o ~/vllm-image.tar` | 先备份镜像资产到新 daemon 能读到的地方。**踩坑**：保存路径必须有足够磁盘空间——`/tmp` 是 tmpfs（仅 7.7G），vllm 镜像约 8.5G，`docker save` 会报 `no space left on device`，应保存到磁盘分区（如家目录，`df -h` 先确认） |
| 2 | `sudo snap stop docker && sudo snap disable docker` | 停用 snap daemon，释放 `/var/run/docker.sock` |
| 3 | `sudo apt-get install -y nvidia-container-toolkit docker-compose-v2` | 安装 GPU 注入工具链与 compose 插件（Ubuntu 官方源自带，无需加第三方源） |
| 4 | `sudo nvidia-ctk runtime configure --runtime=docker` | 把 `nvidia` runtime 注册进 `/etc/docker/daemon.json` |
| 5 | `sudo systemctl start docker` | 启动 docker.io daemon；`docker version` 的 Server 版本应变回 apt 版本（如 29.1.3） |
| 6 | `docker load -i ~/vllm-image.tar` | 把镜像恢复到新 daemon |
| 7 | `docker run --rm --gpus 1 --entrypoint python3 vllm/vllm-openai:latest -c "import torch; print(torch.cuda.is_available())"` | 验证 GPU 注入，输出 `True` 即成功（注意用 `python3`，新版 vllm 镜像无 `python` 命令） |

**验证三连**：

```bash
docker version --format "Server: {{.Server.Version}}"   # 应为 apt 版（如 29.1.3）而非 snap 版（29.6.1）
docker info | grep -A3 Runtimes                          # 应出现 nvidia runtime
docker run --rm --gpus 1 <镜像> python3 -c "import torch; print(torch.cuda.is_available())"   # → True
```

**要点**：

- docker.io 与 snap docker 的**数据目录不同**（`/var/lib/docker` vs `/var/snap/docker/...`），**容器不互通**；镜像用 save/load 迁移，运行中的容器需另行重建。
- 安装 `docker-compose-v2` 后 `docker compose`（空格版）直接可用；之前用软链（`~/.docker/cli-plugins/docker-compose` → snap 版）只是临时方案，修复后应移除。
- WSL 的 GPU 驱动库在宿主 `/usr/lib/wsl/lib`，docker.io daemon 没有 snap 沙箱限制，配合 nvidia-container-toolkit 即可正常注入 GPU。
- **新版 vllm 官方镜像（Ubuntu 24.04 base）只有 `python3`、没有 `python` 命令**，用 `--entrypoint python` 会报 `exec: "python": executable file not found in $PATH`；GPU 探测、自定义入口统一用 `python3`（本机实测 `CUDA: True`）。
- **根因判定线索**（防误判）：驱动/CUDA 全正常、唯独容器内拿不到 GPU → 优先怀疑 Docker 运行时的沙箱限制，而不是镜像或驱动问题。

---

## 16. 日常运维操作

### 16.1 扩缩容

```bash
# 手动扩容（HPA 关闭时）
kubectl scale deploy/PrivShield -n PrivShield --replicas=3

# 查看 HPA 状态（HPA 开启时）
kubectl get hpa -n PrivShield
```

### 16.2 滚动更新

```bash
# 更新镜像版本
kubectl set image deploy/PrivShield \
  agent=myregistry/PrivShield:0.2.0 \
  -n PrivShield

# 查看滚动状态
kubectl rollout status deploy/PrivShield -n PrivShield

# 回滚
kubectl rollout undo deploy/PrivShield -n PrivShield
```

### 16.3 配置变更

```bash
# 编辑 ConfigMap
kubectl edit configmap PrivShield-config -n PrivShield

# 重启 Pod 使配置生效（ConfigMap 更新不会自动触发滚动）
kubectl rollout restart deploy/PrivShield -n PrivShield
```

### 16.4 证书轮换

```bash
# 更新 TLS Secret
kubectl create secret tls pla-tls \
  --cert=new-tls.crt --key=new-tls.key \
  -n PrivShield --dry-run=client -o yaml | kubectl apply -f -

# 重启 Pod 加载新证书
kubectl rollout restart deploy/PrivShield -n PrivShield
```

### 16.5 日志查看

```bash
# 实时日志
kubectl logs -f -n PrivShield deploy/PrivShield

# 最近 100 行
kubectl logs --tail=100 -n PrivShield deploy/PrivShield

# JSON 格式日志过滤（生产模式 logFormat=json）
kubectl logs -n PrivShield deploy/PrivShield | jq '.level == "error"'
```

### 16.6 Helm 预检查（CI/CD）

```bash
# Chart 语法检查
make helm-lint

# 渲染模板（不实际安装）
make helm-template

# 自定义 values 渲染
helm template pla ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=pla-tls
```
