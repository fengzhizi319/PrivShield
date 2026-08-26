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

---

## 3. Pytest Fixture 依赖注入与测试隔离 / Fixture Dependency Injection & Test Isolation

文件 / File：[`tests/conftest.py`](tests/conftest.py)

### 3.1 全局自动使用 Fixture：隐私预算重置

差分隐私预算是全局有状态资源。如果不在测试间重置，前一个测试消耗的预算会影响后续测试的断言结果。PrivShield 通过 `autouse=True` fixture 实现每个测试前的自动清理：

```python
@pytest.fixture(autouse=True)
def reset_all_budgets():
    """每个测试前重置隐私预算状态（自动使用，无需显式引用）。"""
    # 1. 清空默认注册表中的所有 BudgetAccountant 实例
    default_registry.reset()

    # 2. 同步 REST 全局单例服务的预算引用
    try:
        from engine.main import service
        if hasattr(service, "dp_api"):
            service.dp_api.budget = default_registry.get_or_create(service.namespace)
    except (ImportError, AttributeError):
        pass

    yield  # 测试执行点
```

> **学习要点**：`autouse=True` 使得该 fixture 无需被测试函数显式引用就会自动执行。这对于「全局状态清理」「数据库重置」等横切关注点非常有用。

### 3.2 scipy/torch 兼容性补丁

CI 环境中通过 `MagicMock` 模拟 `torch` 模块，但 scipy 内部的 `is_torch_array()` 会调用 `issubclass()` 检查，当 `torch` 是 MagicMock 时抛出 `TypeError`。`conftest.py` 在导入时一次性修复此问题：

```python
try:
    from scipy._external.array_api_compat.common import _helpers as _scipy_helpers
    _orig = _scipy_helpers.is_torch_array

    def _safe_is_torch_array(x):
        try:
            return _orig(x)
        except TypeError:
            return False  # MagicMock 不是真正的 torch，安全返回 False

    _scipy_helpers.is_torch_array = _safe_is_torch_array
except Exception:
    pass  # scipy 未安装或内部 API 已变更
```

---

## 4. 测试目录结构与组织 / Test Directory Structure

```text
tests/
├── conftest.py                    # 全局 fixture 与兼容性补丁
├── __init__.py
├── api/                           # REST/gRPC API 契约测试
│   ├── test_rest.py               # FastAPI TestClient 同步测试
│   └── test_grpc.py               # grpc.aio 异步通道测试
├── privacy/                       # 隐私原语单元测试
│   ├── test_masking.py            # 脱敏规则测试
│   ├── test_dp.py                 # 差分隐私测试
│   ├── test_ldp.py                # 本地差分隐私测试
│   ├── test_kano.py               # K-匿名测试
│   ├── test_qol.py                # 查询混淆测试
│   └── test_budget.py             # 预算会计测试
├── dynclassification/             # 动态分类分级测试
│   ├── test_funnel.py             # 三层漏斗测试
│   ├── test_ner_adapter.py        # NER 适配器 Mock 测试
│   ├── test_llm_adapter.py        # LLM 适配器 Mock 测试
│   ├── test_downgrade_override.py # 降级覆盖规则测试
│   └── test_standards_switching.py# 分类体系切换测试
├── gateway/                       # 网关与负载均衡测试
│   ├── test_balancer.py           # 调度算法测试
│   └── test_proxy.py              # 代理转发测试
├── security/                      # 安全功能测试
│   ├── test_auth.py               # 认证授权测试
│   ├── test_mtls.py               # mTLS 测试
│   ├── test_rate_limit.py         # 速率限制测试
│   └── test_whitelist.py          # 白名单热加载测试
├── observability/                 # 可观测性测试
│   └── test_metrics.py            # Prometheus 指标测试
├── perf/                          # 性能与压力测试
│   └── test_high_concurrency.py   # 高并发压测
├── benchmark_primitives.py        # 隐私原语微基准测试
└── test_all_features_high_concurrency.py  # 全功能高并发测试
```

---

## 5. ML 模型测试隔离打桩策略详解 / ML Mocking Strategy Deep Dive

文件 / File：[`tests/dynclassification/test_llm_adapter.py`](tests/dynclassification/test_llm_adapter.py)

### 5.1 为什么需要 Mock ML 模型？

| 问题 | 影响 |
|---|---|
| PyTorch + Transformers > 5GB | CI 环境磁盘空间不足 |
| GPU 不可用 | CPU 推理极慢，测试套件超时 |
| 模型下载需要网络 | CI 无外网访问 |
| 模型输出不确定性 | 断言结果不稳定 |

### 5.2 三层 Mock 策略

```python
from unittest.mock import MagicMock, patch, PropertyMock

def test_funnel_with_mocked_llm():
    """测试三层漏斗在规则冲突时调度 LLM 仲裁的决策逻辑。"""

    # Layer 1 Mock: 规则引擎返回低置信度，触发降级
    with patch("engine.dynclassification.engine.ConfigurableRuleEngine.evaluate") as mock_rule:
        mock_rule.return_value = MagicMock(level="S2", confidence=0.4)

        # Layer 2 Mock: NER 返回空结果
        with patch("engine.dynclassification.ner_adapter.NerAdapter.classify") as mock_ner:
            mock_ner.return_value = None

            # Layer 3 Mock: LLM 仲裁返回高置信度
            with patch("engine.dynclassification.llm_adapter.LlmAdapter.arbitrate") as mock_llm:
                mock_llm.return_value = MagicMock(
                    level="S3",
                    confidence=0.95,
                    reasoning="Mocked: context indicates high sensitivity",
                )

                funnel = ClassificationFunnel(...)
                result = funnel.classify_field("medical_desc", "患者确诊急性感染")

                assert result.final_level == "S3"
                assert result.engine_layer == "L3_LLM"
                mock_llm.assert_called_once()  # 确认 LLM 被调用了一次
```

### 5.3 Mock 最佳实践

1. **尽量 Mock 边界而非内部**：Mock `LlmAdapter.arbitrate` 而非 `torch.nn.Linear.forward`
2. **验证调用参数**：`mock.assert_called_once_with(expected_args)` 确保传参正确
3. **避免过度 Mock**：只 Mock 外部依赖（网络、GPU、文件系统），不 Mock 纯函数
4. **使用 `patch.object` 而非 `patch`**：更精确，减少字符串路径错误

---

## 6. 异步测试模式 / Async Testing Patterns

### 6.1 pytest-asyncio 基础用法

```python
import pytest
import pytest_asyncio

@pytest.mark.asyncio
async def test_dp_count_async():
    """异步测试差分隐私计数。"""
    dp = DPMechanism(epsilon=0.1)
    result = await dp.count_async([1, 2, 3, 4, 5])
    assert abs(result - 5) < 3  # 加噪后应在真实值附近
```

### 6.2 gRPC 异步通道测试

```python
@pytest_asyncio.fixture
async def grpc_stub():
    """创建与测试 gRPC 服务端连接的异步 Client Stub。"""
    async with grpc.aio.insecure_channel("localhost:50051") as channel:
        yield privacy_pb2_grpc.PrivacyServiceStub(channel)

@pytest.mark.asyncio
async def test_grpc_mask_field(grpc_stub):
    """通过 gRPC 异步通道测试脱敏。"""
    response = await grpc_stub.MaskField(
        privacy_pb2.MaskFieldRequest(field_name="mobile", value="13800138000")
    )
    assert response.masked_value != "13800138000"  # 必须已脱敏
    assert "138" in response.masked_value  # 前缀应保留
```

---

## 7. 基准测试方法论 / Benchmarking Methodology

文件 / File：[`tests/benchmark_primitives.py`](tests/benchmark_primitives.py)

### 7.1 pytest-benchmark 使用指南

```python
class TestDPBenchmarks:
    def test_benchmark_dp_count_10k(self, benchmark, dp_api):
        """测试 10,000 个浮点数数据点的 Laplace 计数加噪性能。"""
        data = np.random.randn(10_000).tolist()
        # benchmark 包装器自动测量多次并统计 min/max/mean/stddev
        res = benchmark(dp_api.count, data, epsilon=0.1)
        assert res > 0

    def test_benchmark_masking_batch_100(self, benchmark, masking_api):
        """测试 100 条手机号/身份证批量脱敏吞吐量。"""
        fields = ["mobile", "id_card", "email"] * 33
        values = ["13800138000", "110101199003072345", "user@example.com"] * 33
        res = benchmark(masking_api.mask_batch, fields, values)
        assert len(res) == 99
```

### 7.2 运行基准测试命令

```bash
# 运行所有基准测试
PYTHONPATH=. pytest tests/benchmark_primitives.py --benchmark-only

# 对比两次运行的结果
PYTHONPATH=. pytest tests/benchmark_primitives.py --benchmark-only --benchmark-compare

# 输出 JSON 格式结果（供 CI 分析）
PYTHONPATH=. pytest tests/benchmark_primitives.py --benchmark-only --benchmark-json=output.json

# 仅运行特定基准
PYTHONPATH=. pytest tests/benchmark_primitives.py -k "dp_count" -v
```

### 7.3 性能基准目标

| 操作 | 目标延迟 | 目标吞吐量 |
|---|---|---|
| 单字段脱敏 | < 0.05ms | > 20,000 ops/s |
| 100 条批量脱敏 | < 5ms | > 200 batches/s |
| DP 计数 (10K 点) | < 1ms | > 1,000 ops/s |
| 分类 (Layer-1 规则) | < 0.1ms | > 10,000 ops/s |
| 分类 (Layer-3 LLM) | < 5s | N/A (受模型推理限制) |

---

## 8. 测试运行命令速查 / Test Commands Quick Reference

```bash
# 运行全部测试
PYTHONPATH=. pytest tests -q

# 运行特定目录
PYTHONPATH=. pytest tests/privacy/ -v
PYTHONPATH=. pytest tests/dynclassification/ -v
PYTHONPATH=. pytest tests/security/ -v

# 运行特定测试文件
PYTHONPATH=. pytest tests/api/test_rest.py -v

# 运行特定测试函数
PYTHONPATH=. pytest tests/privacy/test_dp.py::test_laplace_count -v

# 并行执行（需 pytest-xdist）
PYTHONPATH=. pytest tests -n auto -q

# 显示覆盖率报告
PYTHONPATH=. pytest tests --cov=engine --cov-report=term-missing -q

# 运行 Go E2E 测试
PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./services/service-hub/internal/handlers/

# 运行高并发压测
PYTHONPATH=. pytest tests/test_all_features_high_concurrency.py -v
```

---

## 9. 测试编写最佳实践 / Testing Best Practices

### 9.1 测试命名规范

```python
# 格式: test_<被测功能>_<场景>_<期望结果>
def test_masking_mobile_full_number_returns_prefix_preserved():
    """手机号完整号码脱敏应保留前3后4。"""

def test_dp_budget_exhausted_raises_error():
    """预算耗尽时应抛出 PrivacyBudgetExhaustedError。"""

def test_funnel_rule_low_confidence_falls_to_ner():
    """规则置信度不足时应降级到 NER 层。"""
```

### 9.2 测试组织原则

1. **AAA 模式**：Arrange（准备） → Act（执行） → Assert（断言）
2. **每个测试只验证一件事**：失败时能立即定位问题
3. **测试数据与测试逻辑分离**：使用 fixture 或 `@pytest.mark.parametrize` 提供数据
4. **避免测试间依赖**：每个测试应独立运行，不依赖其他测试的执行顺序或结果

```python
@pytest.mark.parametrize("field_name,value,expected_pattern", [
    ("mobile", "13800138000", "138****8000"),
    ("id_card", "110101199003072345", "110***********2345"),
    ("email", "user@example.com", "u***@example.com"),
])
def test_masking_parametrized(field_name, value, expected_pattern):
    """参数化测试：一个测试函数验证多种 PII 脱敏规则。"""
    result = mask_field(field_name, value)
    assert fnmatch.fnmatch(result, expected_pattern)
```
