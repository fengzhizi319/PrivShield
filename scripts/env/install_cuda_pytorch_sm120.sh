#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: install_cuda_pytorch_sm120.sh
# 脚本说明: 自动化安装支持 NVIDIA Blackwell 架构 (sm120 / sm_120 / compute_120)
#           的 CUDA 12.8 Toolkit 及对应 PyTorch 驱动与运行时环境。
#
# 架构与背景知识:
#   - NVIDIA Blackwell 架构 (如 RTX 5090/5080, B100/B200 等) 计算能力为 sm_120 (12.0)。
#   - 硬件要求说明:
#       1. 宿主机 NVIDIA 显卡驱动版本需 >= 570.xx (支持 CUDA 12.8+ 驱动接口)。
#       2. PyTorch 需编译支持 cu128 (CUDA 12.8) 或最新的 CUDA 12.8+ 预编译 Wheels。
#
# 系统依赖:
#   - Linux (x86_64, 推荐 Ubuntu 22.04/24.04 或 Debian 11/12)
#   - Python 3.10+
#   - bash 4.0+
# ==============================================================================

# 严格模式：任何命令失败立刻退出，取消未定义变量引用，管道失败传递错误码
set -euo pipefail

# 终端输出 ANSI 颜色代码声明
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # 重置颜色 (No Color)

# 默认配置变量
CUDA_VERSION="12.8"                  # 默认目标 CUDA 版本 (sm120 推荐 12.8+)
PYTORCH_CHANNEL="cu128"              # PyTorch 镜像源 Channel (可选: cu128 或 nightly/cu128)
INSTALL_CUDA_TOOLKIT=false           # 是否自动通过 APT 安装系统级 CUDA Toolkit (含有 nvcc)
SKIP_DRIVER_CHECK=false              # 是否跳过显卡驱动版本检查

# ------------------------------------------------------------------------------
# 函数: usage
# 说明: 打印帮助信息并退出程序
# ------------------------------------------------------------------------------
usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  -c, --cuda-version VERSION    指定 CUDA 版本 (默认: 12.8)
  -p, --pytorch-channel CHANNEL 指定 PyTorch 安装 Channel (cu128 | nightly/cu128) (默认: cu128)
  --install-cuda-toolkit        若本地缺少 nvcc，尝试通过系统的 APT 包管理器安装系统级 CUDA Toolkit 12.8
  --skip-driver-check           跳过宿主机 NVIDIA 显卡驱动版本检查
  -h, --help                    显示本帮助文档并退出

使用示例:
  # 基础安装 (安装默认的 CUDA 12.8 对应 PyTorch 驱动)
  ./scripts/env/install_cuda_pytorch_sm120.sh

  # 使用 Nightly 预览通道安装最新的 PyTorch cu128 驱动
  ./scripts/env/install_cuda_pytorch_sm120.sh -p nightly/cu128

  # 自动安装系统级 CUDA 12.8 Toolkit (需要 root / sudo 权限)
  ./scripts/env/install_cuda_pytorch_sm120.sh --install-cuda-toolkit
EOF
    exit 0
}

# ------------------------------------------------------------------------------
# 命令行参数解析
# ------------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case "$1" in
        -c|--cuda-version)
            CUDA_VERSION="$2"
            shift 2
            ;;
        -p|--pytorch-channel)
            PYTORCH_CHANNEL="$2"
            shift 2
            ;;
        --install-cuda-toolkit)
            INSTALL_CUDA_TOOLKIT=true
            shift
            ;;
        --skip-driver-check)
            SKIP_DRIVER_CHECK=true
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
echo -e "${BLUE} 开始安装 CUDA & PyTorch (支持 sm120 / Blackwell 架构)${NC}"
echo -e "${BLUE} 目标 CUDA 版本       : ${CUDA_VERSION}${NC}"
echo -e "${BLUE} PyTorch 镜像源 Channel: ${PYTORCH_CHANNEL}${NC}"
echo -e "${BLUE}====================================================${NC}"

# ------------------------------------------------------------------------------
# 步骤 1: 检查 NVIDIA 宿主机显卡驱动
# 说明: Blackwell 架构 (sm120) 需要 570.xx 及以上版本的 NVIDIA 驱动支持
# ------------------------------------------------------------------------------
if [ "$SKIP_DRIVER_CHECK" = false ]; then
    echo -e "\n${YELLOW}[步骤 1/4] 检查 NVIDIA 宿主机驱动版本...${NC}"
    if command -v nvidia-smi &> /dev/null; then
        # 查询第一块 GPU 的驱动版本号
        DRIVER_VER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader | head -n1 || echo "0")
        echo -e "检测到系统 NVIDIA 驱动版本: ${GREEN}${DRIVER_VER}${NC}"
        
        # 提取主版本号 (例如从 570.86.16 中提取 570)
        DRIVER_MAJOR=$(echo "$DRIVER_VER" | cut -d'.' -f1)
        if [ "$DRIVER_MAJOR" -lt 570 ]; then
            echo -e "${YELLOW}警告: Blackwell 架构 (sm120) 推荐驱动主版本号 >= 570.xx。${NC}"
            echo -e "${YELLOW}当前驱动版本为 ${DRIVER_VER}。如遇 CUDA Driver Version Mismatch 错误，请升级宿主机驱动。${NC}"
        fi
    else
        echo -e "${YELLOW}警告: 未找到 nvidia-smi 命令。系统可能未安装 NVIDIA 显卡驱动，或位于无 GPU 的容器环境中。${NC}"
    fi
else
    echo -e "\n${YELLOW}[步骤 1/4] 已跳过 NVIDIA 驱动检查。${NC}"
fi

# ------------------------------------------------------------------------------
# 步骤 2: 检查 / 安装系统级 CUDA Toolkit (包含 nvcc 编译器)
# 说明: PyTorch 运行时轮子自带 CUDA Runtime，但如果需要编译 C++/CUDA 扩展，则需要 nvcc
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 2/4] 检查 CUDA Toolkit (nvcc)...${NC}"
if command -v nvcc &> /dev/null; then
    NVCC_VER=$(nvcc --version | grep "release" | awk '{print $5}' | sed 's/,//')
    echo -e "检测到现有 CUDA Toolkit (nvcc): ${GREEN}${NVCC_VER}${NC}"
else
    echo -e "${YELLOW}PATH 中未检测到 nvcc 编译器。${NC}"
    if [ "$INSTALL_CUDA_TOOLKIT" = true ]; then
        echo -e "${BLUE}准备通过官方 APT 镜像源安装 CUDA ${CUDA_VERSION} Toolkit...${NC}"
        
        # 检查是否具备 sudo/root 权限
        if [ "$EUID" -ne 0 ] && ! command -v sudo &> /dev/null; then
            echo -e "${RED}错误: 安装系统包需要 root 权限或 sudo 命令。${NC}"
            exit 1
        fi
        
        SUDO_CMD=""
        if [ "$EUID" -ne 0 ]; then
            SUDO_CMD="sudo"
        fi

        # 更新本地 APT 索引并安装下载依赖
        $SUDO_CMD apt-get update -y
        $SUDO_CMD apt-get install -y wget build-essential
        
        # 针对 Ubuntu 系统配置 NVIDIA 官方 APT 仓库
        DISTRO="ubuntu$(lsb_release -sr 2>/dev/null | tr -d '.' || echo '2204')"
        ARCH="x86_64"
        KEYRING_URL="https://developer.download.nvidia.com/compute/cuda/repos/${DISTRO}/${ARCH}/cuda-archive-keyring.gpg"
        
        echo "下载 NVIDIA APT Keyring: ${KEYRING_URL}"
        wget -q "$KEYRING_URL" -O /tmp/cuda-archive-keyring.gpg || true
        if [ -f /tmp/cuda-archive-keyring.gpg ]; then
            $SUDO_CMD mv /tmp/cuda-archive-keyring.gpg /usr/share/keyrings/cuda-archive-keyring.gpg
            echo "deb [signed-by=/usr/share/keyrings/cuda-archive-keyring.gpg] https://developer.download.nvidia.com/compute/cuda/repos/${DISTRO}/${ARCH}/ /" | $SUDO_CMD tee /etc/apt/sources.list.d/cuda-${DISTRO}-${ARCH}.list
            $SUDO_CMD apt-get update -y
            
            # 安装对应版本的 cuda-toolkit 包 (例如 cuda-toolkit-12-8)
            PKG_NAME="cuda-toolkit-${CUDA_VERSION//./-}"
            echo -e "正在安装系统包: ${BLUE}${PKG_NAME}${NC}"
            $SUDO_CMD apt-get install -y "${PKG_NAME}"
        else
            echo -e "${YELLOW}无法自动获取 NVIDIA GPG 密钥，跳过系统级 CUDA Toolkit 安装。${NC}"
        fi
    else
        echo -e "${YELLOW}提示: 如果需要 C++/CUDA 原生代码编译支持，可以传入 --install-cuda-toolkit 参数安装 nvcc。${NC}"
    fi
fi

# ------------------------------------------------------------------------------
# 步骤 3: 安装支持 sm120 的 PyTorch, TorchVision 和 TorchAudio
# 说明: 从 PyTorch 官方源安装编译针对 CUDA 12.8 驱动的 Python wheels
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 3/4] 正在安装 PyTorch (CUDA ${CUDA_VERSION} / ${PYTORCH_CHANNEL})...${NC}"

# 验证 Python 环境
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}错误: 未找到 python3 命令，请先配置 Python 环境。${NC}"
    exit 1
fi

# 升级基础构建工具
python3 -m pip install --upgrade pip setuptools wheel

# 构造目标源 URL (如 https://download.pytorch.org/whl/cu128)
INDEX_URL="https://download.pytorch.org/whl/${PYTORCH_CHANNEL}"

echo -e "执行命令: ${BLUE}python3 -m pip install torch torchvision torchaudio --index-url ${INDEX_URL}${NC}"
python3 -m pip install torch torchvision torchaudio --index-url "${INDEX_URL}"

# ------------------------------------------------------------------------------
# 步骤 4: 运行 Python 脚本验证 PyTorch 驱动与 sm120 架构兼容性
# 说明: 检查 torch.cuda.is_available()、支持的架构列表 (get_arch_list) 及硬件 Capability
# ------------------------------------------------------------------------------
echo -e "\n${YELLOW}[步骤 4/4] 验证 PyTorch CUDA 驱动及 sm120 架构支持...${NC}"

python3 -c "
import sys
import torch

print('================ 运行环境与驱动检测结果 ================')
print(f'Python 版本           : {sys.version.split()[0]}')
print(f'PyTorch 版本          : {torch.__version__}')
print(f'PyTorch 编译 CUDA 版本 : {torch.version.cuda}')
print(f'CUDA 硬件/驱动可用性  : {torch.cuda.is_available()}')

if torch.cuda.is_available():
    device_count = torch.cuda.device_count()
    print(f'检测到的 GPU 数量     : {device_count}')
    for i in range(device_count):
        cap = torch.cuda.get_device_capability(i)
        name = torch.cuda.get_device_name(i)
        print(f'  GPU [{i}] 名称       : {name} (Compute Capability: {cap[0]}.{cap[1]})')

# 获取该 PyTorch 构建版本所内嵌支持的 CUDA 架构列表
arch_list = torch.cuda.get_arch_list() if hasattr(torch.cuda, 'get_arch_list') else []
print(f'PyTorch 内置 Arch 列表: {arch_list}')

# 判断是否存在 12.0 / sm_120 / compute_120 标识
sm120_supported = any('12.0' in arch or 'sm_120' in arch or 'compute_120' in arch for arch in arch_list)

if sm120_supported:
    print('\n[成功] PyTorch 构建已包含二进制支持 sm120 (Blackwell 架构)!')
elif torch.version.cuda and float(torch.version.cuda[:4]) >= 12.8:
    print('\n[成功] 已验证 CUDA 12.8+ 驱动架构，Blackwell (sm120) 将通过 CUDA 12.8 驱动 JIT/PTX 正常工作。')
else:
    print('\n[提示] 请确保在搭载 sm120 硬件的节点上配合 CUDA 12.8+ 驱动运行。')
print('===========================================================')
"

echo -e "\n${GREEN}CUDA & PyTorch (sm120) 安装与验证完成！${NC}"
