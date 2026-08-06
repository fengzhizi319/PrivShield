#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: export_tensorrt_engine.sh
# 脚本说明: 自动化把 ONNX 模型（如 CMeEE Small-NER 命名实体识别模型）
#           针对目标 GPU 架构（如 Blackwell sm_120 / Ada sm_89 / Ampere sm_86）
#           编译构建为 TensorRT 高性能硬件引擎 (.engine)。
#
# 核心流程:
#   1. 检查 ONNX 输入模型文件与参数设置
#   2. 探针本地 GPU 架构 (Compute Capability)
#   3. 配置动态 Shape (Optimization Profile) 与 FP16 精度开关
#   4. 调用 trtexec 或 Python TensorRT Builder 执行底层 GPU 内核编译
#   5. 校验生成的 .engine 引擎可用性
# ==============================================================================

set -euo pipefail

# ANSI 终端颜色代码
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

ONNX_INPUT=".models/raner_cmeee.onnx"
ENGINE_OUTPUT=".models/raner_cmeee.engine"
ENABLE_FP16=true
MIN_SHAPE="input_ids:1x16"
OPT_SHAPE="input_ids:16x128"
MAX_SHAPE="input_ids:64x512"

usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  -i, --input ONNX_PATH     ONNX 输入模型文件路径 (默认: .models/raner_cmeee.onnx)
  -o, --output ENGINE_PATH  输出 TensorRT Engine 保存路径 (默认: .models/raner_cmeee.engine)
  --no-fp16                 关闭 FP16 半精度加速，使用纯 FP32 精度
  --opt-shape SHAPE         推荐输入 Dynamic Shape (默认: input_ids:16x128)
  --max-shape SHAPE         最大输入 Dynamic Shape (默认: input_ids:64x512)
  -h, --help                显示本帮助信息并退出

使用示例:
  ./scripts/env/export_tensorrt_engine.sh
  ./scripts/env/export_tensorrt_engine.sh -i model.onnx -o model_sm120.engine
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -i|--input)
            ONNX_INPUT="$2"
            shift 2
            ;;
        -o|--output)
            ENGINE_OUTPUT="$2"
            shift 2
            ;;
        --no-fp16)
            ENABLE_FP16=false
            shift
            ;;
        --opt-shape)
            OPT_SHAPE="$2"
            shift 2
            ;;
        --max-shape)
            MAX_SHAPE="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo -e "${RED}错误: 未知选项 $1${NC}"
            usage
            ;;
    esac
done

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} ONNX 到 TensorRT Engine 自动化编译工具${NC}"
echo -e "${BLUE} ONNX 输入文件 : ${ONNX_INPUT}${NC}"
echo -e "${BLUE} Engine 输出   : ${ENGINE_OUTPUT}${NC}"
echo -e "${BLUE} FP16 模式     : ${ENABLE_FP16}${NC}"
echo -e "${BLUE}====================================================${NC}"

# 1. 检查输入 ONNX 文件是否存在
if [ ! -f "$ONNX_INPUT" ]; then
    echo -e "${YELLOW}警告: 未在 ${ONNX_INPUT} 找到 ONNX 文件。正在尝试自动下载...${NC}"
    python3 -m privacy_local_agent.privacy.download_ner_model || {
        echo -e "${RED}错误: 无法获取 ONNX 模型，请确保离线模型文件就失。${NC}"
        exit 1
    }
fi

# 2. 识别 GPU 算力架构
echo -e "\n${YELLOW}[步骤 1/3] 探针当前硬件 GPU 架构...${NC}"
python3 -c "
import torch
if torch.cuda.is_available():
    name = torch.cuda.get_device_name(0)
    cap = torch.cuda.get_device_capability(0)
    print(f'目标 GPU : {name} (sm_{cap[0]}{cap[1]})')
else:
    print('警告: 未检测到可用 CUDA GPU，TensorRT 引擎必须在目标 GPU 环境上编译!')
"

# 3. 编译引擎
echo -e "\n${YELLOW}[步骤 2/3] 开始构建 TensorRT Engine...${NC}"
mkdir -p "$(dirname "$ENGINE_OUTPUT")"

if command -v trtexec &> /dev/null; then
    echo -e "使用 ${BLUE}trtexec${NC} 命令行构建..."
    FP16_FLAG=""
    if [ "$ENABLE_FP16" = true ]; then
        FP16_FLAG="--fp16"
    fi

    trtexec \
        --onnx="${ONNX_INPUT}" \
        --saveEngine="${ENGINE_OUTPUT}" \
        ${FP16_FLAG} \
        --minShapes="${MIN_SHAPE}" \
        --optShapes="${OPT_SHAPE}" \
        --maxShapes="${MAX_SHAPE}" \
        --memPoolSize=workspace:2048MiB
else
    echo -e "${YELLOW}未在系统 PATH 中找到 trtexec，使用 Python TensorRT API 构建...${NC}"
    python3 -c "
import tensorrt as trt
import sys

logger = trt.Logger(trt.Logger.INFO)
builder = trt.Builder(logger)
flag = 1 << int(trt.NetworkDefinitionCreationFlag.EXPLICIT_BATCH)
network = builder.create_network(flag)
config = builder.create_builder_config()
parser = trt.OnnxParser(network, logger)

with open('${ONNX_INPUT}', 'rb') as f:
    if not parser.parse(f.read()):
        print('[-] ONNX 解析失败:', parser.get_error(0))
        sys.exit(1)

if ${ENABLE_FP16} and builder.platform_has_tf32:
    config.set_flag(trt.BuilderFlag.FP16)

profile = builder.create_optimization_profile()
# 配置默认 动态 shape
profile.set_shape('input_ids', min=(1, 16), opt=(16, 128), max=(64, 512))
config.add_optimization_profile(profile)

print('[*] TensorRT 开始编译图与自动调优 CUDA Kernel...')
serialized_engine = builder.build_serialized_network(network, config)

with open('${ENGINE_OUTPUT}', 'wb') as f:
    f.write(serialized_engine)
print('[+] TensorRT Engine 编译成功并导出!')
"
fi

# 4. 校验产物
echo -e "\n${YELLOW}[步骤 3/3] 校验导出的 Engine 引擎...${NC}"
if [ -f "$ENGINE_OUTPUT" ]; then
    SIZE_MB=$(du -m "$ENGINE_OUTPUT" | cut -f1)
    echo -e "${GREEN}[成功] TensorRT Engine 编译完成！${NC}"
    echo -e "文件位置: ${ENGINE_OUTPUT}"
    echo -e "文件大小: ${SIZE_MB} MB"
else
    echo -e "${RED}[失败] 引擎构建失败，未生成产物。${NC}"
    exit 1
fi
