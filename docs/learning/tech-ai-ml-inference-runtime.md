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

文件 / File：[`engine/dynclassification/ner_adapter.py`](file:///home/charles/code/PrivShield/engine/dynclassification/ner_adapter.py) & [`engine/dynclassification/llm_adapter.py`](file:///home/charles/code/PrivShield/engine/dynclassification/llm_adapter.py)

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

文件 / File：[`engine/dynclassification/llm_adapter.py`](file:///home/charles/code/PrivShield/engine/dynclassification/llm_adapter.py#L50-L150)

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

文件 / File：[`engine/dynclassification/ner_engines.py`](file:///home/charles/code/PrivShield/engine/dynclassification/ner_engines.py)

针对 Layer 2 Small-NER，`PrivShield` 将 ModelScope 开源的 StructBERT 中文实体识别模型导出为标准 **ONNX** 格式：
- 使用 `onnxruntime.InferenceSession` 替代庞大的 PyTorch 运行时；
- 内存开销从 1.5GB 骤降至 < 200MB；
- 推理速度相比原始 PyTorch 提升 3~5 倍，在 CPU 单核上可达 2ms / 样本。

---

### 2.4 LoRA 轻量微调与 PEFT 训练管线 / LoRA Fine-Tuning Pipeline

文档参考 / Docs：[`docs/llmlora/`](file:///home/charles/code/PrivShield/docs/llmlora/)

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
