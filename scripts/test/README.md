# 性能压测与质量评估脚本 (scripts/test)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 用于进行高并发极限吞吐压测、SLA 响应延迟百分位数基准评估（P50/P90/P99）以及分布式节点负载均衡验证的测试套件。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [`stress_test_suite.py` (异步高并发极限吞吐压测套件)](#stress_test_suitepy)

---

## 详细功能与启动命令

### `stress_test_suite.py`
- **作用说明**: 基于 Python 异步高并发协程（`asyncio` + `httpx`）对 PrivShield 各层服务（Agent 算力层脱敏、Service Hub 调度流水线、网关反向代理等）进行高并发极限吞吐压测，统计 QPS、错误率以及 P50/P90/P95/P99 延迟分布并生成分析报告。
- **参数选项**:
  - `--target <TARGET>`: 压测预设目标：`agent`（核心算力层脱敏）或 `hub`（数据调度中枢流水线）。
  - `--url <URL>`: 指定自定义压测 HTTP/REST 目标端点。
  - `-c, --concurrency <INT>`: 并发协程连接数（默认 50）。
  - `-d, --duration <INT>`: 持续压测时间（秒，默认 10）。
- **执行命令**:
  ```bash
  # 1. 压测 PrivShield Agent 核心脱敏原语 (50 并发, 持续 10 秒)
  python scripts/test/stress_test_suite.py --target agent --concurrency 50 --duration 10
  ```
  ```bash
  # 2. 压测 Service Hub 调度中枢流水线 (30 并发, 持续 10 秒)
  python scripts/test/stress_test_suite.py --target hub --concurrency 30 --duration 10
  ```
  ```bash
  # 3. 压测自定义 HTTP 端点或网关负载均衡器 (100 并发, 持续 20 秒)
  python scripts/test/stress_test_suite.py \
    --url http://127.0.0.1:8080/v1/privacy/mask \
    --concurrency 100 \
    --duration 20
  ```

---

### 压测输出指标报告示例

```text
============================================================
  PrivShield 极限压测报告 (Target: agent)
============================================================
  并发连接数:   50
  压测时长:     10.00 秒
  总请求数:     2348 次
  成功请求:     2348 次 (100.00%)
  失败请求:     0 次 (0.00%)
  系统吞吐率:   234.80 QPS
------------------------------------------------------------
  平均耗时:     21.23 ms
  中位数 (P50): 18.45 ms
  P90 延迟:     32.10 ms
  P95 延迟:     41.67 ms
  P99 延迟:     58.32 ms
============================================================
```
