# Console 自动化运行脚本说明指南

> 本目录（`console/scripts/`）包含了管理 **Privacy Local Agent 控制台** 及其 **代理后端与前台 Web UI** 的全套自动化 Bash 脚本。
> 支持三种运行姿态：**开发热重载模式 (`dev-*.sh`)**、**生产静态托管模式 (`prod-*.sh`)** 以及 **Docker 容器编排模式 (`docker-*.sh`)**。

---

## 1. 脚本分类概览

| 模式 | 脚本前缀 | 特性说明 | 适用场景 |
|---|---|---|---|
| **开发模式** | `dev-*.sh` | Python/Go 进程热重载 + Vite HMR 前端开发服务器 | 本地前端/后端代码开发调试（支持 `<50ms` 实时热更新） |
| **生产代理模式** | `prod-*.sh` | 编译构建前端静态产物 (`dist`) + 后端静态托管/代理 | 无 Docker 环境下的单机静态部署与集成测试 |
| **Docker 容器模式** | `docker-*.sh` | 多阶段 Docker 容器构建 + Docker Compose 全栈编排 | 容器化集群部署、GPU 大模型联合联调、开箱即用体验 |

---

## 2. 脚本使用说明

### 2.1 开发模式脚本 (`dev-*.sh`)

开发模式会启动 Node/Vite 开发服务器（端口 `5173`），修改前端代码可获得 `<50ms` 的 HMR 模块热重载。

#### 1. `./console/scripts/dev-start.sh`
- **作用**：一键启动 **Python REST 代理模式** 开发环境。
- **启动组件**：
  - Privacy Local Agent (REST: `http://127.0.0.1:8079`)
  - Console Python REST 代理后端 (API: `http://127.0.0.1:8080`)
  - Vite 前端开发服务器 (UI: `http://localhost:5173`)
- **用法**：
  ```bash
  bash ./console/scripts/dev-start.sh
  # 或赋予执行权限后直接运行:
  ./console/scripts/dev-start.sh
  # 非交互模式（CI/无 TTY：端口被占用时自动终止占用进程）:
  ./console/scripts/dev-start.sh --force
  ```
  > `dev-start*.sh` 系列均支持 `--force`；不传时端口冲突会交互询问，无 TTY 环境下则报错退出。

#### 2. `./console/scripts/dev-start-go.sh`
- **作用**：一键启动 **Go gRPC 代理模式** 开发环境。
- **启动组件**：
  - Privacy Local Agent (REST: `8079` + gRPC: `50051`)
  - Console Go gRPC 代理后端 (API: `http://127.0.0.1:8081`)
  - Vite 前端开发服务器 (UI: `http://localhost:5173`)
- **用法**：
  ```bash
  bash ./console/scripts/dev-start-go.sh
  ```

#### 3. `./console/scripts/dev-start-all.sh`
- **作用**：一键启动 **双后端模式** 开发环境。
- **启动组件**：
  - Privacy Local Agent (REST: `8079` + gRPC: `50051`)
  - Console Python REST 代理后端 (API: `http://127.0.0.1:8080`)
  - Console Go gRPC 代理后端 (API: `http://127.0.0.1:8081`)
  - Vite 前端开发服务器 (UI: `http://localhost:5173`)
- **特色**：前端顶部将出现 **Backend Selector** 下拉框，可在 Python 与 Go 双后端间自由切换。
- **用法**：
  ```bash
  bash ./console/scripts/dev-start-all.sh
  ```

#### 4. `./console/scripts/dev-start-go-mtls.sh`
- **作用**：一键启动 **Go gRPC mTLS 双向加密认证模式** 开发环境。
- **特色**：若缺失证书，自动调用 `gen-certs.sh` 生成自签名测试证书，Go 代理至 Agent 之间的 gRPC 链路全程加密与双向身份校验。
- **用法**：
  ```bash
  bash ./console/scripts/dev-start-go-mtls.sh
  ```

#### 5. `./console/scripts/dev-stop.sh`
- **作用**：安全停止并清理所有开发模式下的后台 PID 进程。
- **用法**：
  ```bash
  bash ./console/scripts/dev-stop.sh
  ```

---

### 2.2 生产静态托管模式脚本 (`prod-*.sh`)

生产模式会首先自动编译构建 Web 前端产物（生成到 `console/web/dist`），并由代理后端直接托管静态文件，无需运行 Node.js 进程。

#### 1. `./console/scripts/prod-start.sh`
- **作用**：启动 **Python REST 静态托管** 生产环境。
- **服务端口**：控制台 UI & API 统一位于 `http://127.0.0.1:8080`。
- **用法**：
  ```bash
  bash ./console/scripts/prod-start.sh
  ```

#### 2. `./console/scripts/prod-start-go.sh`
- **作用**：启动 **Go gRPC 静态托管** 生产环境。
- **服务端口**：控制台 UI & API 统一位于 `http://127.0.0.1:8081`。
- **用法**：
  ```bash
  bash ./console/scripts/prod-start-go.sh
  ```

#### 3. `./console/scripts/prod-start-all.sh`
- **作用**：同时启动 **Python + Go 双后端静态托管** 生产环境。
- **服务端口**：Python 控制台 `http://127.0.0.1:8080` | Go 控制台 `http://127.0.0.1:8081`。
- **用法**：
  ```bash
  bash ./console/scripts/prod-start-all.sh
  ```

#### 4. `./console/scripts/prod-start-go-mtls.sh`
- **作用**：以 **mTLS 双向加密认证模式** 启动 Go 代理静态托管生产环境。
- **用法**：
  ```bash
  bash ./console/scripts/prod-start-go-mtls.sh
  ```

#### 5. `./console/scripts/prod-stop.sh`
- **作用**：安全停止并清理所有生产静态托管模式下的后台 PID 进程。
- **用法**：
  ```bash
  bash ./console/scripts/prod-stop.sh
  ```

---

### 2.3 Docker 容器编排模式脚本 (`docker-*.sh`)

基于 Docker 镜像构建与 Docker Compose 容器编排，支持在完全隔离的容器环境中运行全套服务。

#### 1. `./scripts/dev/docker-start-agent.sh`
- **说明**：此脚本属于 Agent Core 服务组件，已从 `console/scripts/` 迁移至 `./scripts/dev/docker-start-agent.sh`。
- **作用**：在独立 Docker 容器中构建并启动 Privacy Local Agent 核心 Sidecar 服务。
- **参数**：
  - `core`（默认）：构建轻量级原语镜像 (`privacy-local-agent:0.1.0`)。
  - `ml`：构建全量包含 PyTorch / Transformers / ONNX Runtime 的 ML 镜像 (`privacy-local-agent:0.1.0-ml`)。
- **服务端口**：REST `http://127.0.0.1:8079` | gRPC `127.0.0.1:50051`。
- **用法**：
  ```bash
  # 启动轻量 core 镜像 Agent 容器
  bash ./scripts/dev/docker-start-agent.sh core

  # 启动全量 ml 镜像 Agent 容器
  bash ./scripts/dev/docker-start-agent.sh ml
  ```

#### 2. `./scripts/dev/docker-start-llm.sh`
- **说明**：此脚本属于 Agent LLM 服务组件，已从 `console/scripts/` 迁移至 `./scripts/dev/docker-start-llm.sh`。
- **作用**：在 Docker 容器中启动 vLLM 大模型 GPU 推理服务（采用 Compose `llm` Profile）。
- **服务端口**：OpenAI 兼容 REST API `http://127.0.0.1:8000/v1`。
- **用法**：
  ```bash
  bash ./scripts/dev/docker-start-llm.sh
  ```

#### 3. `./console/scripts/docker-start-go.sh`
- **作用**：使用 Docker Compose 一键构建并启动 **Agent + Go 代理 + React Web UI 容器套件**。
- **服务端口**：Web UI `http://localhost:5173` | Go API `http://localhost:8081`。
- **用法**：
  ```bash
  bash ./console/scripts/docker-start-go.sh
  ```

#### 4. `./console/scripts/docker-start-python.sh`
- **作用**：使用 Docker Compose 一键构建并启动 **Agent + Python 代理 + React Web UI 容器套件**。
- **服务端口**：Web UI `http://localhost:5173` | Python API `http://localhost:8080`。
- **用法**：
  ```bash
  bash ./console/scripts/docker-start-python.sh
  ```

#### 5. `./console/scripts/docker-start-all.sh`
- **作用**：使用 Docker Compose 一键启动 **全栈容器套件**（Agent + 双代理后端 + Web UI + 可选 vLLM）。
- **参数**：传递 `--with-llm` 可同时拉起 vLLM GPU 推理容器。
- **用法**：
  ```bash
  # 启动全栈基础套件
  bash ./console/scripts/docker-start-all.sh

  # 启动全栈基础套件 + vLLM 大模型推理服务
  bash ./console/scripts/docker-start-all.sh --with-llm
  ```

#### 6. `./console/scripts/docker-stop.sh`
- **作用**：一键优雅停止、移除并清理所有由上述 Docker 脚本启动的容器与 Compose 服务。
- **用法**：
  ```bash
  bash ./console/scripts/docker-stop.sh
  ```

---

## 3. 常见问题排查 (Troubleshooting)

1. **端口冲突问题**：
   - 脚本在启动前会自动检测目标端口（如 `8079`、`8080`、`8081`、`5173`）是否被占用。
   - 若检测到端口被占用，脚本会提示并可自动终止冲突进程。

2. **虚拟环境依赖缺失**：
   - `dev-*.sh` 与 `prod-*.sh` 会自动检测 `.venv` 虚拟环境。若不存在，脚本会自动创建并安装 `requirements.txt` 与 `pip install -e .`。

3. **前端热更新失效**：
   - 若使用 `dev-*.sh` 启动后热更新不生效，请检查 `console/web/vite.config.ts` 中的代理配置及网络防火墙设置。

4. **Docker GPU 支持**：
   - 使用 `docker-start-llm.sh` 或 `--with-llm` 启动大模型时，请确保宿主机已安装 NVIDIA 驱动及 `nvidia-container-toolkit`。
