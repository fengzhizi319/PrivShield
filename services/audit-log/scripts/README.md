# 脱敏审计日志服务脚本与独立运维指南 (`scripts/`)

> 本目录包含了 **脱敏审计日志与存证服务 (`audit-log`)** 的所有单服务容器化部署、Kubernetes 独立发布、证书生成与健康检查脚本。

---

## 1. 脚本清单与功能速览

| 脚本文件 | 类型 | 主要功能 | 适用场景 |
|---|---|---|---|
| [`deploy.sh`](deploy.sh) | Docker | 独立编译 Docker 镜像并启动单机容器（挂载 SQLite 审计持久化卷） | 单服务 Docker 容器化运行 / CI 测试 |
| [`stop-docker.sh`](stop-docker.sh) | Docker | 停止并清理 `audit-log` 独立容器 | 容器清理 / 重新部署 |
| [`deploy-k8s.sh`](deploy-k8s.sh) | Kubernetes | 使用 `deploy/k8s/` 目录下的自包含清单独立部署到 K8s | 单服务独立发布 / K8s 集群部署 |
| [`stop-k8s.sh`](stop-k8s.sh) | Kubernetes | 卸载与清理 `audit-log` 在 K8s 中的所有独立资源 | 集群资源清理 / 卸载下线 |
| [`gen-certs.sh`](gen-certs.sh) | 安全/证书 | 生成 TLS 1.3 服务端与客户端证书，支持 gRPC 零信任 mTLS 与公钥提取 | mTLS 证书准备与公钥固定 |
| [`health-check.sh`](health-check.sh) | 运维探针 | 探测 `/health` 端点与底层 SQLite 审计库状态 | 服务探活与健康监控 |

---

## 2. 脚本使用详解

### 2.1 独立 Docker 容器部署

```bash
# 构建并启动独立容器（挂载 audit-data 卷保证 8 要素存证日志持久化）
bash ./scripts/deploy.sh

# 停止并清理容器
bash ./scripts/stop-docker.sh
```

### 2.2 独立 Kubernetes 部署

```bash
# 1. 独立部署到指定命名空间：
bash ./scripts/deploy-k8s.sh -n privshield

# 2. 演练模式（Dry-Run）：
bash ./scripts/deploy-k8s.sh --dry-run

# 3. 卸载集群资源：
bash ./scripts/stop-k8s.sh -n privshield
```

### 2.3 证书生成与健康探针

```bash
# 生成/更新 mTLS 测试证书链
bash ./scripts/gen-certs.sh

# 执行健康检查
bash ./scripts/health-check.sh
```

---

## 3. 环境变量速查

| 环境变量 | 默认值 | 作用脚本 | 说明 |
|---|---|---|---|
| `AUDIT_LOG_IMAGE` | `privshield-audit-log:1.8.0` | `deploy.sh` | Docker 镜像名称 |
| `AUDIT_LOG_CONTAINER` | `privshield-audit-log` | `deploy.sh`, `stop-docker.sh` | Docker 容器名称 |
| `AUDIT_LOG_PORT` | `8084` | 全部部署与检查脚本 | HTTP REST 服务端口 |
| `AUDIT_LOG_GRPC_PORT` | `50054` | 全部部署脚本 | gRPC 服务端口 |
| `AUDIT_LOG_DATA_DIR` | `privshield-audit-log-data` | `deploy.sh` | SQLite 数据卷名/持久化路径 |
| `K8S_NAMESPACE` | `privshield` | `deploy-k8s.sh`, `stop-k8s.sh` | Kubernetes 目标命名空间 |
