# 模拟数据源服务 (Mock Datasource Manager)

`services/datasource-mgr` 是 PrivShield 平台的轻量级模拟数据源服务。**本项目仅用于开发、测试与调试阶段的模拟数据供给与通信验证**；在生产环境中，调度中枢与下游系统将直接对接真实外部数据源。

---

## 核心功能与特性

- **固定模拟数据库**：内置真实脱敏场景常用的医保就医结算数据（`yibao.csv`）与康养健康档案数据（`kangyang.csv`）；
- **4 个专用模拟数据接口**：
  - **API 1**：申请医保就医与结算模拟数据 (`GET /api/v1/yibao` / `rpc GetYibaoData`)
  - **API 2**：申请康养体检与慢病模拟数据 (`GET /api/v1/kangyang` / `rpc GetKangyangData`)
  - **API 3**：预留政务数据源 3 扩展模拟接口 (`GET /api/v1/mock3` / `rpc GetMockData3`)
  - **API 4**：预留企业数据源 4 扩展模拟接口 (`GET /api/v1/mock4` / `rpc GetMockData4`)
- **双协议通信支持**：对外提供 HTTP REST（端口 `:8083`），对内提供高性能 gRPC（端口 `:50053`）；
- **mTLS 双向认证与公钥固定**：gRPC 服务支持 TLS 1.3 证书校验与客户端公钥固定（Public Key Pinning）；
- **开发轻量化**：去除动态关系型数据库连接池、分类引擎联动与重型持久化开销，零外部依赖极速冷启动。

---

## 快速开始

### 本地启动

```bash
cd services/datasource-mgr
bash run.sh
```

默认监听：
- **HTTP REST**：`http://127.0.0.1:8083`
- **gRPC (insecure)**：`127.0.0.1:50053`

### 生产调试启动（启用 mTLS）

```bash
DATASOURCE_MGR_HOST=0.0.0.0 \
DATASOURCE_MGR_PORT=8083 \
DATASOURCE_MGR_GRPC_HOST=0.0.0.0 \
DATASOURCE_MGR_GRPC_PORT=50053 \
DATASOURCE_MGR_TLS_ENABLED=true \
DATASOURCE_MGR_TLS_CERT_FILE=/certs/server.crt \
DATASOURCE_MGR_TLS_KEY_FILE=/certs/server.key \
DATASOURCE_MGR_TLS_CA_FILE=/certs/ca.crt \
DATASOURCE_MGR_TLS_CLIENT_AUTH=require \
DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE=/certs/client_pub.pem \
./bin/datasource-mgr
```

---

## 运行测试

```bash
# 运行全部单元测试
go test -v ./services/datasource-mgr/...

# 运行全仓 Go 测试
make test-go
```

---

## 详细文档目录

- 📘 [详细设计文档 (docs/design.md)](docs/design.md)
- 🔌 [API 接口规范与 Proto 定义 (docs/api.md)](docs/api.md)
- 🛠️ [运维与部署手册 (docs/ops.md)](docs/ops.md)
- 🧪 [测试规范与全景指南 (docs/testing.md)](docs/testing.md)
