# 数据源管理与特征探查 (Datasource Manager)

`services/datasource-mgr` 是 PrivShield 平台的企业级数据源统一纳管与敏感特征自动探查微服务。模块提供 **REST (HTTP/JSON :8083) + gRPC (mTLS :50053)** 双协议接入，支持多源异构数据源连接管理、连通性探测、敏感特征自动打标识别、安全采样读取与全生命周期访问审计。

---

## 核心功能特性

- **双协议接入**：提供标准 REST API（供前端控制台/BFF 使用）与高性能 gRPC 接口（端口 `:50053`，供调度流水线使用）；
- **零信任 mTLS 与公钥固定**：gRPC 通道支持 TLS 1.3 双向证书认证与客户端公钥固定（Public Key Pinning）；
- **多源异构资产纳管**：统一支持关系型数据库（MySQL/PG/Oracle）、API 接口及 CSV 文件等数据源；
- **敏感特征自动探查**：联动上游 PrivShield Agent 三层动态分类漏斗，自动识别 PII 敏感字段并标记 L1-L5 安全等级；
- **安全沙箱采样读取**：内置路径穿越防护（防 LFI）与 50,000 行内存上限防护（DoS 防护）；
- **全量访问审计与存证**：对所有数据源操作记录结构化审计日志；
- **高可用与生产加固**：Slowloris 防护、32 MiB MaxBodySize 限制、Prometheus `/metrics` 监控与 SQLite WAL 持久化。

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

### 生产启动（启用 mTLS 与公钥固定）

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
- 📋 [产品需求文档 (docs/prd.md)](docs/prd.md)
