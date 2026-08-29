# 部署与云原生交付文档索引 (Deployment & Cloud-Native Delivery Index)

本目录包含 `PrivShield` 纯 **Go 1.25+ 云原生生产部署** 与交付模块的全套 SDLC 文档，覆盖 Helm Chart（Go Engine 模板）、原生 Kubernetes manifests（Kustomize）与 Docker Compose 全栈编排。

---

## 📚 文档清单

| 文档 | 说明 | 目标读者 |
|---|---|---|
| [from_code_to_k8s.md](./from_code_to_k8s.md) | **从代码到 K8s 生产实践指南**：Go 多模块编译、极简 Alpine 镜像构建（~25MB）、K8s Pod 编排与 Helm 快速上手 | 开发、初学者、SRE |
| [prd.md](./prd.md) | **产品需求文档（PRD）**：云原生部署目标、交付规范、高可用要求与安全基线 | 架构师、项目经理 |
| [design.md](./design.md) | **部署技术架构设计**：双层协同调度拓扑、Helm Chart 模板架构、资源选型与 Secret 安全管理 | 架构师、后端开发 |
| [examples.md](./examples.md) | **多环境部署实战示例**：Helm、K8s 原生 manifests 与 Docker Compose 完整可执行指南 | SRE、DevOps、开发 |
| [examples/values-custom.yaml](./examples/values-custom.yaml) | **自定义 Helm values 示例**：生产级监控、安全与资源调优模板 | SRE、运维开发 |
| [testing.md](./testing.md) | **部署验证与测试规范**：Helm lint/template、K8s dry-run、冒烟测试与 CI 验证命令 | QA、测试开发、SRE |
| [ops.md](./ops.md) | **生产运维与故障排查 SOP**：容量规划、优雅升级、Secret 轮转与高频故障定位手册 | SRE、运维工程师 |

---

## 🚀 快速开始

### 1. 本地 Docker 镜像构建（Go 极简镜像）

```bash
cd /path/to/PrivShield

# 构建极简 Go 运行时镜像（~25MB，含 Agent 与 Gateway）
docker build -t privshield:10.0.0 .

# 或构建 NVIDIA GPU CUDA 推理加速镜像
docker build -f engine-go/Dockerfile.cuda -t privshield:10.0.0-cuda .
```

### 2. Helm 生产模式部署（Go 原生引擎）

```bash
# 1. 创建 TLS 证书与 API Key Secret
kubectl create secret tls privshield-tls --cert=tls.crt --key=tls.key
kubectl create secret generic privshield-apikeys --from-file=api-keys.json

# 2. 使用生产 Go values 一键安装
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production-go.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys
```

### 3. 原生 Kubernetes 一键部署 (Kustomize)

```bash
# 部署 Go 原生引擎与微服务群
kubectl apply -k ./deploy/k8s/
```

### 4. Docker Compose 本地全栈开发

```bash
# 一键启动 Go Agent、BFF、微服务与控制台前端
cd deploy/docker-compose && docker compose -f docker-compose.go-engine.yml up -d
```