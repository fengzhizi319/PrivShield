# Qwen2.5-0.5B LoRA 微调、导出与评估指南

> 本指南针对在 `privacy-local-agent` Sidecar 架构中针对 **Qwen2.5-0.5B-Instruct** 进行 LoRA 微调、模型量化导出、与 `LlmAdapter` 集成、灰度部署与自动回滚、以及 Benchmark 验证进行详细说明。
>
> **本方案仅面向纯文本分类分级与脱敏场景，不涉及图片 OCR。**

---

## 1. 环境准备与训练配置

推荐使用 **LLaMA-Factory** 或 **Unsloth** 进行轻量级高效 LoRA 微调。由于 Qwen2.5-0.5B 模型参数量极小，微调单卡 RTX 3060 / 4060 (8GB 显存) 即可在 15~30 分钟内完成 50k 样本的训练。

### 1.1 依赖安装

```bash
pip install llamafactory peft transformers datasets trl torch accelerate
# 可选：Unsloth 加速训练（比标准 PEFT 快 2~5 倍）
pip install unsloth
```

### 1.2 LoRA 推荐超参数设置

```yaml
# dataset_info.json 配置
dataset_name: privacy_sft_50k
formatting: sharegpt  # 使用 ShareGPT 格式（system/user/assistant 三轮）
columns:
  messages: conversations

# 模型配置
model_name_or_path: Qwen/Qwen2.5-0.5B-Instruct
stage: sft
do_train: true

# LoRA 参数配置
finetuning_type: lora
lora_rank: 16              # 秩：16 足以拟合领域任务，过高易过拟合
lora_alpha: 32             # alpha = 2 * rank（常用比例）
lora_dropout: 0.05         # 轻度 dropout 防止过拟合
lora_target: q_proj,v_proj,k_proj,o_proj,gate_proj,up_proj,down_proj  # 全量线性层

# 训练超参数
learning_rate: 2.0e-4      # 0.5B 模型适用学习率（1e-4 ~ 5e-4）
num_train_epochs: 3.0       # 50k 样本 3 epoch 足够收敛
per_device_train_batch_size: 8
gradient_accumulation_steps: 4  # 有效 batch = 8 * 4 = 32
lr_scheduler_type: cosine
warmup_ratio: 0.05
fp16: true                  # 若显卡支持可启用 bf16: true
max_length: 512             # 输入截断长度（覆盖绝大多数隐私文本场景）

# 输出路径
output_dir: ./saves/Qwen2.5-0.5B-Privacy-LoRA
logging_steps: 10
save_steps: 500
eval_strategy: steps
eval_steps: 500
load_best_model_at_end: true
metric_for_best_model: eval_loss
```

### 1.3 训练命令

```bash
# 使用 LLaMA-Factory 启动训练
llamafactory-cli train \
    --config ./configs/privacy_lora_qwen05b.yaml

# 或使用 Unsloth 加速（推荐，速度提升 2~5x）
python scripts/train_with_unsloth.py \
    --model Qwen/Qwen2.5-0.5B-Instruct \
    --data ./data/llm_lora/train.jsonl \
    --output ./saves/Qwen2.5-0.5B-Privacy-LoRA
```

### 1.4 架构选择路线：在原版上微调 vs. 在 `nerlora` 上二次微调

在工程选型中，面对已有完成 NER 微调的模型 `nerlora`，建议采取以下策略：

| 维度 | 方案 A：在原版 Base/Instruct 上微调 | 方案 B：在 `nerlora` 上继续微调 (推荐) |
|---|---|---|
| **实体边界敏感度** | 依赖全量分类数据重头学习实体定位 | **继承 NER 先验**，已知哪些 Token 是敏感词 |
| **脱敏抹平能力** | 需同时学习实体定位 + 密级分类 + 重写平滑 | **效果更好**：定位准确度更高，脱敏替换无死角 |
| **训练收敛速度** | 需较多 Epoch 才能收敛 | **收敛极快**（实体特征已表示在 Embedding/Attention 中） |
| **遗忘风险** | 无遗忘风险 | 若不包含纯 NER 数据，第二阶段可能发生 NER 抽框能力衰减 |
| **推荐策略** | 仅做纯分类/重写不需实体输出时可选 | **强烈推荐**：合并 `nerlora` 为 Checkpoint 后，混合 20% NER 数据做增量 SFT |

---

## 2. LoRA 权重合并与轻量化量化导出

为了在 Sidecar 中达到最佳性能（极低内存占用与高并发），训练完成后需将 LoRA 权重合并到主干并导出为 INT4 / GGUF 量化格式：

### 2.1 权重合并 (Merge LoRA Weights)

```bash
llamafactory-cli export \
    --model_name_or_path Qwen/Qwen2.5-0.5B-Instruct \
    --adapter_name_or_path ./saves/Qwen2.5-0.5B-Privacy-LoRA \
    --template qwen \
    --export_dir .models/Qwen2.5-0.5B-Privacy-Merged \
    --export_size 2 \
    --export_device cpu
```

### 2.2 量化导出 (GGUF / INT4 / ONNX)

| 量化格式 | 模型体积 | CPU 推理延迟 | GPU 显存占用 | 适用场景 |
|---|---|---|---|---|
| **GGUF Q4_K_M** | < 400 MB | 20 ~ 50 ms | N/A (纯 CPU) | 边缘设备/无 GPU 的 Sidecar |
| **AutoAWQ INT4** | < 700 MB | N/A | < 700 MB | 有 GPU 的高吞吐 Sidecar |
| **ONNX (fp16)** | ~1 GB | 30 ~ 80 ms | < 1 GB | 跨平台部署（Windows/Linux） |

```bash
# GGUF 量化（使用 llama.cpp）
cd /path/to/llama.cpp
python convert_hf_to_gguf.py .models/Qwen2.5-0.5B-Privacy-Merged \
    --outfile .models/Qwen2.5-0.5B-Privacy-Q4_K_M.gguf \
    --outtype q4_k_m

# AutoAWQ INT4 量化
python -m awq.quantize \
    --model_path .models/Qwen2.5-0.5B-Privacy-Merged \
    --quant_config configs/awq_int4.json \
    --output_path .models/Qwen2.5-0.5B-Privacy-AWQ
```

---

## 3. 集成至 privacy-local-agent

### 3.1 新增 `QwenPrivacyLoRAEngine` 类

在 `privacy_local_agent/dynclassification/llm_engines.py` 中新增微调小模型推理引擎，**直接替换**原有 `Qwen2VLClassifier` 作为 Layer-3 纯文本推理引擎：

```python
class QwenPrivacyLoRAEngine(LlmClassifier):
    """基于微调 Qwen2.5-0.5B-Privacy 的纯文本分类/脱敏推理引擎。
    
    定位：
    - 直接替换 Qwen2VLClassifier 作为 Layer-3 LLM 引擎
    - 模型体积 < 1.5GB，可常驻内存
    - 推理延迟 50~150ms（CPU），适合高并发 Sidecar
    - 输出同时包含分类分级结果 + 无痕抹平文本
    """

    def __init__(self, model_path: str, device: str = "auto"):
        from transformers import AutoModelForCausalLM, AutoTokenizer
        
        self._model_path = model_path
        self._device = device
        
        # 延迟加载：首次 classify() 时才初始化
        self._tokenizer = None
        self._model = None
        self._initialized = False

    def _lazy_init(self):
        if self._initialized:
            return
        try:
            self._tokenizer = AutoTokenizer.from_pretrained(
                self._model_path, trust_remote_code=True
            )
            self._model = AutoModelForCausalLM.from_pretrained(
                self._model_path,
                device_map=self._device,
                torch_dtype="auto",
                trust_remote_code=True,
            )
            self._model.eval()
            self._initialized = True
            logger.info("qwen_privacy_lora_loaded", extra={"path": self._model_path})
        except Exception as e:
            logger.error("qwen_privacy_lora_load_failed", extra={"error": str(e)})
            raise

    def classify(self, text: str, **kwargs) -> dict[str, Any] | None:
        """执行分类分级 + 无痕抹平推理。"""
        self._lazy_init()
        
        prompt = self._build_prompt(text)
        inputs = self._tokenizer(prompt, return_tensors="pt").to(self._model.device)
        
        with torch.no_grad():
            outputs = self._model.generate(
                **inputs,
                max_new_tokens=256,
                temperature=0.1,      # 低温度保证输出确定性
                do_sample=False,       # 贪心解码
                pad_token_id=self._tokenizer.eos_token_id,
            )
        
        response = self._tokenizer.decode(
            outputs[0][inputs.input_ids.shape[1]:],
            skip_special_tokens=True,
        )
        
        return self._parse_response(response)

    def _build_prompt(self, text: str) -> str:
        """构建 ChatML 格式 Prompt。"""
        return (
            "<|im_start|>system\n"
            "你是一个专业的隐私安全Sidecar助手。请分析输入的文本，识别敏感信息，"
            "输出分类分级结果（JSON格式），并提供语义连贯的无痕抹平脱敏重写文本。\n"
            "<|im_end|>\n"
            f"<|im_start|>user\n{text}<|im_end|>\n"
            "<|im_start|>assistant\n"
        )

    def _parse_response(self, response: str) -> dict[str, Any] | None:
        """解析模型输出为结构化字典。"""
        import re
        import json
        
        # 提取 JSON 块（支持 ```json ... ``` 包裹）
        json_match = re.search(r"```json\s*(.*?)\s*```", response, re.DOTALL)
        if json_match:
            json_str = json_match.group(1)
        else:
            json_str = response.strip()
        
        try:
            result = json.loads(json_str)
            # 校验必要字段
            assert "classification" in result
            assert "max_level" in result["classification"]
            assert "smoothed_text" in result
            return result
        except (json.JSONDecodeError, AssertionError) as e:
            logger.warning("lora_parse_failed", extra={"error": str(e), "raw": response[:200]})
            return None  # 解析失败 → 上层降级到规则引擎结果
```

### 3.2 修改 `LlmAdapter` 优先加载微调引擎

在 `privacy_local_agent/dynclassification/llm_adapter.py` 中，修改 `_lazy_init()` 使其优先加载 `QwenPrivacyLoRAEngine`：

```python
def _lazy_init(self):
    """优先加载微调 LoRA 引擎，不存在时回退到原有引擎。"""
    if self._initialized:
        return
    
    text_engine = os.getenv("PRIVACY_LLM_TEXT_ENGINE", "auto")
    lora_path = os.getenv("PRIVACY_LLM_LORA_MODEL_PATH", "")
    
    # 1. 优先尝试加载微调 LoRA 引擎
    if text_engine in ("auto", "qwen_lora"):
        auto_path = lora_path or self._auto_detect_lora_path()
        if auto_path and os.path.isdir(auto_path):
            try:
                from .llm_engines import QwenPrivacyLoRAEngine
                self._classifier = QwenPrivacyLoRAEngine(
                    model_path=auto_path, device=self._device
                )
                logger.info("llm_adapter_initialized", extra={"backend": "qwen_lora"})
                self._initialized = True
                self._available = True
                return
            except Exception as e:
                logger.warning("lora_engine_load_failed", extra={"error": str(e)})
    
    # 2. 回退到原有 Qwen2VL 引擎
    if text_engine in ("auto", "qwen2vl"):
        try:
            from .llm_engines import Qwen2VLClassifier
            self._classifier = Qwen2VLClassifier(
                model_path=self._model_path,
                classify_prompt_template=self._classify_prompt_template,
                device=self._device,
            )
            logger.info("llm_adapter_initialized", extra={"backend": "qwen2vl"})
        except Exception as e:
            self._available = False
            logger.warning("fallback_engine_load_failed", extra={"error": str(e)})
    
    self._initialized = True
```

---

## 4. 灰度部署与自动回滚策略

### 4.1 灰度部署流程

```text
┌─────────────────────────────────────────────────────────────────────┐
│                    灰度部署三阶段                                     │
└─────────────────────────────────────────────────────────────────────┘

Phase 1: Shadow Mode (影子模式)
  - 微调模型与原有 Layer-1+Layer-2 路径并行运行
  - 仅记录结果，不实际返回给用户
  - 对比两者输出差异，统计准确率/延迟
  - 持续时间：24~72 小时

Phase 2: Canary Release (金丝雀发布)
  - 10% 流量路由到微调模型
  - 监控关键指标：错误率、延迟 P99、分类准确率
  - 若指标异常 → 自动回滚到 Phase 1 或完全回退到 Layer-1+Layer-2 路径

Phase 3: Full Rollout (全量发布)
  - 100% 请求路由到微调 LoRA 模型
  - 持续监控 Prometheus 指标
```

### 4.2 自动回滚触发条件

| 指标 | 阈值 | 触发条件 |
|---|---|---|
| JSON 解析失败率 | > 5% | 连续 100 次请求中失败超过 5 次 |
| 推理延迟 P99 | > 500ms | 5 分钟滑动窗口内 P99 超过阈值 |
| 分类准确率下降 | > 10% | 与 Shadow Mode 基线对比下降超过 10% |
| 零泄漏校验失败 | > 1% | 抹平文本再次扫描发现敏感信息 |

### 4.3 回滚实现

```python
# 在 LlmAdapter.classify() 中实现动态回滚
def classify(self, text: str, **kwargs) -> dict | None:
    # 检查是否需要回滚
    if self._should_rollback():
        logger.warning("lora_engine_rollback", extra={"reason": "metrics_degraded"})
        self._classifier = None       # 禁用 LoRA 引擎
        self._available = False
        return None                    # 返回 None → 漏斗降级到 Layer-1+Layer-2 结果
    
    # 正常路由到 LoRA 引擎
    if self._classifier:
        result = self._classifier.classify(text, **kwargs)
        if result is None:
            # 解析失败 → 降级到规则引擎结果
            return None
        return result
    
    return None
```

---

## 5. 效果评估与 Benchmark 验证指标

在 `tests/benchmark_llmlora.py` 中自动化检验微调后的模型效果，核心指标需达到以下标准：

| 评估指标 | 目标基线 (Baseline) | 未微调 0.5B 基座 | 微调后 Qwen2.5-0.5B LoRA | 测量方法 |
|---|---|---|---|---|
| **JSON 格式合法解析率** | 99.0% | 65% ~ 75% | **99.5%+** | 1000 次推理中 JSON 解析成功次数 |
| **密级识别准确率 (L1~L5)** | 88.0% | 68% ~ 75% | **93% ~ 96%** | 与 Layer-1 Ground Truth 对比 |
| **PII 实体 Recall / Precision** | 86% / 90% | 62% / 70% | **92% / 95%** | 在 test.jsonl 上计算 F1 |
| **脱敏无痕抹平自然度 (BLEU/ROUGE)** | 0.72 | 0.55 ~ 0.60 | **0.85+** | 与人工标注参考文本对比 |
| **二次扫描零泄漏率 (Zero-Leakage)** | 98.5% | 80% ~ 85% | **99.5%+** | 抹平文本再次过规则引擎 |
| **单次推理延迟 (CPU)** | 650 ms | 80 ~ 120 ms | **70 ~ 100 ms** | 100 次推理 P50 |
| **内存 / 显存峰值占用** | 4.2 GB | 0.8 ~ 1.2 GB | **0.8 ~ 1.2 GB** | psutil 监控峰值 RSS |

### 5.1 Benchmark 运行命令

```bash
# 运行完整 Benchmark 套件
PYTHONPATH=. python tests/benchmark_llmlora.py \
    --model-path .models/Qwen2.5-0.5B-Privacy-Merged \
    --test-data data/llm_lora/test.jsonl \
    --output results/benchmark_report.json

# 对比测试（微调 LoRA vs 未微调基座）
PYTHONPATH=. python tests/benchmark_llmlora.py \
    --compare \
    --lora-path .models/Qwen2.5-0.5B-Privacy-Merged \
    --base-path Qwen/Qwen2.5-0.5B-Instruct \
    --test-data data/llm_lora/test.jsonl
```

### 5.2 关键评估维度说明

| 维度 | 测试集构成 | 评估重点 |
|---|---|---|
| **结构化字段** | 身份证、手机号、银行卡号等 | 正则可捕获的确定性实体 |
| **隐式 PII** | "他的病情很严重"中的健康暗示 | 语义理解能力 |
| **多实体混合** | 单句含 3~5 个不同类型实体 | 多实体同时识别的完整性 |
| **长文本** | 500+ 字病历/合同 | 长上下文处理能力 |
| **负样本** | 无敏感信息的普通文本 | 防止过度脱敏 |
| **对抗样本** | 全角字符、错别字、空格干扰 | 鲁棒性 |

---

## 6. 常见问题与排查

### Q1: 微调后模型输出格式不稳定怎么办？

- **检查训练数据**：确保 100% 的 assistant 输出是合法 JSON
- **降低 temperature**：推理时使用 `temperature=0.1` 或 `do_sample=False`
- **添加 JSON Schema 约束**：使用 `outlines` 或 `jsonformer` 库强制输出符合 schema

### Q2: 无痕抹平文本语义不连贯怎么办？

- **检查训练数据质量**：确保 `smoothed_text` 字段是自然流畅的中文
- **增加领域重写模板**：在 `CONTEXT_REWRITE_TEMPLATES` 中补充更多自然表达
- **使用 ROUGE-L 过滤**：训练前过滤掉 ROUGE-L < 0.6 的低质量样本

### Q3: 如何在无 GPU 的边缘设备上部署？

- 使用 **GGUF Q4_K_M** 量化格式
- 配合 `llama-cpp-python` 进行纯 CPU 推理
- 预期延迟：50~100ms（4 核 CPU），内存占用 < 500MB

```python
# llama-cpp-python 加载示例
from llama_cpp import Llama
llm = Llama(
    model_path=".models/Qwen2.5-0.5B-Privacy-Q4_K_M.gguf",
    n_ctx=512,
    n_threads=4,  # CPU 线程数
)
output = llm(prompt, max_tokens=256, temperature=0.1)
```

### Q4: 在 `nerlora` 上二次微调后 NER 能力衰减怎么办？

- **混合 NER 数据**：训练集中保留 20% 纯 NER 标注数据（实体定位 + 类型标注），防止遗忘
- **EWC 正则化**：使用 Elastic Weight Consolidation 约束关键参数不偏离原始 NER 权重
- **早停策略**：监控 NER 验证集 F1，当 F1 下降超过 3% 时触发早停
