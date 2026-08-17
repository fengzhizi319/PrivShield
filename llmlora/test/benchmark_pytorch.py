# -*- coding: utf-8 -*-
"""
PyTorch 原生推理性能测试 (Fast Sub-20s Mode).

基于 HuggingFace Transformers 原生接口对 Qwen3.5-0.8B 开展 Batch 推理基准测试。
测试涵盖不同 Batch 大小下的总耗时、单条平摊延迟以及系统综合吞吐量 (Tokens/s)。
"""
# 启用 Python 3.7+ 的类型注解延迟求值特性
from __future__ import annotations

# 导入命令行参数解析模块
import argparse
# 导入 Python 垃圾回收控制模块，用于测试结束后主动清理无用对象
import gc
# 导入 JSON 序列化与反序列化模块，用于测试结果的落盘存储
import json
# 导入高精度性能计时器模块
import time
# 导入 Python 系统级模块，用于操作模块搜索路径 sys.path
import sys
# 导入面向对象的跨平台文件路径处理模块
from pathlib import Path
# 导入类型提示泛型：字典、列表与任意类型
from typing import Dict, List, Any

# 解析当前脚本所在位置的上两级目录作为代码仓库根目录 (PrivShield 根路径)
_REPO_ROOT = Path(__file__).resolve().parents[2]
# 若仓库根目录未包含在 Python 模块搜索路径中，则优先插入到 sys.path 首位
if str(_REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(_REPO_ROOT))

# 导入 PyTorch 深度学习框架
import torch
# 从 HuggingFace Transformers 库导入因果语言模型类与分词器类
from transformers import AutoModelForCausalLM, AutoTokenizer
# 从 llmlora 数据集工具中导入提示词 ChatML 模板渲染函数与 JSONL 数据读取函数
from llmlora.src.dataset.loader import render_prompt_text, load_jsonl


def run_pytorch_benchmark(
    model_path: str,                  # 待评测的大模型本地权重目录绝对/相对路径
    test_data_path: str,              # 评测测试集 JSONL 文件路径
    batch_sizes: List[int] = [1, 4],  # 需要依次执行基准评测的 Batch Size 列表 (默认评测 Batch=1 与 Batch=4)
    max_new_tokens: int = 64,         # 自回归生成阶段允许生成的最大 Token 数量上限
) -> Dict[str, Any]:
    """运行 PyTorch 原生推理性能基准测试并返回结构化测试指标."""
    # 打印基准测试终端分割线
    print("=" * 64)
    # 打印启动提示日志
    print("🔥 启动 PyTorch 原生推理性能 Benchmark (快速模式)")
    # 打印待测模型所在的物理路径
    print(f"  模型路径: {model_path}")
    # 打印基准测试终端分割线
    print("=" * 64)

    # 从磁盘读取评测数据集 (格式为每行一个包含 input 字段的 JSON 对象)
    samples = load_jsonl(test_data_path)
    # 若测试样本文件为空或未包含任何有效记录，则打印错误并提前返回空字典
    if not samples:
        print("❌ 测试集数据为空")
        return {}

    # 检测当前运行环境是否存在可用 CUDA GPU，若存在则使用 GPU 计算，否则降级回退到 CPU
    device = "cuda" if torch.cuda.is_available() else "cpu"
    # 记录模型与分词器开始加载的高精度时间戳
    start_load = time.perf_counter()

    # =========================================================================
    # [Batch 关键机制 1: Left Padding (左侧填充)]
    # 在 Batch 自回归生成中，由于每个样本的 Prompt 长度不同，必须对齐到批次最大长度。
    # 必须指定 padding_side="left"，原因如下：
    # 1. 因果自回归生成是在序列最右端向右逐字追加新生成的 Token；
    # 2. 若使用默认的 right padding，短样本右侧为 <pad>，解码时模型会将 <pad> 误作为有效上下文输入；
    # 3. left padding 使所有样本的有效上下文统一向右靠齐，新 Token 的生成紧接在各样本真实末尾展开。
    # =========================================================================
    tokenizer = AutoTokenizer.from_pretrained(model_path, trust_remote_code=True, padding_side="left")
    # 若分词器中未显式定义 pad_token，则将 eos_token (<|im_end|>) 作为安全兜底的 pad_token
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    # 加载因果语言模型主干权重
    model = AutoModelForCausalLM.from_pretrained(
        model_path,                                                                 # 模型权重目录路径
        trust_remote_code=True,                                                     # 信任并执行模型代码仓库中的自定义混合注意力架构实现
        torch_dtype=torch.bfloat16 if torch.cuda.is_available() else torch.float32, # GPU 环境下启用 bfloat16 混合精度加速，CPU 下使用 float32
        device_map="auto" if torch.cuda.is_available() else None,                   # 自动将模型各层权重映射到可用 GPU 显存
    )
    # 将模型切换至评估模式 (禁用 Dropout 并冻结 BatchNorm/LayerNorm 动态更新)
    model.eval()
    # 计算模型加载阶段累计消耗的秒数
    load_time = time.perf_counter() - start_load
    # 打印模型加载完成日志及耗时
    print(f"✅ PyTorch 模型加载完成，耗时: {load_time:.2f}s\n")

    # 初始化用于存储各个 Batch Size 评测指标的结果字典
    results = {}

    # 打印测试结果表格表头分割线
    print("-" * 64)
    # 格式化打印表格各列标题：Batch 大小、总耗时、单条平摊延迟、系统吞吐量
    print(f"{'Batch Size':<12} | {'总耗时(ms)':<12} | {'单条延迟(ms)':<14} | {'吞吐(tokens/s)':<14}")
    # 打印测试结果表格表头分割线
    print("-" * 64)

    # 遍历待评测的 Batch Size 列表 (例如依次评测 1, 4)
    for bsize in batch_sizes:
        # =====================================================================
        # [Batch 逻辑 2: 样本循环扩充与 Batch 集合切片组装]
        # 若原始测试集样本数量少于当前待测的 batch_size，通过循环复制平铺样本列表，
        # 并通过切片 [:bsize] 精准提取出包含 bsize 条记录的样本列表。
        # =====================================================================
        batch_items = (samples * ((bsize // len(samples)) + 2))[:bsize]
        # 使用 ChatML 对话模板渲染各样本输入，生成包含 System 提示词与 User 输入的完整 Prompt 文本列表
        prompts = [render_prompt_text(tokenizer, s["input"]) for s in batch_items]

        # =====================================================================
        # [Batch 逻辑 3: 批量分词与张量对齐 (Batched Tokenization & Left Padding)]
        # 1. 将 List[str] 批量传入 Tokenizer 进行高效 Rust BPE 并行分词；
        # 2. padding=True 自动探查本批次中最长序列长度，对较短序列自动在左侧填充 pad_token_id；
        # 3. return_tensors="pt" 将输出转换为 PyTorch 张量格式；
        # 4. .to(device) 将生成的 input_ids 与 attention_mask 张量整体搬移到 GPU 显存。
        # =====================================================================
        inputs = tokenizer(prompts, return_tensors="pt", padding=True).to(device)

        # 记录前向推理与自回归解码开始的高精度时间戳
        start_gen = time.perf_counter()
        # 禁用 PyTorch 自动求导机制，避免构建反向传播计算图并大幅节省前向显存开销
        with torch.no_grad():
            # =================================================================
            # [Batch 逻辑 4: 批量并行前向推理与自回归解码 (Batched Generation)]
            # 1. Prefill 预填充阶段: GPU 并行将 Batch 内所有序列的 Prompt 执行矩阵乘法 (GEMM)；
            # 2. Decode 自回归解码阶段: 24 层 Hybrid 网络 (18 层 SSM 状态与 6 层 GQA KV Cache)
            #    按批次维度统一进行步进递推，单次前向计算同时输出 B 条请求的下一个 Token；
            # 3. do_sample=False 启用确定性贪心搜索解码；
            # 4. 输出 outputs 的张量形状为 [B, max_prompt_len + actual_new_tokens]。
            # =================================================================
            outputs = model.generate(
                **inputs,                             # 传入 input_ids 与 attention_mask
                max_new_tokens=max_new_tokens,         # 限制单条序列最大生成长度
                do_sample=False,                      # 贪心解码，保证评测结果具备确定性与可复现性
                pad_token_id=tokenizer.pad_token_id,   # 显式指定 Padding 标记 ID
            )
        # 计算本次 Batch 推理生成所消耗的实际物理时间 (单位: 秒)
        elapsed_sec = time.perf_counter() - start_gen
        # 将耗时换算为毫秒 (ms)
        elapsed_ms = elapsed_sec * 1000.0

        # =====================================================================
        # [Batch 逻辑 5: 批量性能指标统计与吞吐量折算]
        # 1. 单条生成 Token 数 = 模型输出总序列长度 - 输入 input_ids 的长度；
        # 2. 本 Batch 累计生成 Token 总量 = 单条生成 Token 数 * 当前 Batch Size；
        # 3. 系统综合并发吞吐量 (tokens/s) = 累计生成 Token 总数 / 本次 Batch 总耗时 (秒)；
        # 4. 单条平摊处理延迟 (avg_latency_ms) = Batch 总耗时 / Batch Size (反映并发并行的平摊收益)。
        # =====================================================================
        gen_tokens = outputs.shape[1] - inputs["input_ids"].shape[1]
        total_tokens = gen_tokens * bsize
        tokens_per_sec = total_tokens / elapsed_sec if elapsed_sec > 0 else 0.0
        avg_latency = elapsed_ms / bsize

        # 将当前 Batch 大小的评测数据结构化记录到结果字典中
        results[f"batch_{bsize}"] = {
            "batch_size": bsize,                  # 当前测试的 Batch 大小
            "elapsed_ms": elapsed_ms,              # 该 Batch 完成全流程推理的总耗时 (ms)
            "avg_latency_ms": avg_latency,          # 单条请求平摊延迟 (ms)
            "tokens_per_sec": tokens_per_sec,      # 系统每秒生成 Token 吞吐量 (tokens/s)
        }

        # 格式化输出当前 Batch 大小的评测数据行到控制台
        print(f"{bsize:<12} | {elapsed_ms:<12.1f} | {avg_latency:<14.1f} | {tokens_per_sec:<14.2f}")

    # 打印测试结果表格底部分割线
    print("-" * 64)

    # 显式删除模型对象引用，断开显存张量依赖
    del model
    # 强制执行 Python 垃圾回收
    gc.collect()
    # 若运行在 CUDA 环境下，清空 PyTorch 缓存分配器中的显存碎片
    if torch.cuda.is_available():
        torch.cuda.empty_cache()

    # 返回所有 Batch Size 的评测结果字典
    return results


def main():
    """命令行启动主函数，解析入参并执行基准评测."""
    # 创建命令行参数解析器对象
    parser = argparse.ArgumentParser(description="PyTorch 快速推理性能测试")
    # 添加模型路径参数配置 (默认指向 llmlora 训练合并导出的标准模型目录)
    parser.add_argument(
        "--model-path",
        type=str,
        default=str(_REPO_ROOT / "llmlora" / "output" / "models" / "Qwen3.5-0.8B-Privacy-Classifier-Smoother"),
        help="待评测模型权重的本地物理路径",
    )
    # 添加测试集数据文件路径参数配置
    parser.add_argument(
        "--test-data",
        type=str,
        default=str(_REPO_ROOT / "llmlora" / "data" / "test.jsonl"),
        help="评测使用的 JSONL 测试集文件路径",
    )
    # 添加可选的测试结果 JSON 输出文件路径参数配置
    parser.add_argument(
        "--json-out",
        type=str,
        default="",
        help="评测指标结果持久化保存的 JSON 文件路径 (可选)",
    )
    # 解析命令行传入的所有实际参数
    args = parser.parse_args()

    # 调用基准评测主函数执行性能测试
    results = run_pytorch_benchmark(args.model_path, args.test_data)
    # 若用户指定了 --json-out 且测试产生了有效指标，则将结果以格式化 JSON 写入指定文件
    if args.json_out and results:
        Path(args.json_out).write_text(json.dumps(results, indent=2), encoding="utf-8")


# 当该文件作为独立脚本直接执行时的标准程序入口
if __name__ == "__main__":
    main()
