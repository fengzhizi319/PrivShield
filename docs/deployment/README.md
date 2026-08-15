# 部署文档索引

本目录包含 `PrivShield` 部署模块的全套 SDLC 文档，覆盖 Helm Chart、原生 Kubernetes manifests 与 Docker Compose 三种交付形式。


## 目录 (Table of Contents)

- [文档清单](#文档清单)
- [快速开始](#快速开始)
- [运行示例](#运行示例)

---

## 文档清单

| 文档 | 说明 | 目标读者 |
|---|---|---|
| [from_code_to_k8s.md](./from_code_to_k8s.md) | 小白向：从本地代码到 Kubernetes 的完整链路 | 初学者、全栈开发 |
| [prd.md](./prd.md) | 产品需求文档 | 产品经理、项目经理 |
| [design.md](./design.md) | 部署架构、交付形式与配置管理策略 | 架构师、后端开发 |
| [examples.md](./examples.md) | Helm、K8s 原生、Docker Compose 完整部署示例 | SRE、运维、开发 |
| [examples/values-custom.yaml](./examples/values-custom.yaml) | 自定义 Helm values 示例 | SRE、运维 |
| [testing.md](./testing.md) | 部署验证测试策略与可执行命令 | QA、测试开发、SRE |
| [ops.md](./ops.md) | 运维手册、参数建议与故障排查 | SRE、运维 |

## 快速开始

- **纯新手 / 第一次把本地代码部署到 K8s**：先读 [from_code_to_k8s.md](./from_code_to_k8s.md)。
- **了解部署产品需求与验收标准**：阅读 [prd.md](./prd.md)。
- **掌握架构选型与 Chart 结构**：阅读 [design.md](./design.md)。
- **查看完整部署示例**：参考 [examples.md](./examples.md) 或 [examples/values-custom.yaml](./examples/values-custom.yaml)。
- **部署后验证**：按 [testing.md](./testing.md) 执行测试。
- **日常运维与排障**：参考 [ops.md](./ops.md)。

## 运行示例

```bash
cd /path/to/PrivShield

# Helm 开发模式一键部署
helm install privshield ./deploy/helm/PrivShield

# 生产模式（需提前创建 TLS 与 API Key Secret）
helm install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  --set security.tls.existingSecret=privshield-tls \
  --set security.auth.apiKeysSecret=privshield-apikeys

# 原生 K8s 部署
kubectl apply -k ./deploy/k8s/

# Docker Compose 本地启动
cd deploy/docker-compose && docker compose up -d
```