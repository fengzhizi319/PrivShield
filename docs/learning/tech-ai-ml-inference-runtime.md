# AI/ML 推理加速、模型量化与微调技术指南 / AI/ML Inference, ONNX, ModelScope, MLX & LoRA Technical Guide

## 1. 技术简介 / Introduction

在 `PrivShield` 隐私治理架构中，机器学习（ML）与大语言模型（LLM）被深度集成于数据分类分级体系中：
- **Layer 2 (Small-NER)**：负责对无字段名提示的纯文本或混杂数据进行快速命名实体识别（Token Classification），抽取出身份证、病历、手机号、银行卡等微观实体；
- **Layer 3 (Local LLM / VLM)**：负责在规则出现歧义或低置信度时，进行高阶语义理解与 Chain-of-Thought（CoT）推理仲裁。

为了兼顾微服务边车的**极速启动（<1s）、轻量内存占用（Core 镜像仅 ~100MB）与高并发下的稳定性（防 OOM 崩溃）**，`PrivShield` 设计了先进的 AI/ML 运行时架构。

```text
                  PrivShield Sidecar 启动 (Startup)
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 1. 核心轻量进程就绪 (Core Agent Ready)          │
         │    - 零 PyTorch / Transformers 模块级导入      │
         │    - 规则引擎与脱敏原语毫秒级加载               │
         └──────────────────────┬───────────────────────┘
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 2. 懒加载 (Lazy-Loading) 触发机制             │
         │    - 仅在首次接收到 NER/LLM 评估请求时初始化    │
         │    - 缺失权重/库自动优雅降级，不阻断主链路      │
         └──────────────────────┬───────────────────────┘
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 3. 多后端推理适配引擎 (Multi-Engine Adapters)  │
         │    ├── ONNX Runtime (CPU / CUDA / TensorRT)  │
         │    ├── Apple Silicon MLX (统一内存极致加速)   │
         │    ├── PyTorch + HuggingFace Transformers    │
         │    └── 远程 vLLM / OpenAI API (企业级集中集群)│
         └──────────────────────┬───────────────────────┘
                                │
                                ▼
         ┌──────────────────────────────────────────────┐
         │ 4. 生产级并发隔离与内存防 OOM 熔断护栏          │
         │    - 进程级信号量: PRIVACY_LLM_MAX_CONCURRENCY│
         │    - 剩余可用内存守卫: PRIVACY_LLM_MIN_FREE_MEM │
         └──────────────────────────────────────────────┘
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 懒加载与多后端适配器 / Lazy-Loading & Multi-Engine Adapters

文件 / File：[`engine/dynclassification/ner_adapter.py`](engine/dynclassification/ner_adapter.py) & [`engine/dynclassification/llm_adapter.py`](engine/dynclassification/llm_adapter.py)

为了防止在模块导入阶段因尝试加载数十 GB 的模型权重导致进程假死，所有模型适配器均采用双重检查锁（Double-Checked Locking）实现按需懒初始化：

```python
class LlmAdapter:
    """Layer-3 LLM 适配器：支持延迟加载与多后端切换。"""

    def __init__(self):
        self._classifier = None
        self._init_attempted = False
        self._lock = threading.Lock()

    def _lazy_init(self) -> None:
        """首次调用时按需加载模型，失败则优雅降级。"""
        if self._init_attempted:
            return
        with self._lock:
            if self._init_attempted:
                return
            self._init_attempted = True
            
            provider = os.environ.get("PRIVACY_LLM_PROVIDER", "qwen3").lower()
            try:
                if provider == "vllm" or provider == "openai":
                    from .llm_engines import OpenAILlmEngine
                    self._classifier = OpenAILlmEngine(...)
                elif provider == "mlx":
                    from .mlx_llm_engine import MlxLlmEngine
                    self._classifier = MlxLlmEngine(...)
                else:
                    from .llm_engines import Qwen3Classifier
                    self._classifier = Qwen3Classifier(...)
                logger.info(f"LLM classifier initialized successfully with provider: {provider}")
            except Exception as e:
                logger.warning(f"LLM classifier initialization failed: {e}. Degrading to rule-only.")
                self._classifier = None
```

---

### 2.2 进程级推理并发控制与内存防 OOM 熔断 / Concurrency Cap & Memory Safety

文件 / File：[`engine/dynclassification/llm_adapter.py`](engine/dynclassification/llm_adapter.py#L50-L150)

在多 Worker 或高并发请求下，多个线程同时调用 LLM 推理会迅速打满 GPU 显存或系统物理内存（RAM），触发操作系统 OOM Killer，导致 gRPC/HTTP 连接被重置（`connection reset by peer`）。

`PrivShield` 创新实现了**双重安全护栏**：

```python
# 1. 进程级全局推理信号量（由所有 LlmAdapter 共享）
_LLM_MAX_CONCURRENCY = int(os.environ.get("PRIVACY_LLM_MAX_CONCURRENCY", "1"))
_LLM_SEMAPHORE = threading.BoundedSemaphore(_LLM_MAX_CONCURRENCY)
_LLM_SEMAPHORE_WAIT = float(os.environ.get("PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS", "30.0"))

# 2. 系统物理可用内存熔断保护（低于阈值直接跳过 LLM，防止触发 OOM Killer）
_LLM_MIN_FREE_MEM_MB = float(os.environ.get("PRIVACY_LLM_MIN_FREE_MEM_MB", "512.0"))

def _check_system_memory_safe() -> bool:
    """检查宿主机当前可用物理内存是否高于安全红线。"""
    try:
        import psutil
        free_mb = psutil.virtual_memory().available / (1024 * 1024)
        if free_mb < _LLM_MIN_FREE_MEM_MB:
            logger.warning(f"Available memory {free_mb:.1f}MB is below safe threshold {_LLM_MIN_FREE_MEM_MB}MB. Skipping LLM.")
            return False
        return True
    except ImportError:
        return True

def safe_llm_inference(predict_fn: Callable[[], T]) -> T | None:
    """受并发信号量与内存安全阈值保护的 LLM 推理执行包装器。"""
    if not _check_system_memory_safe():
        return None  # 内存不足，优雅降级

    acquired = _LLM_SEMAPHORE.acquire(timeout=_LLM_SEMAPHORE_WAIT)
    if not acquired:
        logger.warning(f"LLM semaphore acquire timeout after {_LLM_SEMAPHORE_WAIT}s. Degrading.")
        return None
    try:
        return predict_fn()
    finally:
        _LLM_SEMAPHORE.release()
```

---

### 2.3 ONNX Runtime 与 StructBERT 轻量实体提取 / ONNX Small-NER

文件 / File：[`engine/dynclassification/ner_engines.py`](engine/dynclassification/ner_engines.py)

针对 Layer 2 Small-NER，`PrivShield` 将 ModelScope 开源的 StructBERT 中文实体识别模型导出为标准 **ONNX** 格式：
- 使用 `onnxruntime.InferenceSession` 替代庞大的 PyTorch 运行时；
- 内存开销从 1.5GB 骤降至 < 200MB；
- 推理速度相比原始 PyTorch 提升 3~5 倍，在 CPU 单核上可达 2ms / 样本。

---

### 2.4 LoRA 轻量微调与 PEFT 训练管线 / LoRA Fine-Tuning Pipeline

文档参考 / Docs：[`docs/llmlora/`](docs/llmlora/)

针对 Qwen-3.5-0.8B / 7B 模型在数据分类分级任务中的定制，`PrivShield` 采用了 **LoRA (Low-Rank Adaptation)** 参数高效微调方案：

$$W = W_0 + \Delta W = W_0 + \frac{\alpha}{r} \cdot B \cdot A$$

其中：
- $W_0 \in \mathbb{R}^{d \times k}$ 为冻结的预训练基座模型权重；
- $A \in \mathbb{R}^{r \times k}, B \in \mathbb{R}^{d \times r}$ 为低秩可训练矩阵，秩 $r = 16$；
- 缩放系数 $\alpha = 32$，微调参数量仅占全量参数的 **0.05%**；
- 目标微调模块：`["q_proj", "k_proj", "v_proj", "o_proj", "gate_proj", "up_proj", "down_proj"]`。

微调后的 LoRA Adapter 权重仅几十 MB，便于动态分发并在推理时无缝热插拔。

---

## 3. 离线模型分发与 CI 测试打桩 / Model Artifacts & Testing

1. **一键离线下载脚本**：
   - LLM 下载：`python -m engine.privacy.download_model`（从 ModelScope / HuggingFace 拉取 Qwen 模型并校验 SHA256）；
   - NER 下载：`python -m engine.privacy.download_ner_model`。
2. **CI 单元测试 Mocking 机制**：
   - 单元测试运行在无 GPU 的精简环境，通过 `unittest.mock.patch` 模拟 `LlmAdapter` 和 `NerAdapter` 的返回值，确保测试套件在 10 秒内高速执行完毕，杜绝 CI 阶段下载重量级模型。

---

## 4. 懒加载架构深度解析 / Lazy-Loading Architecture Deep Dive

### 4.1 为什么需要懒加载？

PrivShield 的核心启动路径必须在 **<1 秒** 内就绪，这意味着不能在模块导入阶段加载数十 GB 的 PyTorch 权重。懒加载的核心思想是：**只在第一次真正需要时才加载重量级资源**。

```text
模块导入阶段 (Import Time)               首次调用阶段 (First Call)
┌─────────────────────────────┐     ┌─────────────────────────────┐
│  import llm_adapter           │     │  adapter.classify(text)       │
│  ├─ 解析类定义              │     │  ├─ _lazy_init()              │
│  ├─ 创建空实例变量          │     │  │   ├─ 加载 PyTorch           │
│  └─ 零 GPU/CPU 内存占用    │     │  │   ├─ 加载模型权重           │
│                               │     │  │   └─ 初始化分类器           │
│  耗时: <1ms                   │     │  └─ 执行实际分类             │
│  内存: ~0MB                   │     │                               │
└─────────────────────────────┘     │  耗时: 5~30s (首次)          │
                                        │  内存: 1.5~8GB (模型权重)   │
                                        └─────────────────────────────┘
```

### 4.2 双重检查锁 (Double-Checked Locking)

多线程环境下，多个请求可能同时触发懒加载。PrivShield 使用双重检查锁确保模型只加载一次：

```python
class LlmAdapter:
    def __init__(self):
        self._classifier = None
        self._init_attempted = False
        self._lock = threading.Lock()

    def _lazy_init(self) -> None:
        if self._init_attempted:
            return  # 快速路径：已初始化，无锁返回
        with self._lock:  # 慢速路径：加锁
            if self._init_attempted:
                return  # 二次检查：其他线程已完成初始化
            self._init_attempted = True
            try:
                # 加载模型...
                self._classifier = self._create_classifier()
            except Exception as e:
                logger.warning(f"LLM init failed: {e}. Degrading to rule-only.")
                self._classifier = None
```

> **学习要点**：双重检查锁是懒加载的经典模式。第一次检查避免不必要的锁竞争，第二次检查确保只有一个线程执行初始化。Python 的 GIL 保证了 `_init_attempted` 的读写是原子的。

---

## 5. 多后端推理引擎详解 / Multi-Engine Inference Backends

### 5.1 引擎优先级与自动降级

PrivShield 支持 4 种推理后端，按优先级自动降级：

```text
┌─────────────────────────────────────────────────────────────────────┐
│  LLM Provider 优先级 (PRIVACY_LLM_PROVIDER)                    │
│                                                                     │
│  1. vllm / openai   → 远程 HTTP API（企业级集中集群）          │
│     └─ 默认并发 16，无本地内存压力                              │
│                                                                     │
│  2. mlx             → Apple Silicon 统一内存加速                    │
│     └─ 仅 macOS + Apple Silicon，内存与 CPU 共享              │
│                                                                     │
│  3. qwen3 (默认)   → 本地 PyTorch + Transformers                   │
│     └─ 默认并发 1（串行化，防 OOM）                              │
│                                                                     │
│  4. 全部失败        → 优雅降级为 rule-only 模式                   │
│     └─ 仅使用 Layer-1 规则引擎，跳过 LLM 层                    │
└─────────────────────────────────────────────────────────────────────┘
```

### 5.2 ONNX Runtime Small-NER 引擎

ONNX Runtime 是微软开源的高性能推理引擎，将 PyTorch 模型导出为 ONNX 格式后可获得 3~5 倍 CPU 推理加速：

```python
class ONNXSmallNerEngine:
    """基于 ONNX Runtime 的轻量中文 NER 引擎。"""

    def __init__(self, model_path: str, vocab_path: str, label_mapping: dict):
        import onnxruntime as ort

        # 选择执行提供器：CUDA > CPU
        providers = ["CUDAExecutionProvider", "CPUExecutionProvider"] \
            if ort.get_device() == "GPU" else ["CPUExecutionProvider"]

        self._session = ort.InferenceSession(model_path, providers=providers)
        self._tokenizer = self._load_tokenizer(vocab_path)
        self._label_mapping = label_mapping

    def extract(self, text: str) -> list[dict]:
        # 1. Tokenize
        input_ids, attention_mask = self._tokenizer.encode(text)

        # 2. ONNX 推理（CPU 单核 ~2ms/样本）
        outputs = self._session.run(
            None,
            {"input_ids": input_ids, "attention_mask": attention_mask}
        )

        # 3. 解码实体标签
        return self._decode_entities(text, outputs[0])
```

**ONNX vs PyTorch 性能对比**：

| 指标 | PyTorch | ONNX Runtime | 提升 |
|---|---|---|---|
| 内存占用 | ~1.5GB | < 200MB | 7.5x |
| CPU 推理延迟 | ~10ms | ~2ms | 5x |
| 启动时间 | ~3s | ~0.5s | 6x |
| 依赖包大小 | ~2GB | ~50MB | 40x |

### 5.3 Apple Silicon MLX 后端

MLX 是 Apple 为 Apple Silicon 优化的机器学习框架，利用统一内存架构（Unified Memory）实现 CPU/GPU 零拷贝推理：

```python
class MlxLlmEngine:
    """Apple Silicon MLX 加速的 LLM 分类器。"""

    def __init__(self, model_path: str):
        import mlx.core as mx
        from mlx_lm import load, generate

        self._model, self._tokenizer = load(model_path)
        self._mx = mx  # 保存 mlx 模块引用

    def classify(self, text: str) -> dict | None:
        # MLX 利用 Apple Silicon 统一内存，无需 GPU 显存拷贝
        prompt = self._build_prompt(text)
        response = generate(self._model, self._tokenizer, prompt=prompt, max_tokens=256)
        return self._parse_response(response)
```

---

## 6. 进程级并发控制与内存防 OOM 熔断 / Process-Wide Concurrency & Memory Safety

### 6.1 问题背景

即使单个 LLM 分类器内部已串行化推理，一个进程中可能存在**多个模型实例**（主分类器 + 多个命名空间/域的服务实例）。并发推理叠加会推高内存，最终触发操作系统 OOM Killer，导致 gRPC 连接被重置（`connection reset by peer`）。

### 6.2 双重安全护栏实现

```python
# 护栏 1：进程级全局推理信号量（所有 LlmAdapter 实例共享）
_LLM_INFER_SEMAPHORE = threading.Semaphore(
    max(1, _env_int("PRIVACY_LLM_MAX_CONCURRENCY", _get_default_llm_concurrency()))
)
# 默认并发数根据 Provider 自动调整：
# - 远程 vLLM/OpenAI: 默认 16（无本地内存压力）
# - 本地 PyTorch/MLX: 默认 1（串行化防 OOM）

# 护栏 2：系统物理可用内存熔断
_LLM_MIN_FREE_MEM_MB = _env_float("PRIVACY_LLM_MIN_FREE_MEM_MB", 512.0)

def _check_system_memory_safe() -> bool:
    """检查宿主机当前可用物理内存是否高于安全红线。"""
    try:
        import psutil
        free_mb = psutil.virtual_memory().available / (1024 * 1024)
        if free_mb < _LLM_MIN_FREE_MEM_MB:
            logger.warning(f"Memory {free_mb:.1f}MB < threshold {_LLM_MIN_FREE_MEM_MB}MB. Skipping LLM.")
            return False
        return True
    except ImportError:
        return True  # psutil 未安装，乐观放行

def safe_llm_inference(predict_fn):
    """受并发信号量与内存安全保护的 LLM 推理包装器。"""
    if not _check_system_memory_safe():
        return None  # 内存不足，优雅降级

    acquired = _LLM_INFER_SEMAPHORE.acquire(timeout=_LLM_SEMAPHORE_WAIT_SECONDS)
    if not acquired:
        logger.warning(f"LLM semaphore timeout after {_LLM_SEMAPHORE_WAIT_SECONDS}s. Degrading.")
        return None
    try:
        return predict_fn()
    finally:
        _LLM_INFER_SEMAPHORE.release()
```

### 6.3 环境变量配置速查

| 变量 | 默认值 | 用途 |
|---|---|---|
| `PRIVACY_LLM_MAX_CONCURRENCY` | 自动 (1/16) | 进程级 LLM 推理并发上限 |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | 30.0 | 信号量排队等待超时 |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | 512.0 | 可用内存低于此值时跳过 LLM |
| `PRIVACY_LLM_PROVIDER` | `qwen3` | LLM 后端提供方 |
| `PRIVACY_LLM_CONFIDENCE_THRESHOLD` | 0.75 | Layer-3 仲裁最低置信度 |
| `PRIVACY_LLM_ENABLE_ARBITRATION` | `true` | 是否启用 LLM 仲裁 |
| `PRIVACY_NER_ENABLE` | `false` | 是否启用 Layer-2 NER |
| `PRIVACY_LLM_ENABLE` | `false` | 显式启用 Layer-3 LLM |
| `PRIVACY_LLM_AUTO_ON_IMAGE` | `true` | 检测到图像输入时自动触发 LLM |

---

## 7. 模型下载与离线分发 / Model Download & Offline Distribution

### 7.1 一键下载脚本

```bash
# 下载 LLM 模型（Qwen3.5）
python -m engine.privacy.download_model
# 下载 NER 模型（StructBERT ONNX）
python -m engine.privacy.download_ner_model
```

下载流程包含 SHA-256 完整性校验，确保模型文件未被篡改或损坏：

```python
def download_and_verify(url: str, target_path: Path, expected_sha256: str) -> None:
    """下载模型文件并校验 SHA-256。"""
    # 1. 下载到临时文件
    tmp_path = target_path.with_suffix(".tmp")
    urllib.request.urlretrieve(url, tmp_path)

    # 2. 计算 SHA-256
    actual_sha256 = hashlib.sha256(tmp_path.read_bytes()).hexdigest()
    if actual_sha256 != expected_sha256:
        tmp_path.unlink()
        raise RuntimeError(f"SHA-256 mismatch: expected {expected_sha256}, got {actual_sha256}")

    # 3. 原子替换（tmp → 最终路径）
    tmp_path.rename(target_path)
```

### 7.2 离线部署方案

在无外网的隔离环境中，可以预先下载模型文件并拷贝到目标机器：

```bash
# 在有网机器上下载
python -m engine.privacy.download_model
python -m engine.privacy.download_ner_model

# 打包模型目录
tar czf privshield-models.tar.gz .models/

# 拷贝到隔离环境并解压
tar xzf privshield-models.tar.gz -C /path/to/privshield/
```

---

## 8. CI 测试 Mock 策略 / CI Testing Mock Strategy

### 8.1 Mock 层级与位置

```text
测试代码 (test_funnel.py)
    │
    │  patch("engine.dynclassification.llm_adapter.LlmAdapter._lazy_init")
    │  patch("engine.dynclassification.ner_adapter.NerAdapter._lazy_init")
    │
    ▼
适配器层 (LlmAdapter / NerAdapter)
    │
    │  返回 Mock 的 classify() / extract() 结果
    │
    ▼
引擎层 (Qwen3Classifier / ONNXSmallNerEngine)
    │
    │  完全跳过，不加载任何模型
    │
    ▼
运行时 (PyTorch / ONNX Runtime)
```

### 8.2 Mock 示例

```python
from unittest.mock import MagicMock, patch

def test_funnel_with_mocked_llm():
    """测试三层漏斗在规则冲突时调度 LLM 仲裁的决策逻辑。"""
    with patch("engine.dynclassification.llm_adapter.LlmAdapter.classify") as mock:
        mock.return_value = {
            "final_level": "S3",
            "confidence": 0.95,
            "reasoning": "Mocked: context indicates high sensitivity",
        }

        funnel = ClassificationFunnel(...)
        result = funnel.classify_field("medical_desc", "患者确诊急性感染")

        assert result.final_level == "S3"
        assert result.engine_layer == "L3_LLM"
        mock.assert_called_once()
```

---

## 9. Docker 镜像分层策略 / Docker Image Layering

PrivShield 使用单 Dockerfile 多阶段构建，通过 `--target` 参数选择不同镜像：

```dockerfile
# Stage 1: Core 基础层
FROM python:3.13-slim AS core
COPY requirements-core.txt .
RUN pip install -r requirements-core.txt
# Core 镜像仅包含 FastAPI/gRPC/Pydantic 等轻量依赖
# 镜像大小: ~100MB

# Stage 2: ML 扩展层
FROM core AS ml
COPY requirements-ml.txt .
RUN pip install -r requirements-ml.txt
# ML 镜像额外包含 torch/transformers/onnxruntime
# 镜像大小: ~3GB
```

```bash
# 构建 Core 镜像（推荐，无 ML 依赖）
docker build --target core -t privshield:1.8.0 .

# 构建 ML 镜像（含完整推理能力）
docker build --target ml -t privshield:1.8.0-ml .
```

---

## 10. 性能基准与调优指南 / Performance Benchmarks & Tuning

| 操作 | 硬件 | 延迟 | 内存 | 吞吐量 |
|---|---|---|---|---|
| Rule Engine (Layer-1) | 任意 CPU | < 0.1ms | ~10MB | > 10,000 ops/s |
| ONNX NER (Layer-2) | CPU 单核 | ~2ms | ~200MB | ~500 ops/s |
| PyTorch LLM (Layer-3) | CPU | 5~30s | ~1.5GB | ~0.03 ops/s |
| PyTorch LLM (Layer-3) | GPU (A100) | 1~3s | ~8GB | ~0.3 ops/s |
| MLX LLM (Layer-3) | Apple M2 | 2~5s | ~4GB | ~0.2 ops/s |
| vLLM Remote (Layer-3) | 远程集群 | 0.5~2s | 0 (本地) | ~16 concurrent |

**调优建议**：

1. **Core 镜像 + 远程 vLLM**：生产环境推荐方案，本地镜像仅 100MB，推理卸载到 GPU 集群
2. **ML 镜像 + 本地推理**：边缘部署/无 GPU 集群场景，使用 ONNX NER + 本地 LLM
3. **Apple Silicon 开发机**：使用 MLX 后端获得最佳本地推理体验
4. **内存受限环境**：设置 `PRIVACY_LLM_MIN_FREE_MEM_MB=1024` 提高安全阈值

---

## 11. NER 适配器实现细节 / NER Adapter Implementation

文件 / File：[`engine/dynclassification/ner_adapter.py`](engine/dynclassification/ner_adapter.py)

### 11.1 引擎优先级与自动降级

NerAdapter 采用与 LlmAdapter 相同的懒加载策略，但在引擎选择上有自己的优先级链：

```python
class NerAdapter:
    def _lazy_init(self) -> None:
        """延迟初始化 NER 引擎（ONNX 优先，ModelScope 回退）。"""
        if self._initialized:
            return
        with self._init_lock:
            if self._initialized:
                return
            self._initialized = True

            # 优先级 1: ONNX 轻量引擎（推荐，内存占用 < 200MB）
            try:
                from .ner_engines import ONNXSmallNerEngine
                self._engine = ONNXSmallNerEngine(
                    model_path=self._model_path,
                    vocab_path=self._vocab_path,
                    label_mapping=self._label_mapping,
                )
                logger.info("NER engine initialized: ONNXSmallNerEngine")
                return
            except Exception as e:
                logger.debug(f"ONNX NER engine unavailable: {e}")

            # 优先级 2: ModelScope 引擎（较重，作为回退）
            try:
                from .ner_engines import ModelScopeSmallNerEngine
                self._engine = ModelScopeSmallNerEngine(...)
                logger.info("NER engine initialized: ModelScopeSmallNerEngine")
                return
            except Exception as e:
                logger.debug(f"ModelScope NER engine unavailable: {e}")

            # 全部失败：标记不可用，后续 extract() 直接返回空列表
            self._available = False
            logger.warning("NER engine unavailable. Layer-2 will be skipped.")
```

### 11.2 安全降级策略

| 场景 | 行为 | 影响 |
|---|---|---|
| onnxruntime 未安装 | 尝试 ModelScope | 无影响 |
| modelscope 未安装 | 标记不可用 | Layer-2 跳过，直接到 Layer-3 |
| 模型文件不存在 | 标记不可用 | Layer-2 跳过 |
| 推理异常 | 返回空列表 | 不影响上层流程 |

---

## 12. LLM 仲裁流程详解 / LLM Arbitration Flow

文件 / File：[`engine/dynclassification/llm_adapter.py`](engine/dynclassification/llm_adapter.py)

### 12.1 仲裁触发条件

LLM 仲裁不是每次请求都触发，而是仅在以下条件下激活：

```text
Layer-1 规则引擎评估
    │
    ├─ 置信度 ≥ 0.75 → 直接返回规则结果（不调用 LLM）
    │
    ├─ 置信度 < 0.75 → 触发 LLM 仲裁
    │
    ├─ 规则冲突（普通规则 S3 vs 降级规则 S2）→ 触发 LLM 仲裁
    │
    └─ 不确定标签 → 触发 LLM 仲裁
```

### 12.2 仲裁 Prompt 构建

```python
def arbitrate(self, field_name, value, conflict_tags, taxonomy):
    """构建仲裁专用 prompt 并调用 LLM 分类。"""
    prompt = f"""你是一个数据安全分类专家。请判断以下字段的敏感度等级。

字段名: {field_name}
字段值: {value[:500]}  # 截断超长内容

已知标签冲突: {conflict_tags}
分类体系: {taxonomy.to_prompt_string()}

请返回 JSON 格式:
{{"final_level": "S?", "confidence": 0.?, "reasoning": "..."}}
"""
    result = self.classify(prompt)
    if result and result.get("confidence", 0) >= self._confidence_threshold:
        return result
    return None  # 置信度不足，返回 None 让上层使用规则结果
```

### 12.3 推理超时保护

LLM 推理可能因模型加载或输入过长而耗时过久。PrivShield 设置了 180 秒的推理超时：

```python
import signal

def _classify_with_timeout(self, text: str, timeout: float = 180.0):
    """带超时的 LLM 分类调用。"""
    result = [None]
    exception = [None]

    def _worker():
        try:
            result[0] = self._classifier.classify(text)
        except Exception as e:
            exception[0] = e

    thread = threading.Thread(target=_worker, daemon=True)
    thread.start()
    thread.join(timeout=timeout)

    if thread.is_alive():
        logger.warning(f"LLM inference timed out after {timeout}s")
        return None  # 超时降级
    if exception[0]:
        raise exception[0]
    return result[0]
```

---

## 13. 环境 Profile 配置体系 / Environment Profile Configuration

PrivShield 使用 `PRIVACY_ENV_PROFILE` 环境变量加载不同的 LLM 后端配置：

```bash
# 加载 vLLM 配置
export PRIVACY_ENV_PROFILE=vllm
# 实际加载: config/env/vllm.env
```

各 Profile 配置文件示例：

```bash
# config/env/vllm.env
PRIVACY_LLM_PROVIDER=vllm
PRIVACY_VLLM_URL=http://vllm-server:8000/v1
PRIVACY_VLLM_API_KEY=sk-xxx
PRIVACY_LLM_MAX_CONCURRENCY=16

# config/env/qwen3.env
PRIVACY_LLM_PROVIDER=qwen3
PRIVACY_LLM_MODEL_PATH=.models/qwen3-0.8b
PRIVACY_LLM_MAX_CONCURRENCY=1

# config/env/mlx.env
PRIVACY_LLM_PROVIDER=mlx
PRIVACY_MLX_MODEL_PATH=.models/qwen3-mlx
PRIVACY_LLM_MAX_CONCURRENCY=1

# config/env/openai.env
PRIVACY_LLM_PROVIDER=openai
PRIVACY_OPENAI_API_KEY=sk-xxx
PRIVACY_OPENAI_MODEL=gpt-4o-mini
PRIVACY_LLM_MAX_CONCURRENCY=16
```

---

## 14. 启动时异步预热 / Async Warmup on Startup

为避免首个请求承担模型加载的冷启动延迟，PrivShield 支持在 REST 服务启动后异步预热 LLM：

```bash
# 启用启动时 LLM 预热
export PRIVACY_WARMUP_LLM=true
python -m engine.server
```

预热流程：
1. FastAPI 服务启动完成后，在后台线程中触发 `LlmAdapter._lazy_init()`
2. 模型加载完成前，服务已可接受请求（Layer-1/2 正常工作）
3. 预热完成后，Layer-3 请求不再承受冷启动延迟

---

## 15. 扩展阅读 / Further Reading

1. **ONNX Runtime 文档**：https://onnxruntime.ai/docs/
2. **Apple MLX 框架**：https://ml-explore.github.io/mlx/
3. **LoRA 论文**：Hu et al., "LoRA: Low-Rank Adaptation of Large Language Models" (2021)
4. **vLLM 部署**：https://docs.vllm.ai/en/latest/
5. **HuggingFace Transformers**：https://huggingface.co/docs/transformers/
