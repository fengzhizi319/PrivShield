# K8s/Helm 云原生部署产品需求文档 (PRD)

---

## 1. 概述

本文档定义 `PrivShield` 云原生生产部署与交付的产品需求、核心规范与验收标准。平台基于 **纯 Go 1.25+ 云原生架构** 实现，通过 Helm Chart、原生 Kubernetes manifests（Kustomize）与 Docker Compose 三种交付形态，支持极简 Alpine 运行时（~25MB）与 CUDA 加速运行时、TLS/mTLS 零信任安全、动态配置热加载、多副本高可用及弹性伸缩。

---

## 2. 设计目标

- **极简超轻量镜像**：基于 `golang:1.25-alpine3.21` 多阶段编译，生成单个静态二进制，镜像体积控制在 **~25MB**，内存占用 **< 50MB**，冷启动 **< 50ms**。
- **全栈 Helm 编排**：提供企业级 Helm Chart（支持 `engineType: go`），集成 Go Worker Deployment、Service、ConfigMap、Secret、Ingress、HPA 与 NetworkPolicy。
- **原生 K8s Kustomize 支持**：提供自包含、开箱即用的原生 K8s 清单（`deploy/k8s/`）。
- **零信任安全基线**：非 root 用户运行（`USER privacy:privacy`），只读文件系统，支持 TLS/mTLS 证书与 API Key 经 Secret 动态注入与热轮转。
- **弹性与自愈**：双协议健康探针（`/health` liveness 与 `/readyz` readiness），支持 HPA 基于 CPU/内存进行秒级弹性扩缩容。

---

## 3. 用户故事

| 角色 | 场景与诉求 |
|---|---|
| **SRE / DevOps** | 通过 `helm install -f values-production-go.yaml` 一键拉起高可用集群，并配置 HPA 自动应对突发流量。 |
| **安全合规官** | 运行镜像符合 CIS 基线（无 root 权限、无已知 CVE 基础镜像），TLS 证书与 API Key 通过 Secret 注入，支持 5s CN 白名单热重载。 |
| **全栈开发者** | 本地使用 `docker compose -f docker-compose.go-engine.yml up` 一秒拉起 Agent、BFF 与 Web 控制台进行端到端联调。 |
| **算法工程师** | 针对大模型 NER 加速场景，可直接选用 `Dockerfile.cuda` 镜像运行于 NVIDIA GPU 算力节点。 |

---

## 4. 功能需求

### 4.1 Helm Chart 需求

| ID | 需求描述 | 验收标准 |
|---|---|---|
| **DEP-HELM-1** | 支持通过 `engineType: go` 部署 Go 原生引擎多副本 | Deployment 正常调度，就绪探针成功 |
| **DEP-HELM-2** | 支持 `values.yaml`、`values-production-go.yaml`、`values-ml.yaml` 多场景预置配置 | 配置继承清晰，无模板渲染错误 |
| **DEP-HELM-3** | ConfigMap 挂载动态领域规则与 Profile 配置 | Agent 启动正确加载 `config/` 与 `rules/` |
| **DEP-HELM-4** | Secret 注入 TLS 证书（`/certs`）与 API Key | 启动时加载证书并开启常量时间鉴权 |
| **DEP-HELM-5** | Service 暴露 REST（8079）与 gRPC（50051）双协议端口 | HTTP 与 gRPC 均可正常通信 |
| **DEP-HELM-6** | 健康探针规范 | Liveness 使用 `/health`，Readiness 使用 `/readyz` |
| **DEP-HELM-7** | HPA v2 自动弹性伸缩 | CPU > 70% 或 内存 > 80% 时自动扩容（2 → 10 副本） |
| **DEP-HELM-8** | Ingress 外部流量暴露与 SSL 卸载 | 支持 Nginx Ingress / ALB 路由转发 |
| **DEP-HELM-9** | NetworkPolicy 东西向流量隔离 | 仅允许集群内合法组件访问 Agent 端口 |
| **DEP-HELM-10** | ServiceMonitor / Prometheus 监控集成 | 暴露标准 `/metrics` 供指标自动采集 |

### 4.2 原生 Kubernetes 清单需求

| ID | 需求描述 | 验收标准 |
|---|---|---|
| **DEP-K8S-1** | 提供开箱即用的 Kustomize 清单 | `kubectl apply -k deploy/k8s/` 一键执行成功 |
| **DEP-K8S-2** | 样例 Secret 配置模板 | `secret.example.yaml` 规范清晰，不含真实凭据 |
| **DEP-K8S-3** | Go 引擎专用清单 | `deployment-go.yaml` 与 `service-go.yaml` 完备 |

### 4.3 Docker Compose 全栈编排需求

| ID | 需求描述 | 验收标准 |
|---|---|---|
| **DEP-COMPOSE-1** | 提供轻量级 Go 引擎开发全栈编排 | `docker-compose.go-engine.yml` / `docker-compose.dev-go-engine.yml` |
| **DEP-COMPOSE-2** | 支持 mTLS 双向认证编排验证 | `docker-compose.mtls-go-engine.yml` |

---

## 5. 非功能需求

| 维度 | 要求 |
|---|---|
| **镜像体积** | Go Alpine 基础镜像体积 **≤ 30MB**。 |
| **启动时间** | 容器启动至 `/health` 就绪 **≤ 100ms**。 |
| **内存开销** | Go Agent 单副本基础常驻内存 **≤ 30MB**。 |
| **安全合规** | 容器内以 UID 1000 非 root 权限运行，Capabilities 全部 Drop。 |

---

## 6. 验收标准

- [x] `Dockerfile` 与 `engine-go/Dockerfile.cuda` 多阶段构建 100% 成功。
- [x] Helm Chart `deploy/helm/PrivShield/` 结构完备且支持 Go 引擎部署。
- [x] `deploy/k8s/` 原生 Kustomize 清单完备。
- [x] Docker Compose 本地全栈编排文件完备且测试通过。
- [x] 全套测试通过：`make test` 100% PASS。