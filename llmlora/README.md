ba'sh# llmlora — Qwen3.5 基座 LoRA 专精微调（纯文本隐私分类分级与无痕抹平）

基于 `basemodels/cmeee_merged`（Qwen3.5 0.8B CMeEE NER 合并基座，约 752M 参数）做二阶段 LoRA SFT，
专门面向**纯文本**场景（不考虑图片 OCR）：

- **分类分级仲裁**：L1~L5 密级裁定 + 实体识别（Ground Truth 来自项目 Layer-1 规则引擎）
- **无痕抹平脱敏**：上下文自然重写（Natural Context Rewriting），零泄漏 QA 保证

完整设计方案见 [docs/design_and_workflow.md](docs/design_and_workflow.md)。

---

## 1. 环境要求（重要）

| 依赖 | 要求 | 原因 |
|---|---|---|
| transformers | **>= 5.2**（已锁定 5.14.1） | Qwen3.5 架构 `qwen3_5_text` 在 transformers 4.x 无法加载 |
| torch | 继承系统环境（支持 CUDA） | venv 以 `--system-site-packages` 创建 |
| peft / accelerate / datasets / faker | venv 内安装 | LoRA 注入与数据蒸馏 |

训练环境独立于主项目，位于 `llmlora/.venv`，首次使用先执行 `setup_env.sh`。

---

## 2. 目录结构

```text
llmlora/
├── .venv/                        # 独立训练环境（transformers 5.x）
├── basemodels/
│   └── cmeee_merged/             # Qwen3.5 0.8B CMeEE 合并基座
├── data/                         # train.jsonl / dev.jsonl / test.jsonl
├── docs/
│   └── design_and_workflow.md    # 完整设计方案与工作流
├── output/
│   ├── saves/qwen35-cmeee-privacy-lora/     # LoRA adapter 权重
│   └── models/Qwen3.5-0.8B-Privacy-Classifier-Smoother/  # 合并导出端到端模型
├── scripts/                      # 常用命令 sh 脚本 + Python 入口
└── src/
    ├── dataset/                  # loader.py (Labels Masking) / data_collator.py
    ├── inference/
    │   ├── engine.py             # QwenPrivacyLoRAEngine (PyTorch 原生推理)
    │   └── engine_vllm.py        # QwenPrivacyVLLMEngine (vLLM 高性能推理)
    ├── models/                   # trainer.py (LoRATrainingRunner)
    └── utils/                    # config.py / logger.py / metrics.py
```

---

## 3. 常用命令脚本（`scripts/`）

所有脚本自动定位仓库根目录、校验独立 venv，可在任意目录执行；额外参数原样透传给对应 Python 入口。

| 脚本 | 用途 |
|---|---|
| `scripts/setup_env.sh` | 创建/更新独立训练环境（transformers 5.14.1 等依赖）并校验版本 |
| `scripts/generate_data.sh` | 生成训练/验证/测试数据（规则引擎打标 + 零泄漏 QA） |
| `scripts/train.sh` | LoRA 训练一键启动（默认训练完自动合并导出并同步复制至 `.models/Qwen3.5-0.8B-Privacy-Classifier-Smoother`） |
| `scripts/evaluate.sh` | Benchmark 评估（JSON 合法率/密级 Acc/实体 F1/零泄漏/延迟） |
| `scripts/smoke_test.sh` | 端到端冒烟：小数据生成 → 10 步训练 + 合并 → 快速评估 |

### 3.1 首次使用：搭建环境

```bash
./llmlora/scripts/setup_env.sh
# 指定解释器：PYTHON_BIN=/home/charles/miniconda3/envs/pri/bin/python ./llmlora/scripts/setup_env.sh
```

### 3.2 数据生成

```bash
./llmlora/scripts/generate_data.sh                          # 默认 1000/100/50
./llmlora/scripts/generate_data.sh --train-size 2000 --seed 123
```

### 3.3 训练

```bash
./llmlora/scripts/train.sh                                  # 默认 3 epoch, bs=4, lr=2e-4, 自动合并
./llmlora/scripts/train.sh --epochs 5 --lr 1e-4             # 自定义超参
./llmlora/scripts/train.sh --max-steps 10 --no-merge        # 冒烟快跑
./llmlora/scripts/train.sh --resume-from-checkpoint <dir>   # 断点续训
```

### 3.4 评估

```bash
# PyTorch 后端（默认）
./llmlora/scripts/evaluate.sh                               # 默认评估合并模型（全部测试样本）
./llmlora/scripts/evaluate.sh --max-samples 20

# vLLM 后端（更快，约 7x 加速）
./llmlora/scripts/evaluate.sh --backend vllm --max-samples 20

# 基座 + LoRA adapter 模式
./llmlora/scripts/evaluate.sh \
    --model-path llmlora/basemodels/cmeee_merged \
    --adapter-path llmlora/output/saves/qwen35-cmeee-privacy-lora
```

> 注意：使用合并模型时默认**不再叠加** adapter（避免双重应用 LoRA 权重）；
> 如需叠加请显式传 `--adapter-path`。

| 推理后端 | 首次加载 | 单条推理延迟 | 吞吐 | 适用场景 |
|---|---|---|---|---|
| PyTorch（默认） | ~5s | ~4200ms | ~0.24 条/s | 开发调试、小批量评估 |
| vLLM | ~22s（含 CUDA Graph 捕获） | ~570ms | ~1.76 条/s | 大批量评估、生产部署 |

### 3.5 端到端冒烟测试

```bash
./llmlora/scripts/smoke_test.sh
```

### 3.6 等价的原生命令（不用脚本时）

```bash
cd /home/charles/code/sfwork/privacy-local-agent

llmlora/.venv/bin/python -m llmlora.scripts.generate_data --train-size 1000 --dev-size 100 --test-size 50
llmlora/.venv/bin/python -m llmlora.scripts.train --epochs 3 --batch-size 4
llmlora/.venv/bin/python -m llmlora.scripts.evaluate --model-path llmlora/output/models/Qwen3.5-0.8B-Privacy-Classifier-Smoother
llmlora/.venv/bin/python -m llmlora.scripts.evaluate --backend vllm --model-path llmlora/output/models/Qwen3.5-0.8B-Privacy-Classifier-Smoother
```

---

## 4. 核心 Python 参数速查

| 参数（train.py） | 默认 | 说明 |
|---|---|---|
| `--epochs` | 3 | 训练轮数 |
| `--max-steps` | -1 | 最大步数（-1=跑满 epoch，冒烟用） |
| `--batch-size` / `--grad-accum-steps` | 4 / 4 | 批大小 / 梯度累积 |
| `--lr` | 2e-4 | 学习率 |
| `--max-length` | 512 | 单样本最大 token 长度 |
| `--lora-r` / `--lora-alpha` / `--lora-dropout` | 16 / 32 / 0.05 | LoRA 超参 |
| `--dtype` | auto | auto / bf16 / fp16 / fp32 |
| `--no-merge` | — | 训练后不自动合并导出 |

---

## 5. 技术要点速览

1. **规则引擎驱动的数据打标**：`generate_data.py` 对接项目 `ConfigurableRuleEngine`（仅 general-pii + medical 规则包；finance 为 C 级体系不混入），level/category 由规则裁定，配合零泄漏双重校验（字面残留 + 规则复扫）。
2. **Prompt Labels Masking**：Qwen3.5 chat template 不含 `{% generation %}` 标记，官方 assistant mask 不可用；`loader.py` 采用「prompt 前缀长度定位」方案，损失仅作用于 Assistant JSON 输出。
3. **thinking 标记处理**：训练与推理统一 `enable_thinking=False`，避免模板注入空思考块污染 JSON 输出。
4. **LoRA 注入层**：自动探查全部 Linear 叶子层（含混合注意力的 `in_proj_*` 系列），排除 `lm_head`/`embed`/`mtp`，可训练参数约 1.42%。
5. **Sidecar 接入**：`src/inference/engine.py` 提供 `QwenPrivacyLoRAEngine`（线程安全、延迟加载、批处理），可替换 privacy-local-agent Layer-3 的通用 LLM 引擎。
6. **双推理后端**：支持 PyTorch 原生推理和 vLLM 高性能推理，通过 `--backend pytorch|vllm` 切换。vLLM 后端利用 PagedAttention 和 CUDA Graphs 实现约 7x 加速。
