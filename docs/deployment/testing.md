# 部署验证与测试规范 (Deployment Testing Guide & Report)

---

## 1. 概述

本文档定义 `PrivShield` 云原生部署包的测试策略、验证命令与验收标准，覆盖 Helm Chart（Go 原生模板）、原生 Kubernetes manifests（Kustomize）与 Docker Compose 全栈编排。

---

## 2. 测试策略与目标

- **Helm Chart 语法与模板验证**：验证 Chart 结构、Go Template 语法及各场景 values 文件的渲染正确性。
- **K8s Manifests Dry-Run 校验**：验证原生清单可通过 `kubectl apply --dry-run=client` 校验。
- **容器镜像健康验证**：验证极简 Go 运行时镜像（~25MB）与 CUDA 运行时镜像构建成功并正常响应探针。
- **端到端冒烟与安全验证**：验证 REST/gRPC 服务暴露、API Key 鉴权与 TLS 双向握手。

---

## 3. Helm Chart 验证命令

### 3.1 Chart 静态 Lint 检查

```bash
cd /path/to/PrivShield
helm lint deploy/helm/PrivShield
```

**预期结果**：提示 `1 chart(s) linted, 0 chart(s) failed`，无 ERROR。

### 3.2 默认模板渲染验证

```bash
helm template test-release deploy/helm/PrivShield
```

### 3.3 生产 Go 引擎模板渲染验证

```bash
helm template prod-go deploy/helm/PrivShield \
  -f deploy/helm/PrivShield/values-production-go.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys
```

**检查项**：
- Deployment 中正确配置 `engineType: go` 且容器镜像为 `privshield-go:1.0.0`；
- 探针包含 `/health`（liveness）与 `/readyz`（readiness）；
- HPA 与 NetworkPolicy 资源被正确渲染；
- TLS 卷挂载与 API Key Secret 引用正确。

---

## 4. 原生 Kubernetes Kustomize 验证

### 4.1 客户端 Dry-Run 校验

```bash
cd /path/to/PrivShield
kubectl apply -k deploy/k8s/ --dry-run=client
```

**预期结果**：所有 Namespace、ConfigMap、Deployment、Service 资源通过客户端校验，语法无误。

### 4.2 实际部署状态断言

```bash
# 查看所有 Pod 就绪状态
kubectl get pods -n PrivShield -l app.kubernetes.io/part-of=PrivShield

# 查看 Service 端口映射
kubectl get svc -n PrivShield

# 端口转发测试
kubectl port-forward -n PrivShield svc/PrivShield-Go 8079:8079
```

---

## 5. Docker Compose 全栈验证

```bash
cd deploy/docker-compose

# 1. 启动 Go 引擎全栈
docker compose -f docker-compose.go-engine.yml up -d

# 2. 检查所有容器均处于 Up (healthy) 状态
docker compose -f docker-compose.go-engine.yml ps

# 3. 停止全栈
docker compose -f docker-compose.go-engine.yml down
```

---

## 6. 功能接口冒烟测试

部署成功后，执行以下 API 探测脚本：

```bash
HOST="http://localhost:8079"

# 1. 健康探针测试
curl -s -f "$HOST/health" | grep -q "ok" && echo "✅ Health check PASSED"

# 2. 差分隐私计算接口测试
curl -s -f -X POST "$HOST/v1/privacy/dp/count" \
  -H "Content-Type: application/json" \
  -d '{"values": [1, 0, 1, 1, 0], "epsilon": 1.0}' | grep -q "result" && echo "✅ DP count PASSED"

# 3. 字段敏感脱敏接口测试
curl -s -f -X POST "$HOST/v1/privacy/mask" \
  -H "Content-Type: application/json" \
  -d '{"field": "phone", "value": "13800138000", "type": "phone"}' | grep -q "masked" && echo "✅ Masking PASSED"
```

---

## 7. 验收检查清单 (Checklist)

- [x] `helm lint deploy/helm/PrivShield` 检查 0 失败。
- [x] `values-production-go.yaml` 渲染无未定义变量或语法错误。
- [x] `kubectl apply -k deploy/k8s/ --dry-run=client` 校验通过。
- [x] `docker-compose.go-engine.yml` 成功启动并通过 `/health` 探针。
- [x] 极简 Go 运行时镜像（`Dockerfile`）编译通过。
- [x] 冒烟测试（Health / DP / Masking）返回 HTTP 200。
- [x] 启用 API Key 后，未授权请求返回 401，合法凭证请求返回 200。