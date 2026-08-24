# 从本地代码到 Kubernetes：PrivShield 部署入门

> 目标读者：刚接触 K8s / Helm，希望把本地跑通的 `PrivShield` 代码部署到 Kubernetes 的同学。
> 读完本文，你应该能：
> 1. 理解“本地 Python 代码 → 容器镜像 → K8s Pod” 的完整链路；
> 2. 用 Helm 或原生 manifests 把服务跑在 K8s 上；
> 3. 能排查最常见的启动失败问题。

---

## 目录

- [1. 一句话总结](#1-一句话总结)
- [2. 前置准备](#2-前置准备)
- [3. 本地代码如何变成容器镜像](#3-本地代码如何变成容器镜像)
  - [3.1 Dockerfile 多阶段构建](#31-dockerfile-多阶段构建)
  - [3.2 构建镜像](#32-构建镜像)
  - [3.3 本地运行验证](#33-本地运行验证)
- [4. 镜像怎么进入 Kubernetes](#4-镜像怎么进入-kubernetes)
  - [4.1 需要一个镜像仓库](#41-需要一个镜像仓库)
  - [4.2 minikube / kind 本地加载镜像](#42-minikube--kind-本地加载镜像)
  - [4.3 真实集群：推送镜像](#43-真实集群推送镜像)
- [5. Kubernetes 部署的三种方式](#5-kubernetes-部署的三种方式)
- [6. 最小可运行 K8s 部署：原生 manifests](#6-最小可运行-k8s-部署原生-manifests)
  - [6.1 资源清单拆解](#61-资源清单拆解)
  - [6.2 部署与验证](#62-部署与验证)
- [7. 生产级 Helm 部署](#7-生产级-helm-部署)
  - [7.1 Helm Chart 结构](#71-helm-chart-结构)
  - [7.2 开发模式安装](#72-开发模式安装)
  - [7.3 生产模式安装（TLS + 认证）](#73-生产模式安装tls--认证)
  - [7.4 升级与回滚](#74-升级与回滚)
- [8. 可观测性：Prometheus + Grafana](#8-可观测性prometheus--grafana)
- [9. 常见问题排查](#9-常见问题排查)
- [10. 命令速查表](#10-命令速查表)
- [11. 延伸阅读](#11-延伸阅读)

---

## 1. 一句话总结

`PrivShield` 本质是一个 Python 服务。本地开发时你直接运行 Python；部署到 K8s 时，我们先把代码和依赖打包进 Docker 镜像，再通过 **Deployment** 让 K8s 运行这个镜像的多个副本，用 **Service** 暴露 REST/gRPC 端口，用 **ConfigMap/Secret** 注入配置和证书。

```text
本地代码
   │  docker build
   ▼
Docker 镜像
   │  push / load
   ▼
K8s 集群
   │  helm install / kubectl apply
   ▼
Pod → Service → Ingress(可选)
```

---

## 2. 前置准备

| 工具 | 作用 | 最低版本 |
|---|---|---|
| Docker | 构建、运行镜像 | 20.10+ |
| kubectl | 与 K8s 集群交互 | 1.25+ |
| Helm | 安装/管理 Chart | 3.12+ |
| 一个 K8s 集群 | 运行服务 | — |

如果你还没有集群，学习阶段推荐：

- **minikube**（单节点，本地 VM）
- **kind**（在 Docker 里跑 K8s，轻量）
- **microk8s**（Ubuntu 推荐）

生产环境请使用真实的多节点 K8s 集群。

---

## 3. 本地代码如何变成容器镜像

### 3.1 Dockerfile 多阶段构建

项目根目录的 `Dockerfile` 采用多阶段构建：

```text
base（安装系统依赖 + 核心 Python 依赖）
  ├──► core（隐私原语：脱敏、DP、K-匿名、规则分类）
  └──► ml（core + torch/transformers/onnxruntime，支持 NER/LLM）
```

- `core` 镜像小（~350 MB），适合大多数场景。
- `ml` 镜像大（~4 GB+），只有需要本地大模型时才用。

关键行为：

- 容器内默认监听 `0.0.0.0:8079`（REST）和 `0.0.0.0:50051`（gRPC）。
- 入口脚本是 `docker-entrypoint.sh`，最终执行 `python -m engine.server`。

### 3.2 构建镜像

在项目根目录执行：

```bash
# 构建 core 镜像（推荐默认）
docker build --target core -t PrivShield:0.1.0 .

# 构建 ml 镜像
docker build --target ml -t PrivShield:0.1.0-ml .

# 也可以用 Makefile
make docker-core
make docker-ml
```

构建完成后查看镜像：

```bash
docker images | grep PrivShield
```

### 3.3 本地运行验证

```bash
# 本地运行 core 镜像
docker run -p 8079:8079 -p 50051:50051 PrivShield:0.1.0

# 另开一个终端测试健康检查
curl http://localhost:8079/health
```

如果看到 `{"status":"ok","namespace":"default"}`，说明镜像没问题。

---

## 4. 镜像怎么进入 Kubernetes

K8s 节点要从某个地方拉取镜像，通常有两种方式：

### 4.1 需要一个镜像仓库

生产环境一般使用：

- Docker Hub
- 阿里云 ACR
- 腾讯云 TCR
- Harbor 私有仓库
- GitHub Container Registry

### 4.2 minikube / kind 本地加载镜像

如果你用 minikube：

```bash
# 让 Docker 使用 minikube 内部的 Docker daemon
eval $(minikube docker-env)

# 重新构建镜像（此时镜像会存在 minikube 内部）
docker build --target core -t PrivShield:0.1.0 .

# 退出 minikube docker-env 后，kubectl 就能看到镜像
```

如果你用 kind：

```bash
kind load docker-image PrivShield:0.1.0 --name <你的集群名>
```

### 4.3 真实集群：推送镜像

```bash
# 1. 给镜像打仓库标签
docker tag PrivShield:0.1.0 myregistry.example.com/PrivShield:0.1.0

# 2. 推送
docker push myregistry.example.com/PrivShield:0.1.0
```

在 K8s 部署时，把 `image.repository` 和 `image.tag` 改成这个地址。

---

## 5. Kubernetes 部署的三种方式

本项目提供三种部署形态：

| 方式 | 路径 | 适用场景 | 难度 |
|---|---|---|---|
| **Helm Chart** | `deploy/helm/PrivShield/` | 生产/需要灵活配置 | 中 |
| **原生 K8s manifests** | `deploy/k8s/` | 学习/最小化/不想用 Helm | 低 |
| **Docker Compose** | `deploy/docker-compose/` | 本地联调，不是 K8s | 低 |

**建议学习顺序**：

1. 先跑通 Docker Compose（本地验证代码和镜像）。
2. 再用 `deploy/k8s/` 理解每个 K8s 资源的作用。
3. 最后用 Helm 体验生产级配置（TLS、认证、HPA、NetworkPolicy）。

---

## 6. 最小可运行 K8s 部署：原生 manifests

`deploy/k8s/` 是 Kustomize 组织的一组最小清单，适合学习。

### 6.1 资源清单拆解

```text
deploy/k8s/
├── namespace.yaml          # 创建 PrivShield 命名空间
├── configmap.yaml          # 挂载 privacy-profile.yaml（非敏感配置）
├── deployment.yaml         # 运行 Pod：镜像、端口、环境变量、探针
├── service.yaml            # ClusterIP：暴露 8079/50051
├── secret.example.yaml     # TLS 证书 + API Key 示例（需自己填值）
└── kustomization.yaml      # Kustomize 入口
```

#### Namespace

把资源隔离在一个命名空间里，方便管理。

#### ConfigMap

把 `privacy-profile.yaml` 内容放进 ConfigMap，再挂载到容器 `/etc/PrivShield/`。

> 注意：代码只读取 `primitives:` 段，其他顶层键（如 `server:`）无效。

#### Deployment

最核心的资源，告诉 K8s：

- 用什么镜像；
- 启动几个副本；
- 监听什么端口；
- 环境变量（如 `PRIVACY_PROFILE`、`PRIVACY_LOG_FORMAT`）；
- 健康检查：`/health` 存活，`/readyz` 就绪；
- 资源限制：避免某个 Pod 吃光节点资源。

#### Service

`ClusterIP` 类型表示“只能在集群内部访问”。Pod 的 IP 会变，Service 提供稳定的虚拟 IP 和 DNS。

REST 调用：

```text
PrivShield.PrivShield.svc:8079
```

gRPC 调用：

```text
PrivShield.PrivShield.svc:50051
```

#### Secret（可选）

TLS 证书和 API Key 属于敏感信息，不应该写进镜像或 ConfigMap，要用 Secret。默认 `secret.example.yaml` 没有启用，需要复制并填真实值。

### 6.2 部署与验证

```bash
# 1. 确保镜像已在集群可用（minikube docker-env / kind load / push 到仓库）

# 2. 一键部署
cd /path/to/PrivShield
kubectl apply -k deploy/k8s/

# 3. 查看 Pod 状态
kubectl get pods -n PrivShield -w

# 4. 等 Pod Running 后，端口转发到本地测试
kubectl port-forward -n PrivShield svc/PrivShield 8079:8079 50051:50051

# 5. 本地测试
curl http://localhost:8079/health
curl http://localhost:8079/readyz
```

---

## 7. 生产级 Helm 部署

Helm 把 K8s 资源模板化，允许你通过 `values.yaml` 灵活配置，而不用直接改 YAML。

### 7.1 Helm Chart 结构

```text
deploy/helm/PrivShield/
├── Chart.yaml              # Chart 元数据
├── values.yaml             # 默认值（开发模式）
├── values-production.yaml  # 生产覆盖值
├── values-ml.yaml          # ML 镜像覆盖值
└── templates/              # 模板，会被渲染成真实 K8s YAML
    ├── _helpers.tpl        # 命名/标签辅助函数
    ├── namespace.yaml
    ├── serviceaccount.yaml
    ├── deployment.yaml
    ├── service.yaml
    ├── configmap.yaml
    ├── secret.yaml
    ├── ingress.yaml
    ├── hpa.yaml
    ├── poddisruptionbudget.yaml
    ├── networkpolicy.yaml
    └── servicemonitor.yaml
```

### 7.2 开发模式安装

```bash
# 1. 构建镜像并确保集群能访问
# 2. 安装 Chart
helm install privshield ./deploy/helm/PrivShield

# 查看状态
helm list
kubectl get pods -l app.kubernetes.io/name=PrivShield
```

默认行为：

- 1 个副本；
- TLS / Auth / RateLimit 关闭；
- Service 暴露 REST + gRPC；
- HPA / Ingress / NetworkPolicy / ServiceMonitor 关闭。

### 7.3 生产模式安装（TLS + 认证）

```bash
# 1. 创建 TLS Secret（包含 tls.crt / tls.key）
kubectl create secret tls privshield-tls \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  -n PrivShield

# 2. 创建 API Key Secret
# 文件 api-keys.json 示例：
# {
#   "my-api-key": { "name": "gateway", "scopes": ["*"] }
# }
kubectl create secret generic privshield-apikeys \
  --from-file=api-keys.json=path/to/api-keys.json \
  -n PrivShield

# 3. 使用生产 values 安装
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys \
  --set image.repository=myregistry.example.com/PrivShield \
  --set image.tag=0.1.0
```

生产模式会同时启用：

- 2 副本；
- TLS + API Key 认证；
- 速率限制；
- HPA（2~10 副本）；
- NetworkPolicy；
- ServiceMonitor（需要 Prometheus Operator）。

### 7.4 升级与回滚

```bash
# 升级镜像版本
helm upgrade privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set image.tag=0.2.0

# 查看历史
helm history privshield

# 回滚到上一版本
helm rollback privshield
```

---

## 8. 可观测性：Prometheus + Grafana

项目内置 `/metrics` 端点，暴露 `privacy_*` 前缀的指标。

```bash
# 本地测试指标
curl http://localhost:8079/metrics | head -20
```

### 8.1 Prometheus 抓取

如果使用 Prometheus Operator，在 Helm 安装时启用 ServiceMonitor：

```bash
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set serviceMonitor.enabled=true
```

### 8.2 Grafana 仪表盘

`deploy/grafana/dashboard.json` 是预置仪表盘，导入 Grafana：

```text
Grafana → Dashboards → Import → 上传 deploy/grafana/dashboard.json
```

### 8.3 Docker Compose 监控栈

本地学习时可以直接启动 Prometheus + Grafana：

```bash
cd deploy/docker-compose
docker compose --profile monitoring up -d
```

---

## 9. 常见问题排查

| 现象 | 可能原因 | 排查命令 |
|---|---|---|
| `ImagePullBackOff` | 镜像未推送到仓库 / 标签错误 | `kubectl describe pod -n PrivShield <pod>` |
| `CrashLoopBackOff` | 配置文件路径错误 / TLS 证书缺失 | `kubectl logs -n PrivShield deploy/PrivShield` |
| 健康检查失败 | 端口未监听 / 探针路径错误 | 确认 `PRIVACY_HEALTH_NO_AUTH=true`，Service 暴露 8079 |
| 就绪探针 503 | `PRIVACY_PROFILE` 未挂载 / SQLite DB 不可写 | `kubectl get configmap -n PrivShield` |
| 认证 401 | API Key 不匹配 | 检查 Secret 中的 `api-keys.json`，请求头 `X-API-Key` |
| 速率限制 429 | RPS 超过限制 | 调大 `PRIVACY_RATE_LIMIT_DEFAULT_RPS` |
| OOMKilled | 内存不足 | 调大 `resources.limits.memory`；ml 镜像建议 ≥8Gi |

通用诊断流程：

```bash
# 1. 看 Pod 事件
kubectl describe pod -n PrivShield -l app=PrivShield

# 2. 看日志
kubectl logs -n PrivShield deploy/PrivShield

# 3. 进容器内部验证
kubectl exec -it -n PrivShield deploy/PrivShield -- /bin/sh
curl http://localhost:8079/health
```

---

## 10. 命令速查表

```bash
# ── 镜像构建 ──
docker build --target core -t PrivShield:0.1.0 .
docker build --target ml -t PrivShield:0.1.0-ml .

# ── 本地运行 ──
docker run -p 8079:8079 -p 50051:50051 PrivShield:0.1.0

# ── 镜像推送 ──
docker tag PrivShield:0.1.0 myregistry/PrivShield:0.1.0
docker push myregistry/PrivShield:0.1.0

# ── 原生 K8s ──
kubectl apply -k deploy/k8s/
kubectl get pods -n PrivShield -w
kubectl port-forward -n PrivShield svc/PrivShield 8079:8079

# ── Helm ──
helm install privshield ./deploy/helm/PrivShield
helm upgrade privshield ./deploy/helm/PrivShield -f ./deploy/helm/PrivShield/values-production.yaml
helm rollback privshield

# ── 验证 ──
curl http://localhost:8079/health
curl http://localhost:8079/readyz
curl http://localhost:8079/metrics | head -20
```

---

## 11. 延伸阅读

- [Deployment PRD](./prd.md) — 产品需求与验收标准
- [Deployment Design](./design.md) — 架构选型、Chart 结构、滚动更新策略
- [Deployment Ops](./ops.md) — 完整运维手册、环境变量参考、故障排查
- [Deployment Examples](./examples.md) — Helm/K8s/Docker Compose 详细示例
- [Deployment Testing](./testing.md) — 部署验证与 CI 建议
- [生产安全 Ops](../production_security/ops.md) — TLS、认证、速率限制配置
- [可观测性 Ops](../production_observability/ops.md) — Prometheus/Grafana/Tracing 配置
