# 模拟数据源服务 (Mock Datasource Manager)

`services/datasource-mgr` 是 PrivShield 平台的轻量级模拟数据源服务。**本项目专为开发、测试与调试阶段提供真实业务数据仿真与跨服务通信验证**；在生产环境中，调度中枢将直接对接真实外部数据源。

---

## 核心功能与特性

- **固定模拟数据库**：内置脱敏场景常用的医保就医结算数据（`yibao.csv`）与康养健康档案数据（`kangyang.csv`）；
- **4 个专用模拟数据接口**：
  - **API 1**：申请医保就医与结算模拟数据 (`GET /api/v1/yibao` / `rpc GetYibaoData`)
  - **API 2**：申请康养体检与慢病模拟数据 (`GET /api/v1/kangyang` / `rpc GetKangyangData`)
  - **API 3**：预留政务数据源 3 扩展模拟接口 (`GET /api/v1/mock3` / `rpc GetMockData3`)
  - **API 4**：预留企业数据源 4 扩展模拟接口 (`GET /api/v1/mock4` / `rpc GetMockData4`)
- **双协议通信支持**：对外提供 HTTP REST（端口 `:8083`），对内提供高性能 gRPC（端口 `:50053`）；
- **mTLS 双向认证与公钥固定**：gRPC 服务支持 TLS 1.3 证书校验与客户端公钥固定（Public Key Pinning）；
- **测试证书持久入库**：预置全套测试证书链与已固定的公钥文件（`certs/client.pub`），无需每次测试重新生成，保障公钥固定机制可复现。

> 📖 **深度学习指南**：完整架构解析、数据集字典说明与源码导读见 [docs/learning-guide.md](docs/learning-guide.md)。

---

## 运行脚本指南

### 1. 开发运行 (Development Run)

无需 mTLS，直接启动轻量开发服务：

```bash
cd services/datasource-mgr
bash scripts/dev-run.sh
# 或者使用 Makefile 快捷命令：
make dev
```

监听：
- **HTTP REST**：`http://127.0.0.1:8083`
- **gRPC (insecure)**：`127.0.0.1:50053`

### 2. 生产加固运行 (Production Run with mTLS)

启用完整的 TLS 1.3 双向证书校验与客户端公钥固定：

```bash
cd services/datasource-mgr
bash scripts/prod-run.sh
# 或者使用 Makefile 快捷命令：
make prod
```

监听：
- **HTTP REST**：`http://0.0.0.0:8083`
- **gRPC (mTLS)**：`0.0.0.0:50053`（校验 `certs/ca.crt`、`certs/server.crt` 与固定公钥 `certs/client.pub`）

### 3. 证书重新生成脚本 (Generate Certs)

如需更新测试证书链：

```bash
cd services/datasource-mgr
bash scripts/gen-certs.sh
# 或 make gen-certs
```

生成文件清单（存放于 `certs/` 并提交至 Git）：
- `ca.crt` / `ca.key`：测试根 CA
- `server.crt` / `server.key`：服务端 X.509 证书（SAN: `localhost`, `127.0.0.1`）
- `client.crt` / `client.key`：客户端 X.509 证书（EKU: `clientAuth`）
- `client.pub`：提取的客户端 RSA 公钥（用于静态公钥固定校验）

---

## 运行测试

```bash
# 运行 datasource-mgr 全部单元测试
go test -v ./services/datasource-mgr/...

# 运行整个 Go 工作区测试
make test-go
```
