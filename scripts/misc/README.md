# 诊断与实验工具脚本 (scripts/misc)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 在研发与算法调优过程中使用的专用诊断脚本、Prompt 结构审查与边界条件探针工具。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [`analyze_redact.py` (医疗文本抹平边界用例与幂等性分析)](#analyze_redactpy)
- [`diag_fullprompt.py` (大模型 Chat Template 与 Token 序列结构诊断)](#diag_fullpromptpy)

---

## 详细功能与启动命令

### `analyze_redact.py`
- **作用说明**: 对医疗病历复杂句法脱敏（死因、顿号并列疾病、服药句式、标点连续性及长文本）进行边界条件探针，验证无敏感信息文本是否被误伤以及多轮脱敏的幂等性与零泄露保证。
- **执行命令**:
  ```bash
  python scripts/misc/analyze_redact.py
  ```

---

### `diag_fullprompt.py`
- **作用说明**: 加载本地 Qwen3.5 分类分级微调模型 Tokenizer，复刻完整的 System Prompt、防护包裹（`wrap_untrusted_text`）与 Chat Template 组装逻辑，输出尾部 Token 序列与模板渲染格式，以便调试模型输出行为与边界截断。
- **执行命令**:
  ```bash
  python scripts/misc/diag_fullprompt.py
  ```
