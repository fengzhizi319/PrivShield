"""临时分析脚本：验证 redact_medical_text 的若干疑点（不改动源码）。"""
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from engine.medical_pipeline.rules import redact_medical_text, _REDACT_MAX_TEXT_LENGTH

cases = [
    # 1. 干净文本（无任何 L4/L5 词）是否被“自愈”逻辑篡改
    ("干净文本-亲属+引号", "弟弟说'你好'，今天天气不错。"),
    ("干净文本-中文引号", "母亲“高血压”控制良好。"),
    ("干净文本-长期。", "他长期。"),
    ("干净文本-重复标点", "注意保暖。。。多休息"),
    ("干净文本-换行", "第一段。\n\n第二段。"),
    # 2. 步骤 5 是否架空了步骤 6/7（顿号列表场景）
    ("顿号列表-敏感在前", "一弟患'重度精神分裂症'、'2型糖尿病'。"),
    ("顿号列表-敏感在后", "一弟患'2型糖尿病'、'重度精神分裂症'。"),
    ("单敏感疾病", "一弟患'重度精神分裂症'。"),
    # 3. 服药句法缺少结尾关键词时
    ("服药-无治疗后缀", "服用'奥氮平片'20mg qd。"),
    ("服药-有治疗后缀", "长期服用'奥氮平片'20mg qd控制症状。"),
    # 4. 死因
    ("死因", "因'恶性肿瘤'去世。"),
    ("死因-导致并发症", "因'HIV'导致的并发症去世。"),
    # 5. 幂等性
    ("幂等-两次", None),
]

for name, text in cases:
    if text is None:
        continue
    out = redact_medical_text(text)
    changed = "✏️ " if out != text else "  "
    print(f"{changed}[{name}]\n  原: {text!r}\n  出: {out!r}")

# 幂等性专项
t = "一弟患'重度精神分裂症'、'2型糖尿病'。"
o1 = redact_medical_text(t)
o2 = redact_medical_text(o1)
print(f"\n幂等性: {o1!r} -> {o2!r}  {'一致' if o1 == o2 else '不一致!'}")

# 6. 性能：接近阈值上限的超长文本（无敏感词，最坏情况：每个位置都尝试 71 项交替）
for fill in ["普通的临床记录文字，患者一般情况良好。" * 5, "a" * 200]:
    n = _REDACT_MAX_TEXT_LENGTH - 100
    text = (fill * (n // len(fill) + 1))[:n]
    start = time.perf_counter()
    redact_medical_text(text)
    elapsed = time.perf_counter() - start
    print(f"性能: 长度={len(text)}, 填充={'中文' if fill.startswith('普通') else '单字符'}, 耗时={elapsed:.3f}s")

# 超过阈值时的降级路径
text = "正常的病历描述，" * 4000 + "艾滋病" + "正常的病历描述，" * 4000
start = time.perf_counter()
out = redact_medical_text(text)
print(f"降级路径: 长度={len(text)}, 耗时={time.perf_counter() - start:.3f}s, '艾滋'残留={'艾滋' in out}")
