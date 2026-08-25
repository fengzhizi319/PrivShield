# AI 模型管理与推理服务脚本 (scripts/models)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 所需的本地 AI 模型的自动化下载、权重转换（macOS Apple Silicon MLX）以及 vLLM OpenAI 兼容推理服务的启动脚本。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [`download_model.py` (多模态大模型权重下载)](#download_modelpy)
- [`download_ner_model.py` (Small-NER 命名实体识别模型下载)](#download_ner_modelpy)
- [`convert_models_to_mlx.py` (Apple MLX 格式模型转换)](#convert_models_to_mlxpy)
- [`start_vllm_server.sh` (Shell 脚本启动 vLLM 推理服务)](#start_vllm_serversh)
- [`run_vllm_server.py` (Python 脚本启动 vLLM 推理服务)](#run_vllm_serverpy)

---

## 详细功能与启动命令

### `download_model.py`
- **作用说明**: 优先通过 ModelScope 国内高速镜像（或 Hugging Face 镜像）自动下载多模态大模型（Qwen2-VL-2B-Instruct）权重至 `.models/` 目录。
- **执行命令**:
  ```bash
  python scripts/models/download_model.py
  ```

---

### `download_ner_model.py`
- **作用说明**: 自动下载 Layer-2 Small-NER 医疗命名实体识别所需的 ONNX 模型（`raner_cmeee.onnx`）与分词词表（`vocab.txt`）至 `.models/` 目录。
- **执行命令**:
  ```bash
  python scripts/models/download_ner_model.py
  ```

---

### `convert_models_to_mlx.py`
- **作用说明**: 【macOS 专属】在 Apple Silicon (M1/M2/M3/M4) 芯片的 Mac 上，将 Hugging Face 格式的模型权重转换为 Apple MLX 格式，以利用 Apple Metal GPU 进行端侧低功耗零显存占用推理。
- **参数选项**:
  - `--model <PATH>`: Hugging Face 格式模型源路径。
  - `--output <PATH>`: 输出 MLX 格式模型目录。
- **执行命令**:
  ```bash
  # 转换多模态大模型权重至 MLX 格式
  python scripts/models/convert_models_to_mlx.py \
      --model .models/Qwen2-VL-2B-Instruct \
      --output .models/Qwen2-VL-2B-Instruct-mlx
  ```
  ```bash
  # 转换 Small-NER 模型权重至 MLX 格式
  python scripts/models/convert_models_to_mlx.py \
      --model .models/raner_cmeee \
      --output .models/raner_cmeee-mlx
  ```

---

### `start_vllm_server.sh`
- **作用说明**: 【推荐方式】通过 Shell 脚本一键启动高性能 vLLM 本地大模型推理服务，加载环境变量配置并对外提供 OpenAI 兼容的 `/v1/chat/completions` API 接口（端口 `:8000`）。
- **执行命令**:
  ```bash
  bash ./scripts/models/start_vllm_server.sh
  ```

---

### `run_vllm_server.py`
- **作用说明**: 通过 Python 运行时直接读取环境配置文件并拉起 vLLM 大模型推理子进程。
- **执行命令**:
  ```bash
  python scripts/models/run_vllm_server.py
  ```
