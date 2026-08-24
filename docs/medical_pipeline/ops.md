# 医疗敏感数据全流程治理流水线 — 运维与部署指南 (Ops)

> **文档版本**: 1.0  
> **面向对象**: SRE 工程师、运维人员、测试开发

---

## 1. 命令行工具与脚本使用

### 1.1 数据生成脚本 (`scripts/data/generate_medical_data.py`)

用于生成高仿真医疗记录 `kangyang.csv`（脚本默认 20 条；仓库内各样例目录中预置的 `kangyang.csv` 均为 **100 条**）：

```bash
cd /path/to/PrivShield

# 生成 100 条数据保存到 data/kangyang.csv (默认 seed 2026，与仓库预置样例一致)
python scripts/data/generate_medical_data.py --output data/kangyang.csv --count 100

# 自定义记录条数与种子
python scripts/data/generate_medical_data.py --output tmp/custom_data.csv --count 50 --seed 42
```

### 1.2 分发脚本至控制台后端样例目录

生成的 `kangyang.csv` 需要自动分发给测试控制台的 Python 与 Go 后端：

```bash
# 复制至 Python 后端样例目录
cp data/kangyang.csv console/bff-py/samples/kangyang.csv

# 复制至 Go 后端样例目录
cp data/kangyang.csv console/bff-go/internal/samples/kangyang.csv
```

---

## 2. Agent 服务部署与配置

### 2.1 环境变量配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `PRIVACY_REST_HOST` | `127.0.0.1` | Agent REST 服务主机地址 |
| `PRIVACY_REST_PORT` | `8079` | Agent REST 服务端口 |
| `PRIVACY_DYNCLASSIFICATION_RULES_DIR` | `rules` | 分类分级规则存放目录 |
| `PRIVACY_PROFILE` | — | 隐私配置文件 YAML 路径 |
| `PRIVACY_IMAGE_ALLOWED_DIRS` | `<cwd>/data`、`uploads`、`samples`、`medical_images` + 系统临时目录 | 图片输入路径沙箱白名单（`os.pathsep` 分隔）。仅允许读取白名单目录内的图片文件，拒绝 `../` 目录穿越与 symlink 逃逸（任意文件读取防护）。生产环境应显式设置为可信图片存储目录 |

### 2.2 启动 Agent REST 服务

```bash
# 方式 1: 在 Python 环境下启动单进程服务器
python -m engine.server

# 方式 2: 通过 Makefile 目标启动
make run-server
```

---

## 3. 控制台代理后端启动

### 3.1 启动 Python 控制台后端

```bash
cd console/bff-py
./run.sh
# 服务监听在 http://127.0.0.1:8000
```

### 3.2 启动 Go 控制台后端

```bash
cd console/bff-go
go run cmd/server/main.go
# 服务监听在 http://127.0.0.1:8080
```

### 3.3 快速同时启动开发环境 (Go + Vite HMR)

```bash
./scripts/dev/dev-start-go.sh
```

---

## 4. 常见问题排查 (Troubleshooting)

### Q1: 前端请求 `/api/medical_pipeline` 返回 502 Bad Gateway
- **原因**: 控制台代理后端无法连接上游 `PrivShield` REST 服务 (默认 `127.0.0.1:8079`)。
- **解决**:
  1. 确认 `python -m engine.server` 正常启动并在 `8079` 监听。
  2. 检查 `PRIVACY_REST_HOST` 与 `PRIVACY_REST_PORT` 配置。

### Q2: 单元测试提示 `ImportError: cannot import name 'gen_id_card'`
- **原因**: Python 模块路径搜索顺序异常或未添加环境变量。
- **解决**: 运行测试时前置 `PYTHONPATH=.` 环境变量：
  ```bash
  PYTHONPATH=. pytest tests/test_medical_pipeline.py -v
  ```
