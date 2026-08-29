# 从本地代码到 Kubernetes：PrivShield 云原生部署实战指南

> 目标读者：希望将本地开发调试的 `PrivShield` 纯 Go 1.25+ 代码打包并部署到 Kubernetes 生产集群的开发者与 DevOps 工程师。
> 读完本文，你将掌握：
> 1. 理解“本地 Go 源码 → Alpine 多阶段镜像（~25MB） → K8s Pod 编排”的全流程；
> 2. 使用 Helm Chart 或 Kustomize 原生清单将服务一键部署至 Kubernetes；
> 3. 掌握生产就绪的配置注入、安全加固（TLS/mTLS/Secret）与故障排查技巧。

---

## 1. 核心流程概览

`PrivShield` 核心引擎采用 **Go 1.25+ 云原生架构** 实现。本地开发时可通过 `go run` 或 `make build` 编译单二进制运行；部署到 Kubernetes 时，通过 Dockerfile 多阶段构建产出极小运行镜像，并由 Kubernetes Deployment、Service、ConfigMap 与 Secret 进行资源调度与配置注入。

```text
┌─────────────────────────┐
│ 本地 Go 代码 (go.work)   │
│ pkg/ privacy-go-sdk/    │
│ engine-go/              │
└────────────┬────────────┘
             │ docker build (Go 1.25 Multi-stage)
             ▼
┌─────────────────────────┐
│ 极简 Alpine 镜像 (~25MB) │
│ - /app/privshield-agent │
│ - /app/privshield-gateway│
└────────────┬────────────┘
             │ push / kind load
             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Kubernetes 生产集群                                              │
│                                                                  │
│  Ingress (南北入口) ──► Service (:8079 REST / :50051 gRPC)       │
│                                │                                 │
│                                ▼                                 │
│                     PrivShield Pod (2+ 副本)                     │
│                     - 非 root 用户运行 (UID 1000)                │
│                     - 挂载 ConfigMap (规则库)                    │
│                     - 挂载 Secret (TLS 证书与 API Keys)          │
│                     - 探针检查 (/health 与 /readyz)              │
│                     - HPA 弹性伸缩 (CPU/内存双阈值)              │
└──────────────────────────────────────────────────────────────────┘
```

---

## 2. 前置准备工具

| 工具 | 作用 | 推荐版本 |
|---|---|---|
| **Go** | 本地代码编译与单元测试 | 1.25+ |
| **Docker** | 镜像多阶段构建与本地容器运行 | 20.10+ |
| **kubectl** | 与 Kubernetes 集群交互 | 1.25+ |
| **Helm** | Kubernetes 生产应用包管理器 | 3.12+ |
| **Kubernetes 集群** | 运行生产/测试 Pod（支持 Kind / Minikube / K8s 物理集群） | 1.25+ |

---

## 3. 本地代码如何构建为容器镜像

### 3.1 Dockerfile 多阶段构建原理解析

项目根目录的 [`Dockerfile`](../../Dockerfile) 采用高效的双阶段构建机制：

- **Stage 1: `golang:1.25-alpine3.21` 编译阶段**
  - 设置 `CGO_ENABLED=0` 生成纯静态二进制；
  - 自动编译 `privshield-agent`（REST :8079 + gRPC :50051）与 `privshield-gateway`（REST :8000 + gRPC :50000）；
  - 去除符号表与调试信息（`-ldflags="-s -w"`），最小化二进制大小。
- **Stage 2: `alpine:3.21` 极简运行阶段**
  - 仅安装 CA 证书与时区包（`ca-certificates tzdata`）；
  - 创建非 root 系统用户 `privacy:privacy`；
  - 从 Stage 1 拷贝编译产物以及 `config/`、`rules/` 规则配置；
  - 配置内置 `HEALTHCHECK` 探针（`wget -qO- http://127.0.0.1:8079/health`）。

### 3.2 镜像构建命令

```bash
cd /path/to/PrivShield

# 1. 构建标准 Go 原生镜像 (推荐默认，~25MB)
docker build -t privshield:10.0.0 .

# 2. 或构建 NVIDIA GPU CUDA 加速镜像 (支持 ONNX GPU 推理)
docker build -f engine-go/Dockerfile.cuda -t privshield:10.0.0-cuda .
```

### 3.3 本地容器运行验证

```bash
# 启动容器并映射端口
docker run -d --name privshield-test \
  -p 8079:8079 -p 50051:50051 \
  privshield:10.0.0

# 验证健康检查端点
curl -s http://127.0.0.1:8079/health
# 返回: {"status":"ok","version":"10.0.0"}

# 验证敏感数据脱敏接口
curl -s -X POST http://127.0.0.1:8079/v1/privacy/mask \
  -H "Content-Type: application/json" \
  -d '{"field":"phone","value":"13812345678","type":"phone"}'

# 清理测试容器
docker rm -f privshield-test
```

---

## 4. 部署至 Kubernetes 集群

### 4.1 方案 A：使用 Helm Chart 一键生产部署（推荐）

#### 1. 准备 TLS 证书与 API Key Secret
```bash
# 创建 TLS Secret
kubectl create secret tls privshield-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key

# 创建 API Key Secret
cat > api-keys.json <<'EOF'
{
  "prod-client-key-1": {
    "name": "hospital-client",
    "scopes": ["mask", "dp", "classify"]
  }
}
EOF

kubectl create secret generic privshield-apikeys \
  --from-file=api-keys.json
```

#### 2. 执行 Helm 安装
```bash
# 使用生产级 values-production-go.yaml 部署
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production-go.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys \
  --set goEngine.image.repository=privshield \
  --set goEngine.image.tag=10.0.0
```

#### 3. 检查部署状态
```bash
# 查看 Pod 与 Service 状态
kubectl get pods,svc,hpa -l app.kubernetes.io/name=PrivShield
```

---

### 4.2 方案 B：使用原生 Kubernetes Kustomize 部署

```bash
cd /path/to/PrivShield

# 应用清单
kubectl apply -k deploy/k8s/

# 查看资源状态
kubectl get all -n PrivShield
```

---

## 5. 生产高频故障排查与速查表

| 故障现象 | 根因定位 | 排查与解决命令 |
|---|---|---|
| **`ImagePullBackOff`** | 镜像名、Tag 错误或私有镜像仓库认证失败 | `kubectl describe pod <pod-name>` 检查镜像路径，确认 `imagePullSecrets` 配置正确。 |
| **`CrashLoopBackOff`** | 配置文件路径不存在、证书挂载错误或端口冲突 | `kubectl logs <pod-name>` 查看标准输出日志；检查 ConfigMap 挂载路径是否为 `/app/config`。 |
| **探针 `Readiness probe failed`** | 启动阶段未能及时就绪（如模型加载超时或配置解析异常） | 检查 `/readyz` 响应；在 Helm values 中适当调大 `probes.readiness.initialDelaySeconds`。 |
| **REST 调用报 `401 Unauthorized`** | 开启了鉴权但请求未携带 `X-API-Key`，或 Secret 中 key 文件名非 `api-keys.json` | 确认 Secret 内文件名为 `api-keys.json`，并在请求头添加 `X-API-Key: <key>`。 |
| **gRPC 客户端报 `UNAVAILABLE`** | gRPC 长连接未通过正确端口连接，或 TLS/mTLS 握手不匹配 | 检查 Service 端口是否暴露 `50051`，客户端是否配置对应的根 CA 证书。 |
