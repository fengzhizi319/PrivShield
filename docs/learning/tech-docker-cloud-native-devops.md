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
