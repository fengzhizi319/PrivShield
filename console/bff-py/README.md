# Privacy Test Console - Backend

Python FastAPI 代理服务，用于转发请求到 `PrivShield` 并提供示例数据。

## 运行

```bash
cd console/bff-py
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
./run.sh
```

## 单元测试

```bash
cd console/bff-py
source .venv/bin/activate
pip install -r requirements.txt  # 已包含 pytest 所需依赖
pytest tests -v
```

测试使用 `fastapi.testclient.TestClient` 调用应用路由，并对 `app.main.agent_client.request` 进行 mock，因此**不需要**真实启动 `PrivShield`。

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PRIVACY_AGENT_URL` | `http://127.0.0.1:8079` | `PrivShield` REST 地址 |
| `PRIVACY_AGENT_API_KEY` | - | 认证 API Key（agent 开启 auth 时） |
| `PRIVACY_CONSOLE_HOST` | `127.0.0.1` | 后端监听地址 |
| `PRIVACY_CONSOLE_PORT` | `8080` | 后端监听端口 |
| `PRIVACY_CONSOLE_STATIC_DIR` | `../web/dist` | 前端构建产物目录 |
| `CONSOLE_API_KEY` | - | Console 自身 API Key（设置后 `/api/*` 需携带 `Authorization: Bearer`） |
| `CONSOLE_RATE_LIMIT` | `600` | 单 IP 每分钟请求上限（0 表示不限流） |
| `CONSOLE_MAX_UPLOAD_BYTES` | `10485760` | `/api/upload` 上传大小上限（字节，默认 10MB） |
| `LB_ALLOWED_HOSTS` | - | `/api/lb_test` 探测目标 host 白名单（逗号分隔，命中后跳过私有网段校验） |
| `LB_ALLOW_PRIVATE_IPS` | `false` | 允许 `/api/lb_test` 探测私有网段地址（内网部署场景） |

## 核心文件

- `app/main.py` - FastAPI 入口，注册路由和静态资源
- `app/client.py` - 转发请求到 agent 的 httpx 客户端
- `app/config.py` - 环境变量配置
- `app/fixtures/samples.py` - 所有端点的示例数据
- `smoke_test.py` - 自动化冒烟测试

## 文档

- `docs/design.md` - 架构与设计决策（选型、转发机制、SPA 托管、后端身份标识）
- `docs/api.md` - 全部 HTTP 端点的请求 / 响应说明与 cURL 示例
- `docs/test.md` - 测试策略、单元 / 冒烟测试与调试技巧
- `docs/ops.md` - 运维手册（开发 / 生产模式区别、配置、跨域 CORS 解决方案、启停与排障）

## 烟雾测试

```bash
source .venv/bin/activate
python smoke_test.py
```

