# PrivShield Docker Compose 编排指南

本文档介绍 `deploy/docker-compose/` 下的 Docker Compose 编排文件体系与使用方式。

---

## 1. 编排文件矩阵 (Compose Files Matrix)

PrivShield 针对不同生命周期环境提供了专门优化的 Compose 文件：

| 编排文件 | 适用环境 | 核心特性 | 启动命令 |
|---|---|---|---|
| [`docker-compose.yml`](docker-compose.yml) | **通用/默认全栈** | 基础全栈编排，包含 Core Agent、Go BFF、Web UI、3 个中台微服务，支持 `--profile llm` / `monitoring` / `phase-b` | `docker compose up -d` |
| [`docker-compose.prod.yml`](docker-compose.prod.yml) | **生产环境 (Prod)** | 全链路 TLS、API Key 强鉴权、Redis 分布式限流、JSON 结构化日志、无源码挂载纯镜像运行、`restart: always`、安全加固 (`no-new-privileges`) | `docker compose -f docker-compose.prod.yml up -d` |
| [`docker-compose.dev.yml`](docker-compose.dev.yml) | **开发联调 (Dev)** | 挂载宿主机源码目录（代码热修改生效）、控制台纯文本日志、关闭 TLS/Auth 免密调试、开放全部调试端口、含 3 个中台微服务 | `docker compose -f docker-compose.dev.yml up -d` |
| [`docker-compose.test.yml`](docker-compose.test.yml) | **自动化测试/CI (Test)** | 包含自动化 `test-runner` 容器，等待所有被测服务就绪后执行端到端 API 冒烟测试并自动退出返回退出码 | `docker compose -f docker-compose.test.yml up --abort-on-container-exit --exit-code-from test-runner` |

---

## 2. 生产环境部署说明 (Production)

### 2.1 准备环境与证书

```bash
cd deploy/docker-compose

# 1. 复制生产环境变量模板
cp .env.prod.example .env
# 编辑 .env，设置强密码与 API Key

# 2. 准备 TLS 证书（若启用 TLS）
mkdir -p certs
cp /path/to/tls.crt certs/tls.crt
cp /path/to/tls.key certs/tls.key
```

### 2.2 启动生产服务

```bash
# 启动基础核心套件（Core + Redis + Go Proxy + Web UI）
docker compose -f docker-compose.prod.yml up -d

# 启动全量生产套件（含 vLLM GPU 推理与监控）
docker compose -f docker-compose.prod.yml --profile llm --profile monitoring up -d
```

### 2.3 查看状态与日志

```bash
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f PrivShield
```

---

## 3. 开发联调说明 (Development)

```bash
cd deploy/docker-compose

# 启动开发环境（源码只读挂载，实时热调试）
# 包含 Core Agent + Go BFF + Web UI + 3 个中台微服务 (service-hub/datasource-mgr/audit-log)
docker compose -f docker-compose.dev.yml up -d

# 可选：同时启动 Phase B PostgreSQL（多副本 Hub 模式）
docker compose -f docker-compose.dev.yml --profile phase-b up -d

# 访问服务
# Web UI:           http://localhost:5173
# Agent REST:       http://localhost:8079/docs
# Agent gRPC:       localhost:50051
# Go 代理:          http://localhost:8081
# Service Hub:      http://localhost:8082 / gRPC localhost:50052
# Datasource Mgr:   http://localhost:8083 / gRPC localhost:50053
# Audit Log:        http://localhost:8084 / gRPC localhost:50054
# PostgreSQL:       localhost:5432 (需启用 phase-b profile)
```

---

## 4. 自动化 CI 集成测试说明 (CI / Testing)

```bash
cd deploy/docker-compose

# 启动集成测试，测试完成后自动返回 0/1 退出码
docker compose -f docker-compose.test.yml up --abort-on-container-exit --exit-code-from test-runner

# 测试结束后清理测试容器与临时卷
docker compose -f docker-compose.test.yml down -v
```
