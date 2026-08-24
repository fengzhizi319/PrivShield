# Console 自动化运行脚本说明指南 (已归并收敛)

> **⚠️ 路径迁移声明 / DEPRECATION NOTICE**:
> 按照系统 Monorepo 单轨化治理与架构收敛规范（`migration-design.md` §12.2），原位于本目录（`console/scripts/`）下的所有生命周期与运维脚本已全面归并迁移至项目根目录的 **`scripts/dev/`** 与 **`scripts/prod/`**。
>
> 本目录下的脚本文件当前仅保留作为**向后兼容转发器**（输出 `DEPRECATED` 提示并自动转发调用至新路径）。请在新脚本、CI/CD 及日常操作中直接使用根目录的 `scripts/dev/` 与 `scripts/prod/` 路径。

---

## 1. 新脚本体系映射表

| 原路径 (`console/scripts/`) | 新路径 (`scripts/dev/` 或 `scripts/prod/`) | 模式说明 |
|---|---|---|
| `dev-start.sh` | `scripts/dev/dev-start.sh` | 开发模式：Python REST BFF + Vite HMR |
| `dev-start-go.sh` | `scripts/dev/dev-start-go.sh` | 开发模式：Go gRPC BFF + Vite HMR |
| `dev-start-go.ps1` | `scripts/dev/dev-start-go.ps1` | 开发模式：Windows PowerShell Go + Vite |
| `dev-start-all.sh` | `scripts/dev/dev-start-all.sh` | 开发模式：双后端 + Vite HMR |
| `dev-start-go-mtls.sh` | `scripts/dev/dev-start-go-mtls.sh` | 开发模式：Go gRPC mTLS + Vite HMR |
| `dev-stop.sh` | `scripts/dev/dev-stop.sh` | 开发模式：一键停止与端口清理 |
| `dev-start-new-modules.sh` | `scripts/dev/dev-start-new-modules.sh` | 开发模式：启动三大微服务 (8082/8083/8084) |
| `dev-stop-new-modules.sh` | `scripts/dev/dev-stop-new-modules.sh` | 开发模式：停止三大微服务 |
| `e2e-start-all-services.sh` | `scripts/dev/e2e-start-all-services.sh` | E2E 模式：启动 Agent + 三大微服务 |
| `e2e-stop-all-services.sh` | `scripts/dev/e2e-stop-all-services.sh` | E2E 模式：停止全部 E2E 服务 |
| `integration-test-new-modules.sh` | `scripts/dev/integration-test-new-modules.sh` | 集成测试：三大微服务 curl 自动化验证 |
| `docker-start-go.sh` | `scripts/dev/docker-start-go.sh` | Docker 模式：Agent + Go BFF + Web |
| `docker-start-python.sh` | `scripts/dev/docker-start-python.sh` | Docker 模式：Agent + Python BFF + Web |
| `docker-start-all.sh` | `scripts/dev/docker-start-all.sh` | Docker 模式：全栈容器编排（含 vLLM 选项） |
| `docker-stop.sh` | `scripts/dev/docker-stop.sh` | Docker 模式：停止与清理容器 |
| `prod-start.sh` | `scripts/prod/prod-start.sh` | 生产模式：Python REST 静态托管 |
| `prod-start-go.sh` | `scripts/prod/prod-start-go.sh` | 生产模式：Go gRPC 静态托管 |
| `prod-start-all.sh` | `scripts/prod/prod-start-all.sh` | 生产模式：双后端静态托管 |
| `prod-start-go-mtls.sh` | `scripts/prod/prod-start-go-mtls.sh` | 生产模式：Go gRPC mTLS 静态托管 |
| `prod-stop.sh` | `scripts/prod/prod-stop.sh` | 生产模式：一键停止生产进程 |

---

## 2. 运行时产物路径规范

在脚本体系归并后，运行时产物已从 `console/` 提至仓库根目录统一管理：
- **PID 文件**：统一存放于 `$PROJECT_ROOT/.pids/`（已被根目录 `.gitignore` 忽略）；
- **日志文件**：统一存放于 `$PROJECT_ROOT/.logs/`（已被根目录 `.gitignore` 忽略）；
- **数据库持久化**：统一存放于 `$PROJECT_ROOT/data/`。
