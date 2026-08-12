#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: install_tensorrt_sm120.sh
# 脚本说明: 自动化安装支持 NVIDIA Blackwell 架构 (sm120 / sm_120 / CUDA 12.8)
#           的 TensorRT 10.x 高级推理解析器及相关 Python/C++ 依赖。
#
# 架构与背景知识:
#   - TensorRT 10.x (如 TensorRT 10.8+) 开始全面提供针对 CUDA 12.8 及 Blackwell
#     架构 (sm120) 的算子融合与硬件加速支持。
#   - 脚本会自动安装 tensorrt, tensorrt-cu12, tensorrt-lean, tensorrt-dispatch
#     以及用于 ONNX 模型的 TensorRT 执行提供程序 (onnxruntime-gpu)。
#
# 系统依赖:
#   - Linux (x86_64)
#   - Python 3.13+
#   - 宿主机已配置 CUDA 12.8 驱动或运行环境
# ==============================================================================

# 严格模式：遇到错误立即终止，防止未定义变量引起的隐患
set -euo pipefail

# 终端 ANSI 彩色输出定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # 重置颜色 (No Color)

# 默认参数变量
TRT_VERSION="10.8.0"                 # 默认目标 TensorRT 版本号
INSTALL_ONNX_GPU=true                # 是否自动安装支持 TensorRT EP 的 onnxruntime-gpu
INSTALL_SYSTEM_DEBS=false            # 是否尝试通过系统 APT 安装 C++ 原生库 (libnvinfer-dev 等)
EXTRA_INDEX_URL="https://pypi.nvidia.com" # NVIDIA 官方 PyPI 额外索引地址

# ------------------------------------------------------------------------------
# 函数: usage
# 说明: 打印帮助说明文档
# ------------------------------------------------------------------------------
usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  -v, --version VERSION         指定 TensorRT 版本 (默认: 10.8.0)
  --no-onnx-gpu                 跳过安装 onnxruntime-gpu
  --system-install              尝试通过 APT 安装系统级 C++ TensorRT 库 (libnvinfer-dev 等)
  -h, --help                    显示本帮助信息并退出

使用示例:
  # 基础安装 (安装默认版本的 TensorRT Python 运行时)
  ./scripts/env/install_tensorrt_sm120.sh

  # 附带安装系统级 C++ 开发头文件 (需要 root / sudo 权限)
  ./scripts/env/install_tensorrt_sm120.sh --system-install
EOF
    exit 0
}

# ------------------------------------------------------------------------------
# 命令行选项解析
# ------------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case "$1" in
        -v|--version)
            TRT_VERSION="$2"
            shift 2
            ;;
        --no-onnx-gpu)
            INSTALL_ONNX_GPU=false
            shift
            ;;
        --system-install)
            INSTALL_SYSTEM_DEBS=true
            shift
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
echo -e "${BLUE} 开始安装 TensorRT (支持 sm120 / Blackwell 架构)${NC}"
echo -e "${BLUE} 目标 TensorRT 版本    : ${TRT_VERSION}${NC}"
echo -e "${BLUE} NVIDIA 额外 PyPI 索引 : ${EXTRA_INDEX_URL}${NC}"
echo -e "${BLUE}====================================================${NC}"

# ------------------------------------------------------------------------------
# 步骤 1: 检查 Python 环境与 CUDA 上下文
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 1/4] 检查 Python 与 CUDA 运行环境...${NC}"

if ! command -v python3 &> /dev/null; then
    echo -e "${RED}错误: 未检测到 python3，请先配置 Python 环境。${NC}"
    exit 1
fi

# 检查当前 PyTorch 是否可正确感知 CUDA (如果已安装 PyTorch)
python3 -c "
import torch
print(f'PyTorch 版本   : {torch.__version__}')
print(f'PyTorch CUDA   : {torch.version.cuda}')
print(f'CUDA 硬件可用性: {torch.cuda.is_available()}')
" 2>/dev/null || echo -e "${YELLOW}提示: 当前 Python 环境中未检测到 PyTorch 或 CUDA 扩展，将直接安装 TensorRT 包。${NC}"

# ------------------------------------------------------------------------------
# 步骤 2: (可选) 安装系统级 TensorRT C++ 依赖包
# 说明: 包括 libnvinfer-dev, libnvonnxparsers-dev 等，便于 C++ 开发或部署
# ------------------------------------------------------------------------------
if [ "$INSTALL_SYSTEM_DEBS" = true ]; then
    echo -e "\n${YELLOW}[步骤 2/4] 尝试通过 APT 安装系统级 TensorRT C++ 开发库...${NC}"
    if [ "$EUID" -ne 0 ] && ! command -v sudo &> /dev/null; then
        echo -e "${RED}错误: 安装系统包需要 root 权限或 sudo 命令。${NC}"
        exit 1
    fi
    SUDO_CMD=""
    if [ "$EUID" -ne 0 ]; then
        SUDO_CMD="sudo"
    fi

    $SUDO_CMD apt-get update -y
    $SUDO_CMD apt-get install -y libnvinfer-dev libnvonnxparsers-dev libnvparsers-dev python3-libnvinfer || {
        echo -e "${YELLOW}当前系统 APT 源中未找到官方包，将主要依赖 PyPI 提供的 TensorRT Python 库。${NC}"
    }
else
    echo -e "\n${YELLOW}[步骤 2/4] 已跳过系统级 APT C++ 库安装 (如需安装可添加 --system-install 参数)。${NC}"
fi

# ------------------------------------------------------------------------------
# 步骤 3: 使用 Pip 安装 TensorRT Python 轮子组件
# 说明:
#   - tensorrt: 主接口
#   - tensorrt-cu12: CUDA 12.x 专属加速库
#   - tensorrt-lean & tensorrt-dispatch: 10.x 引入的轻量级运行时与分发组件
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 3/4] 通过 pip 安装 TensorRT Python 轮子组件...${NC}"
python3 -m pip install --upgrade pip setuptools wheel

echo -e "正在从 NVIDIA 索引源安装: ${BLUE}tensorrt, tensorrt-cu12, tensorrt-lean, tensorrt-dispatch${NC}..."
python3 -m pip install \
    "tensorrt" \
    "tensorrt-cu12" \
    "tensorrt-lean" \
    "tensorrt-dispatch" \
    --extra-index-url "${EXTRA_INDEX_URL}" || {
        echo -e "${YELLOW}使用 NVIDIA PyPI 索引安装部分轮子超时/失败，回退使用标准 PyPI 索引...${NC}"
        python3 -m pip install tensorrt tensorrt-cu12
    }

# 安装 ONNX Runtime GPU 扩展 (带 TensorRT Execution Provider)
if [ "$INSTALL_ONNX_GPU" = true ]; then
    echo -e "正在安装 ${BLUE}onnxruntime-gpu${NC} (用于支持 ONNX TensorRT 执行引擎)..."
    python3 -m pip install onnxruntime-gpu --extra-index-url https://aiinfra.pkgs.visualstudio.com/PublicPackages/_packaging/onnxruntime-cuda-12/pypi/simple/ || \
    python3 -m pip install onnxruntime-gpu || true
fi

# ------------------------------------------------------------------------------
# 步骤 4: 运行 Python 验证脚本
# 说明: 测试 tensorrt 模块导入、日志对象初始化与 Builder 引擎能力
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 4/4] 验证 TensorRT 安装与驱动初始化...${NC}"

python3 -c "
import sys

print('================ TensorRT 驱动与运行时验证 ================')
try:
    import tensorrt as trt
    print(f'TensorRT 版本           : {trt.__version__}')
    
    # 初始化 TensorRT Logger 与 Builder
    logger = trt.Logger(trt.Logger.INFO)
    builder = trt.Builder(logger)
    print(f'TensorRT Builder 初始化 : 成功 [SUCCESS]')
    
    # 检查硬件感知 (如果 PyTorch 可用)
    try:
        import torch
        if torch.cuda.is_available():
            dev_name = torch.cuda.get_device_name(0)
            cap = torch.cuda.get_device_capability(0)
            print(f'当前 GPU 硬件           : {dev_name} (sm_{cap[0]}{cap[1]})')
    except Exception:
        pass
        
    print('\n[成功] TensorRT 已成功安装并可正常初始化引擎！')
except Exception as e:
    print(f'\n[错误] TensorRT 初始化验证失败: {e}')
    sys.exit(1)
print('=======================================================')
"

echo -e "\n${GREEN}TensorRT (sm120) 安装与验证全部完成！${NC}"
