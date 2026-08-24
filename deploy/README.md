# 数盾部署与运维编排全景指南 (PrivShield Deployment Guide)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 全套企业级生产与开发部署编排方案，支持从单机 Docker 极速体验到大规模 Kubernetes 云原生集群部署。

---

## 1. 部署形态总览与选型矩阵

| 部署形态 | 目录位置 | 适用场景 | 核心特性 | 快速启动命令 |
|---|---|---|---|---|
| **Docker Compose** | [`deploy/docker-compose/`](docker-compose/README.md) | 单机 / 边缘节点 / 开发测试 | 包含 Prod、Dev、Test 三套编排，一键拉起 Agent + BFF + 3大微服务 + Web UI | `bash ./scripts/prod/deploy-docker-compose.sh` |
| **Kubernetes (原生清单)** | [`deploy/k8s/`](k8s/kustomization.yaml) | 生产私有云 / 自动化运维集群 | Kustomize 声明式编排，运行时解耦部署（Core 与 vLLM 分离） | `bash ./scripts/prod/deploy-k8s.sh` |
| **Helm Chart** | [`deploy/helm/PrivShield/`](helm/PrivShield/Chart.yaml) | 企业级生产标准交付 / 云原生 PaaS | 模板化参数配置、HPA 弹性扩缩、NetworkPolicy 安全隔离、mTLS 支持 | `bash ./scripts/prod/deploy-helm.sh` |
| **Prometheus 监控** | [`deploy/prometheus/`](prometheus/prometheus.yml) | 全链路指标采集与告警 | 自动采集 Agent 算力引擎、BFF 代理网关与 3 大中台微服务指标 | `docker compose --profile monitoring up -d` |
| **Grafana 可视化** | [`deploy/grafana/`](grafana/dashboard.json) | 生产大屏与实时监控看板 | 预置 QPS、P95 延迟、三层漏斗命中率与敏感字段统计大屏 | `http://localhost:3000` (admin/密码) |

---

## 2. 全栈组件拓扑与端口映射

```text
                                  ┌──────────────────────────┐
                                  │   Console Web UI         │
                                  │   (React SPA :5173 / :80)│
                                  └─────────────┬────────────┘
                                                │
                                  ┌─────────────▼────────────┐
                                  │   Go gRPC BFF 代理网关    │
                                  │   (:8081 / HTTP/2 多路复用)│
                                  └─────────────┬────────────┘
                                                │
                                  ┌─────────────▼────────────┐
                                  │   PrivShield Core Agent   │
                                  │   (REST: 8079 / gRPC: 50051) │
                                  └─────────────┬────────────┘
                                                │
        ┌───────────────────────────────────────┼───────────────────────────────────────┐
        │                                       │                                       │
┌───────▼──────────────┐             ┌──────────▼───────────┐                ┌──────────▼───────────┐
│ Service Hub 调度中枢  │             │ Datasource Mgr 数据源 │                │ Audit Log 审计存证   │
│ (:8082 调度流水线)   │             │ (:8083 敏感特征探查)  │                │ (:8084 SHA-256 存证) │
└──────────────────────┘             └──────────────────────┘                └──────────────────────┘
```

---

## 3. 各部署方案操作指南

### 3.1 Docker Compose 部署

```bash
# 1. 基础生产模式（Core + Redis + Go BFF + 微服务群 + Web UI）
bash ./scripts/prod/deploy-docker-compose.sh

# 2. 启用 GPU vLLM 大模型推理容器
bash ./scripts/prod/deploy-docker-compose.sh --with-llm

# 3. 启用 Prometheus + Grafana 监控套件
bash ./scripts/prod/deploy-docker-compose.sh --with-monitoring

# 4. 生产健康全面巡检
bash ./scripts/prod/prod_health_check.sh

# 5. 停止集群
bash ./scripts/prod/stop-docker-compose.sh
```

### 3.2 Helm Chart 生产级部署

```bash
# 1. 语法检查与模板渲染验证
make helm-lint
make helm-template

# 2. 安装/升级 Chart（默认 values.yaml）
helm upgrade --install privshield ./deploy/helm/PrivShield -n privshield --create-namespace

# 3. 生产高可用部署（含 Secret 引用与资源限制）
helm upgrade --install privshield ./deploy/helm/PrivShield \
  -f ./deploy/helm/PrivShield/values-production.yaml \
  -n privshield

# 4. 卸载 Release
bash ./scripts/prod/uninstall-helm.sh
```

### 3.3 原生 Kubernetes (Kustomize) 部署

```bash
# 部署全套 K8s 资源
kubectl apply -k ./deploy/k8s/

# 检查 Pod 状态
kubectl get pods -n PrivShield

# 停止与清理
kubectl delete -k ./deploy/k8s/
```

---

## 4. 监控与可观测性集成

### 4.1 Prometheus 抓取端点
Prometheus 配置文件 [deploy/prometheus/prometheus.yml](prometheus/prometheus.yml) 预置了对以下服务的自动抓取：
* `PrivShield Agent` (`:8079/metrics`)
* `Go BFF Gateway` (`:8081/metrics`)
* `Service Hub` (`:8082/metrics`)
* `Datasource Manager` (`:8083/metrics`)
* `Audit Log` (`:8084/metrics`)

### 4.2 告警规则
预置告警规则位于 [deploy/prometheus/alerts.yml](prometheus/alerts.yml)：
* 网关无健康节点 / 降级告警（`GatewayNoHealthyNodes`, `GatewayDegradedCapacity`）
* 请求与分类高延迟告警（`HighRequestLatencyP95`, `HighClassificationLatency`）
* 5xx 错误率与认证拒绝率告警（`HighGatewayErrorRate`, `HighAuthDenialRate`）
* 隐私预算耗尽预警（`PrivacyBudgetExhausted`）

---

## 5. 生产安全加固核查表

在正式生产上线前，请确认已完成以下加固项：

- [ ] **传输安全**：启用 TLS/mTLS，配置有效证书（替换默认自签名测试证书）；
- [ ] **身份鉴权**：设置强密码 `PRIVACY_AUTH_EXTERNAL_KEYS_JSON` / `api-keys.json`；
- [ ] **容器安全**：确认所有容器以非 root 用户（UID 1000）运行，开启 `no-new-privileges`；
- [ ] **持久化存储**：为 SQLite 数据库（预算库与微服务库）配置具有备份策略的本地持久卷或网络卷；
- [ ] **资源配额**：在 Helm/Compose 中明确配置 `requests` 与 `limits`，防止 OOM 级联故障。
