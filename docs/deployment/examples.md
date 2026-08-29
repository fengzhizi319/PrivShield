# 部署实战使用示例 (Deployment Practical Examples)

---

## 1. 概述

本文档提供 `PrivShield` 的完整多环境部署实战指南，涵盖 Helm 安装（Go 原生引擎与 CUDA 变体）、Kubernetes 原生 Kustomize 部署与 Docker Compose 本地全栈启动。

---

## 2. 镜像构建与规格选择

| 镜像类型 | Dockerfile | 镜像 Tag 示例 | 适用场景 | 建议资源配置 |
|---|---|---|---|---|
| **Go 极简镜像 (推荐默认)** | [`Dockerfile`](../../Dockerfile) | `privshield:10.0.0` | 脱敏、差分隐私、K-匿名、L1 规则分类、L3 外部熔断 LLM、网关 | **Requests: 0.1 CPU / 128MiB**<br>**Limits: 1.0 CPU / 512MiB** |
| **CUDA GPU 加速镜像** | [`engine-go/Dockerfile.cuda`](../../engine-go/Dockerfile.cuda) | `privshield:10.0.0-cuda` | ONNX Runtime GPU 推理加速 Small-NER | **Requests: 1.0 CPU / 2GiB**<br>**Limits: 4.0 CPU / 8GiB (含 1 GPU)** |

---

## 3. Helm Chart 部署示例

### 3.1 开发与测试模式（快速启动）

```bash
cd /path/to/PrivShield

# 1. 本地构建 Go 镜像
docker build -t privshield:10.0.0 .

# 2. 安装 Helm Release
helm install privshield ./deploy/helm/PrivShield \
  --set engineType=go \
  --set goEngine.image.repository=privshield \
  --set goEngine.image.tag=10.0.0

# 3. 验证 Pod 就绪状态
kubectl get pods -l app.kubernetes.io/name=PrivShield
```

---

### 3.2 生产环境高可用部署（启用 TLS + API Key 认证 + HPA）

```bash
# 1. 创建生产 TLS Secret (PEM 格式)
kubectl create secret tls privshield-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key

# 2. 创建 API Key 认证 Secret (key 必须为 api-keys.json)
cat > api-keys.json <<'EOF'
{
  "prod-gateway-key": {
    "name": "production-gateway",
    "scopes": ["*"]
  },
  "prod-service-hub-key": {
    "name": "service-hub",
    "scopes": ["mask", "dp", "classify"]
  }
}
EOF

kubectl create secret generic privshield-apikeys \
  --from-file=api-keys.json

# 3. 使用生产级 values-production-go.yaml 部署
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production-go.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys \
  --set goEngine.image.repository=myregistry.example.com/privshield \
  --set goEngine.image.tag=10.0.0
```

---

### 3.3 使用自定义 values 文件

```bash
helm install privshield ./deploy/helm/PrivShield \
  -f docs/deployment/examples/values-custom.yaml
```

---

## 4. 原生 Kubernetes Kustomize 部署示例

### 4.1 基础一键部署

```bash
cd /path/to/PrivShield

# 直接应用 Kustomize 编排清单
kubectl apply -k ./deploy/k8s/

# 查看命名空间下全部资源
kubectl get all -n PrivShield
```

### 4.2 启用 TLS 证书与 Secret 挂载

1. 复制样例 Secret 并填入生产证书与密钥：
   ```bash
   cp deploy/k8s/secret.example.yaml deploy/k8s/secret.yaml
   # 编辑 deploy/k8s/secret.yaml 配置真实 tls.crt / tls.key / api-keys.json
   ```

2. 在 `deploy/k8s/kustomization.yaml` 中包含 `secret.yaml`：
   ```yaml
   resources:
     - namespace.yaml
     - configmap.yaml
     - deployment-go.yaml
     - service-go.yaml
     - secret.yaml
   ```

3. 执行部署应用：
   ```bash
   kubectl apply -k ./deploy/k8s/
   ```

---

## 5. Docker Compose 全栈实战示例

### 5.1 启动 Go Agent + 控制台 + 微服务全家桶

```bash
cd deploy/docker-compose

# 启动 Go 引擎全栈（后台运行）
docker compose -f docker-compose.go-engine.yml up -d

# 检查各微服务容器运行状态
docker compose -f docker-compose.go-engine.yml ps

# 测试 Agent 健康探针
curl http://localhost:8079/health
```

### 5.2 启动 mTLS 双向认证全栈

```bash
# 启动具备证书双向校验的 Go 引擎全栈
docker compose -f docker-compose.mtls-go-engine.yml up -d
```

---

## 6. 验证与冒烟测试

```bash
# 1. 验证 REST 健康检查端点
curl -s http://<host>:8079/health | jq

# 2. 验证敏感数据脱敏接口
curl -s -X POST http://<host>:8079/v1/privacy/mask \
  -H "Content-Type: application/json" \
  -d '{
    "field": "id_card",
    "value": "110101199003072345",
    "type": "id_card"
  }' | jq

# 3. 验证差分隐私加噪计算
curl -s -X POST http://<host>:8079/v1/privacy/dp/count \
  -H "Content-Type: application/json" \
  -d '{
    "values": [1, 0, 1, 1, 0],
    "epsilon": 1.0
  }' | jq
```