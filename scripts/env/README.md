# 硬件加速与环境构建脚本 (scripts/env)

本目录包含 **数联天下 · 数盾 (`PrivShield`)** 在 Linux GPU 加速环境（尤其是 NVIDIA Blackwell / Ada / Ampere 等新一代架构）下的底层驱动、CUDA Toolkit、PyTorch 及 TensorRT 高性能推理引擎编译与部署脚本。

每个脚本均支持独立运行，以下为各脚本的详细说明与独立启动代码。

---

## 目录索引

- [`install_cuda_pytorch_sm120.sh` (CUDA 12.8 & PyTorch 安装与校验)](#install_cuda_pytorch_sm120sh)
- [`install_tensorrt_sm120.sh` (TensorRT 10.x & ONNX-GPU 安装)](#install_tensorrt_sm120sh)
- [`export_tensorrt_engine.sh` (编译 CMeEE Small-NER TensorRT 引擎)](#export_tensorrt_enginesh)

---

## 详细功能与启动命令

### `install_cuda_pytorch_sm120.sh`
- **作用说明**: 自动化安装 NVIDIA Blackwell (如 RTX 50 系列 / B100 / B200, 计算能力 `sm_120`) 及 Ampere/Ada 架构所需的 CUDA 12.8 Toolkit 和最新 PyTorch 运行时，并执行 GPU 张量计算与算力探针校验。
- **参数选项**:
  - `--install-cuda-toolkit`: 同时安装系统级 CUDA Toolkit（含 `nvcc` 编译器）。
  - `--pytorch-channel <CHANNEL>`: 指定 PyTorch 下载通道（如 `nightly/cu128`）。
- **执行命令**:
  ```bash
  # 默认安装 (CUDA 12.8 + PyTorch cu128 运行时)
  bash ./scripts/env/install_cuda_pytorch_sm120.sh
  ```
  ```bash
  # 完整安装系统级 CUDA Toolkit 与 nvcc 编译器
  bash ./scripts/env/install_cuda_pytorch_sm120.sh --install-cuda-toolkit
  ```

---

### `install_tensorrt_sm120.sh`
- **作用说明**: 安装针对 CUDA 12.8 与新一代 GPU 架构优化的 TensorRT 10.x 运行库（`tensorrt`, `tensorrt-cu12`, `tensorrt-lean` 等）及 `onnxruntime-gpu` 执行提供程序。
- **参数选项**:
  - `-v, --version <VER>`: 指定 TensorRT 版本（默认 `10.8.0`）。
  - `--system-install`: 同时安装系统 APT 底层依赖。
  - `--no-onnx-gpu`: 跳过 onnxruntime-gpu 仅安装核心 TensorRT。
- **执行命令**:
  ```bash
  # 默认安装 TensorRT 10.8.0 及 onnxruntime-gpu
  bash ./scripts/env/install_tensorrt_sm120.sh
  ```

---

### `export_tensorrt_engine.sh`
- **作用说明**: 读取 `.models/raner_cmeee.onnx` 等命名实体识别模型，结合 GPU 本地算力，使用 FP16 半精度与优化 Profile 编译生成极速硬件引擎 `.models/raner_cmeee.engine`。
- **参数选项**:
  - `--input <PATH>`: 指定输入的 ONNX 模型路径。
  - `--output <PATH>`: 指定生成的 TensorRT `.engine` 路径。
  - `--opt-shape <SHAPE>`: 优化批处理维度（如 `"input_ids:16x128"`）。
  - `--no-fp16`: 禁用 FP16（改用纯 FP32 精度编译）。
- **执行命令**:
  ```bash
  # 默认编译 CMeEE Small-NER 引擎 (FP16 开启, 自动探测本地 GPU)
  bash ./scripts/env/export_tensorrt_engine.sh
  ```
  ```bash
  # 自定义输入/输出路径编译
  bash ./scripts/env/export_tensorrt_engine.sh \
    --input .models/my_ner.onnx \
    --output .models/my_ner.engine
  ```
