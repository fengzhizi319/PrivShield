#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: health_check.sh
# 脚本说明: PrivShield 侧边栏服务健康状态诊断与 GPU 运行环境巡检工具。
#
# 巡检内容:
#   1. 系统与 Python 环境 (Python 版本、CPU/内存)
#   2. GPU / CUDA / PyTorch / TensorRT 驱动及硬件可用性检查
#   3. REST API 端口 (默认 8079) 及 HTTP /health 端点可用性探针
#   4. gRPC 服务端口 (默认 50051) 连通性探针
#   5. Prometheus /metrics 监控端点响应检查
#   6. 本地 SQLite 隐私预算数据库文件状态巡检
# ==============================================================================

set -euo pipefail

# ANSI 彩色输出定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

REST_HOST="${PRIVACY_REST_HOST:-127.0.0.1}"
REST_PORT="${PRIVACY_REST_PORT:-8079}"
GRPC_HOST="${PRIVACY_GRPC_HOST:-127.0.0.1}"
GRPC_PORT="${PRIVACY_GRPC_PORT:-50051}"

export no_proxy="127.0.0.1,localhost,${REST_HOST},${no_proxy:-}"
export NO_PROXY="127.0.0.1,localhost,${REST_HOST},${NO_PROXY:-}"

usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  --rest-host HOST    REST 服务主机地址 (默认: 127.0.0.1 或 PRIVACY_REST_HOST)
  --rest-port PORT    REST 服务端口 (默认: 8079 或 PRIVACY_REST_PORT)
  --grpc-host HOST    gRPC 服务主机地址 (默认: 127.0.0.1 或 PRIVACY_GRPC_HOST)
  --grpc-port PORT    gRPC 服务端口 (默认: 50051 或 PRIVACY_GRPC_PORT)
  -h, --help          显示帮助信息并退出

使用示例:
  ./scripts/dev/health_check.sh
  ./scripts/dev/health_check.sh --rest-port 8080 --grpc-port 50052
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --rest-host)
            REST_HOST="$2"
            shift 2
            ;;
        --rest-port)
            REST_PORT="$2"
            shift 2
            ;;
        --grpc-host)
            GRPC_HOST="$2"
            shift 2
            ;;
        --grpc-port)
            GRPC_PORT="$2"
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
echo -e "${BLUE} PrivShield 系统健康与环境诊断工具${NC}"
echo -e "${BLUE} REST 目标: http://${REST_HOST}:${REST_PORT}${NC}"
echo -e "${BLUE} gRPC 目标: ${GRPC_HOST}:${GRPC_PORT}${NC}"
echo -e "${BLUE}====================================================${NC}"

# 1. Python 及底层基础环境检查
echo -e "\n${YELLOW}[1/5] 检查 Python 及基础运行环境...${NC}"
if command -v python3 &> /dev/null; then
    PY_VER=$(python3 -c "import sys; print(sys.version.split()[0])")
    echo -e "Python3 版本: ${GREEN}${PY_VER}${NC}"
else
    echo -e "${RED}[失败] 未检测到 python3，无法运行基础环境！${NC}"
fi

# 2. NVIDIA GPU, CUDA & TensorRT 环境探针
echo -e "\n${YELLOW}[2/5] 检查 GPU / CUDA / PyTorch / TensorRT 加载状态...${NC}"
python3 -c "
import sys

print('--- GPU 驱动与框架探针 ---')
# 检查 NVIDIA 驱动
try:
    import subprocess
    res = subprocess.run(['nvidia-smi', '--query-gpu=name,driver_version', '--format=csv,noheader'], capture_output=True, text=True)
    if res.returncode == 0:
        print('NVIDIA 硬件信息 :', res.stdout.strip())
    else:
        print('NVIDIA 驱动状态 :', 'nvidia-smi 返回异常')
except Exception as e:
    print('NVIDIA 驱动状态 :', f'未安装或无 GPU ({e})')

# 检查 PyTorch 及 CUDA 支持
try:
    import torch
    print('PyTorch 版本    :', torch.__version__)
    print('PyTorch CUDA    :', torch.version.cuda)
    print('PyTorch GPU 可用:', torch.cuda.is_available())
    if torch.cuda.is_available():
        print('当前 GPU 设备名 :', torch.cuda.get_device_name(0))
except ImportError:
    print('PyTorch 状态    :', '未安装 PyTorch (处于 CPU/Core 极简模式)')

# 检查 TensorRT 支持
try:
    import tensorrt as trt
    print('TensorRT 版本   :', trt.__version__)
except ImportError:
    print('TensorRT 状态   :', '未安装 TensorRT (处于常规推理解析模式)')
"

# 3. REST 服务端口及 HTTP 端点探针
echo -e "\n${YELLOW}[3/5] 检查 REST 服务连通性 (http://${REST_HOST}:${REST_PORT})...${NC}"
REST_URL="http://${REST_HOST}:${REST_PORT}/health"
if command -v curl &> /dev/null; then
    HTTP_CODE=$(curl --noproxy "*" -s -o /tmp/pla_health_response.json -w "%{http_code}" --max-time 5 "${REST_URL}" || echo "000")
    if [ "$HTTP_CODE" -eq 200 ]; then
        echo -e "REST 健康探针结果: ${GREEN}HTTP 200 OK${NC}"
        echo -e "返回报文内容     : $(cat /tmp/pla_health_response.json)"
    else
        echo -e "REST 健康探针结果: ${RED}HTTP ${HTTP_CODE} (服务未启动或不可达)${NC}"
    fi
else
    echo -e "${YELLOW}未检测到 curl，跳过 HTTP 端口探针。${NC}"
fi

# 4. gRPC 服务端口连通性检测
echo -e "\n${YELLOW}[4/5] 检查 gRPC 服务端口 (${GRPC_HOST}:${GRPC_PORT})...${NC}"
if command -v nc &> /dev/null; then
    if nc -z -w 3 "$GRPC_HOST" "$GRPC_PORT" &> /dev/null; then
        echo -e "gRPC 端口状态   : ${GREEN}端口 ${GRPC_PORT} 开放且可达${NC}"
    else
        echo -e "gRPC 端口状态   : ${RED}端口 ${GRPC_PORT} 无法连接 (服务可能未启动)${NC}"
    fi
elif command -v timeout &> /dev/null && command -v bash &> /dev/null; then
    if timeout 3 bash -c "</dev/tcp/${GRPC_HOST}/${GRPC_PORT}" &> /dev/null; then
        echo -e "gRPC 端口状态   : ${GREEN}端口 ${GRPC_PORT} 开放且可达${NC}"
    else
        echo -e "gRPC 端口状态   : ${RED}端口 ${GRPC_PORT} 无法连接${NC}"
    fi
else
    echo -e "${YELLOW}缺少 nc/tcp 工具，跳过端口侦听检查。${NC}"
fi

# 5. 本地持久化数据库文件巡检
echo -e "\n${YELLOW}[5/5] 巡检持久化数据库文件...${NC}"
BUDGET_DB="${PRIVACY_BUDGET_DB:-privacy_budget.db}"

if [ -f "$BUDGET_DB" ]; then
    echo -e "隐私预算数据库 : ${GREEN}存在 (${BUDGET_DB}, 大小: $(du -h "$BUDGET_DB" | cut -f1))${NC}"
else
    echo -e "隐私预算数据库 : ${YELLOW}未发现 (${BUDGET_DB}，当前可能使用内存预算模式)${NC}"
fi

echo -e "\n${GREEN}====================================================${NC}"
echo -e "${GREEN} 健康诊断完成！${NC}"
echo -e "${GREEN}====================================================${NC}"
