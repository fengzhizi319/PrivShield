#!/usr/bin/env python3
"""临时诊断：复刻 Qwen3Classifier 完整 prompt，检查生成 token 序列。"""
import os
import sys
from pathlib import Path

os.environ["PRIVACY_ENV_PROFILE"] = ""
PROJECT_ROOT = str(Path(__file__).resolve().parents[2])
sys.path.insert(0, PROJECT_ROOT)

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

from engine.dynclassification.utils import wrap_untrusted_text

mp = os.path.join(PROJECT_ROOT, ".models/Qwen3.5-0.8B-Privacy-Classifier-Smoother")
tok = AutoTokenizer.from_pretrained(mp, trust_remote_code=True)
model = AutoModelForCausalLM.from_pretrained(
    mp, dtype=torch.bfloat16, device_map="cuda", trust_remote_code=True
)
model.eval()

system_prompt = (
    "你是一个专业的隐私安全Sidecar助手。请分析输入的文本，识别敏感信息，"
    "输出分类分级结果（JSON格式），并提供语义连贯的无痕抹平脱敏重写文本。\n\n"
    "【数据分类分级标准指南】\n"
    "- L1 (公开数据): 无敏感信息的公开资讯、通用日常文本。\n"
    "- L2 (内部数据): 业务统计指标、系统日志、设备运维等低敏感内部数据。\n"
    "- L3 (敏感数据/个人基本信息): 姓名、身份证号、手机号、银行卡号、电子邮箱等个人基础标识与资产信息。\n"
    "- L4 (高敏感数据/诊疗与金融敏感): 疾病诊断（如重度抑郁症、高血压、冠心病）、病历主诉、处方药品等医疗健康敏感信息。\n"
    "- L5 (极敏感数据): 基因组、生物特征、特级商业机密等核心数据。\n\n"
    "请严格根据上述标准进行定级，并仅输出符合以下 JSON 格式的结构化内容，不要包含额外的解释文字或 ``` 块：\n"
    "{\n"
    '  "final_level": "L1/L2/L3/L4/L5",\n'
    '  "confidence": 0.0到1.0之间的浮点数,\n'
    '  "reasoning": "定级判别的推理过程说明",\n'
    '  "sanitized_text": "语义连贯的无痕抹平脱敏重写文本"\n'
    "}"
)
user_text = f"请评估以下文本数据的敏感数据等级：\n{wrap_untrusted_text('身份证号：510101199001011234')}"
messages = [
    {"role": "system", "content": system_prompt},
    {"role": "user", "content": user_text},
]
prompt = tok.apply_chat_template(
    messages, tokenize=False, add_generation_prompt=True, enable_thinking=False
)
print("PROMPT_TAIL>", repr(prompt[-160:]))
inputs = tok([prompt], return_tensors="pt").to(model.device)
with torch.no_grad():
    out = model.generate(**inputs, max_new_tokens=512)
new_ids = out[0][inputs["input_ids"].shape[1] :]
print("num_new_tokens:", len(new_ids))
print("last 25 ids:", new_ids[-25:].tolist())
print("DECODE_SPECIAL>", repr(tok.decode(new_ids, skip_special_tokens=False)[-400:]))
