# 现代测试工程、Pytest 异步测试与性能基准评测技术指南 / Testing Engineering, Pytest & Benchmarking Technical Guide

## 1. 技术简介 / Introduction

在涉及金融、医疗与核心数据隐私保护的系统中，**软件质量工程（Software Quality Engineering）** 直接关系到数据资产的安全底线。任何脱敏算法的边界遗漏、差分隐私噪声分布偏差或多线程并发竞态，都会导致不可逆的隐私泄露。

`PrivShield` 建立了多层次、跨语言的严密测试防护网：
- **微观单元测试（Unit Testing）**：基于 **pytest** 与 **pytest-asyncio**，针对 100+ 脱敏规则、DP 敏感度、K-匿名等价类与 AST 算子实现 >90% 代码分支覆盖；
- **ML 模型隔离打桩（ML Mocking Strategy）**：在无 GPU/无网络依赖的 CI 环境中对 Small-NER 与本地 LLM 进行优雅 Mock，保障测试套件在 10 秒内执行完成；
- **隐私原语性能基准测试（Benchmarking）**：基于 **pytest-benchmark** 测量纳秒级脱敏与毫秒级加噪吞吐量；
- **跨语言端到端全链路集成（Go E2E Tests）**：通过真实 HTTP/gRPC 协议验证从 Go BFF、Go 中台微服务到 Python 引擎的完整调用链。

```text
                  PrivShield 自动化测试金字塔 (Testing Pyramid)
                                      ▲
                                     / \
                                    /   \
                                   / E2E \    Go TestRealE2E (真实多微服务链路)
                                  / 集成测试 \
                                 /───────────\
                                / 性能基准测试 \  pytest-benchmark (吞吐与延迟压测)
                               /───────────────\
                              / 异步 API 契约测试 \  TestClient & grpc.aio Channel
                             /───────────────────\
                            /   算法与规则单元测试   \  DP/LDP/Masking/AST Operators (Mock ML)
                           /─────────────────────────\
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 Pytest Fixture 依赖注入与测试夹具 / Test Fixtures & In-Process Channels

文件 / File：[`tests/conftest.py`](tests/conftest.py)

#### (1) 同步与异步 FastAPI TestClient

```python
import pytest
from fastapi.testclient import TestClient
from engine.main import app
from engine.service import PrivacyService

@pytest.fixture
def client():
    """提供轻量级 FastAPI TestClient，无需启动物理端口。"""
    with TestClient(app) as c:
        yield c

@pytest.fixture
def privacy_service():
    """提供独立的 PrivacyService 单例实例。"""
    return PrivacyService()
```

#### (2) 内存级 gRPC In-Process 通道测试

无需启动外部 TCP 监听，利用 `grpc.aio.insecure_channel` 直接测试 gRPC Servicer：

```python
@pytest.fixture
async def grpc_stub(grpc_server):
    """创建与本地测试 gRPC 服务端连接的异步 Client Stub。"""
    async with grpc.aio.insecure_channel("localhost:50051") as channel:
        stub = privacy_pb2_grpc.PrivacyServiceStub(channel)
        yield stub
```

---

### 2.2 重量级 ML 模型测试隔离打桩 / ML Mocking Strategy for CI

文件 / File：[`tests/dynclassification/test_ner_adapter.py`](tests/dynclassification/test_ner_adapter.py) & [`tests/dynclassification/test_llm_adapter.py`](tests/dynclassification/test_llm_adapter.py)

为了让 CI 流水线脱离数十 GB 的 PyTorch 权重并在纯 CPU 环境高速运行，单元测试通过 `unittest.mock` 对底层适配器打桩：

```python
from unittest.mock import MagicMock, patch

def test_funnel_arbitration_with_mocked_llm():
    """测试三层漏斗在遇到规则冲突时调度 LLM 仲裁的决策逻辑。"""
    with patch("engine.dynclassification.llm_adapter.LlmAdapter.arbitrate") as mock_arbitrate:
        # 1. 构造 Mock 返回：裁定等级为 S3，置信度 0.95
        mock_arbitrate.return_value = MagicMock(
            level="S3",
            confidence=0.95,
            reasoning="Mocked LLM reasoning: context indicates high sensitivity",
            tags=[...],
        )

        # 2. 执行漏斗评估
        funnel = ClassificationFunnel(...)
        result = funnel.classify_field("medical_desc", "患者确诊急性感染")

        # 3. 严格断言
        assert result.final_level == "S3"
        assert result.engine_layer == "L3_LLM"
        assert "Mocked LLM" in result.reasoning
```

---

### 2.3 隐私原语微基准性能评测 / Micro-Benchmarking

文件 / File：[`tests/benchmark_primitives.py`](tests/benchmark_primitives.py)

```python
class TestDPBenchmarks:
    """差分隐私算子基准评测。"""

    def test_benchmark_dp_count_10k(self, benchmark, dp_api):
        """测试 10,000 个浮点数数据点的 Laplace 计数加噪性能。"""
        data = np.random.randn(10_000).tolist()
        res = benchmark(dp_api.count, data, epsilon=0.1)
        assert res > 0

    def test_benchmark_masking_batch_100(self, benchmark, masking_api):
        """测试 100 条手机号/身份证批量脱敏吞吐量。"""
        fields = ["mobile", "id_card", "email"] * 33
        values = ["13800138000", "110101199003072345", "user@example.com"] * 33
        res = benchmark(masking_api.mask_batch, fields, values)
        assert len(res) == 99
```

运行性能基准测试命令：
```bash
PYTHONPATH=. pytest tests/benchmark_primitives.py --benchmark-only
```

---

### 2.4 Go 中台真实 E2E 链路测试 / Real End-to-End Testing

文件 / File：[`services/service-hub/internal/handlers/e2e_test.go`](services/service-hub/internal/handlers/e2e_test.go)

在真实集成测试中，Go 测试套件通过设置环境变量 `PRIVSHIELD_E2E=1` 启动对运行中真实微服务群的端到端调用验证：

```go
func TestRealE2E_FullPipeline(t *testing.T) {
    if os.Getenv("PRIVSHIELD_E2E") == "" {
        t.Skip("Skipping real E2E test; set PRIVSHIELD_E2E=1 to enable")
    }

    // 1. 调用 Go BFF 发送待脱敏与分类记录
    // 2. 验证 Service Hub 6 阶段流水线调度
    // 3. 验证 Python Agent 差分隐私加噪正确性
    // 4. 验证 Audit-Log 不可篡改哈希链写入成功
}
```
