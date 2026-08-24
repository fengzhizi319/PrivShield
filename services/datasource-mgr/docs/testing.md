# 模拟数据源服务 (Mock Datasource Manager) — 测试规范

> 本文档详细说明 **数联天下 · 数盾 (`PrivShield`)** 模拟数据源模块（`services/datasource-mgr`）的测试架构、用例覆盖与执行方式。

---

## 1. 测试全景与模块覆盖

| 测试包 | 测试文件 | 覆盖内容与核心断言 |
|---|---|---|
| `internal/grpcserver` | `server_test.go` | **全部 9 个 gRPC 方法**（Health/GetYibaoData/GetKangyangData/GetMockData3/GetMockData4/GetDataBySource/ListMockSources/GetDataSource/TestConnection）、输入校验、mTLS 凭证构造、CA 链校验与公钥固定 (Public Key Pinning) |
| `internal/handlers` | `handlers_test.go` | **HTTP REST Handler 层**（Health、API 1 医保数据申请、API 2 康养数据申请、API 3/4 预留数据申请、数据源列表、数据源记录获取、连通性测试、表结构元数据） |
| `internal/config` | `config_test.go` | 默认配置、自定义环境变量加载、`Address()`、`GRPCAddress()` 与 mTLS 配置解析 |
| `internal/models` | `models_test.go` | `MockDataSource`、`DataQueryResponse`、`MetadataResponse` 的 JSON 序列化与反序列化验证 |

---

## 2. 运行测试命令

```bash
# 1. 运行 datasource-mgr 全部单元测试
go test -v ./services/datasource-mgr/...

# 2. 运行全仓 Go 测试
make test-go
```
