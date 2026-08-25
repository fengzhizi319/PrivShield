#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: health_check.sh
# 脚本说明: PrivShield 核心算力服务、BFF 网关与中台微服务群健康状态诊断与环境巡检工具。
#
# 执行步骤总览：
#   0. 自动定位并切换至项目根目录
#   1. 解析命令行参数（--rest-host、--rest-port、--grpc-host、--grpc-port、--all）
#   2. 检查系统 Python 3 基础运行环境与版本
#   3. 探测 NVIDIA GPU / CUDA / PyTorch / TensorRT 驱动及深度学习框架可用性
#   4. 探测 核心 Agent REST API 端口连通性及 HTTP /health 端点报文响应
#   5. 探测 核心 Agent gRPC 服务端口 TCP 连通性
#   6. 可选探测 BFF 网关与微服务群（service-hub:8082, datasource-mgr:8083, audit-log:8084, bff:8081/8080）
#   7. 巡检本地 SQLite 隐私预算数据库持久化文件状态
#   8. 输出巡检统计汇总与准确退出码
#
# 用法 / Usage:
#   ./scripts/dev/health_check.sh [选项]
# ==============================================================================

set -euo pipefail

# ANSI 彩色输出定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# ── 步骤 0：定位并切换至项目根目录 ────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

# ── 步骤 1：定位默认参数与环境变量 ────────────────────────────────────────
REST_HOST="${PRIVACY_REST_HOST:-127.0.0.1}"
REST_PORT="${PRIVACY_REST_PORT:-8079}"
GRPC_HOST="${PRIVACY_GRPC_HOST:-127.0.0.1}"
GRPC_PORT="${PRIVACY_GRPC_PORT:-50051}"
CHECK_ALL=false

export no_proxy="127.0.0.1,localhost,${REST_HOST},${no_proxy:-}"
export NO_PROXY="127.0.0.1,localhost,${REST_HOST},${NO_PROXY:-}"

TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0
WARNING_CHECKS=0

# ── 步骤 2：帮助说明与命令行解析 ──────────────────────────────────────────
usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  --rest-host HOST    REST 服务主机地址 (默认: 127.0.0.1 或 PRIVACY_REST_HOST)
  --rest-port PORT    REST 服务端口 (默认: 8079 或 PRIVACY_REST_PORT)
  --grpc-host HOST    gRPC 服务主机地址 (默认: 127.0.0.1 或 PRIVACY_GRPC_HOST)
  --grpc-port PORT    gRPC 服务端口 (默认: 50051 或 PRIVACY_GRPC_PORT)
  --all               全面探测中台微服务群 (service-hub, datasource-mgr, audit-log, bff)
  -h, --help          显示帮助信息并退出

使用示例:
  ./scripts/dev/health_check.sh
  ./scripts/dev/health_check.sh --all
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
        --all)
            CHECK_ALL=true
            shift 1
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo -e "${RED}错误: 未知参数 $1${NC}"
            usage
            ;;
    esac
done

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} PrivShield 基础环境与服务健康巡检${NC}"
echo -e "${BLUE} 工作目录: ${PROJECT_ROOT}${NC}"
echo -e "${BLUE}====================================================${NC}"

# 1. 检查 Python 基础运行环境
echo -e "\n${YELLOW}[1/5] 检查 Python 运行环境与核心组件...${NC}"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
if command -v python3 &> /dev/null; then
    PY_VER=$(python3 -c "import sys; print(sys.version.split()[0])")
    echo -e "Python 版本     : ${GREEN}${PY_VER}${NC}"
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
else
    echo -e "${RED}[错误] 未检测到 python3，请先安装 Python 3.10+ 环境！${NC}"
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
fi

# 2. 检查 GPU / CUDA 驱动与推理解析器
echo -e "\n${YELLOW}[2/5] 检查 GPU 算力与推理框架环境...${NC}"
python3 -c "
import sys

# 检查 PyTorch 与 CUDA 算力架构
try:
    import torch
    print('PyTorch 版本    :', torch.__version__)
    if torch.cuda.is_available():
        gpu_name = torch.cuda.get_device_name(0)
        cap = torch.cuda.get_device_capability(0)
        print('CUDA 状态       : 可用 (设备: %s, 算力架构: sm_%d%d)' % (gpu_name, cap[0], cap[1]))
    else:
        print('CUDA 状态       : 未启用 (使用 CPU 模式)')
except ImportError:
    print('PyTorch 状态    : 未安装 PyTorch')

# 检查 ONNX Runtime
try:
    import onnxruntime as ort
    providers = ort.get_available_providers()
    print('ONNX Runtime    :', ort.__version__, '(Providers: %s)' % ', '.join(providers))
except ImportError:
    print('ONNX Runtime    : 未安装')

# 检查 TensorRT 扩展
try:
    import tensorrt as trt
    print('TensorRT 版本   :', trt.__version__)
except ImportError:
    print('TensorRT 状态   :', '未安装 TensorRT (处于常规推理解析模式)')
"

# 3. REST 服务端口及 HTTP 端点探针
echo -e "\n${YELLOW}[3/5] 检查核心 Agent REST 连通性 (http://${REST_HOST}:${REST_PORT})...${NC}"
REST_URL="http://${REST_HOST}:${REST_PORT}/health"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
if command -v curl &> /dev/null; then
    HTTP_CODE=$(curl --noproxy "*" -s -o /tmp/privshield_health_response.json -w "%{http_code}" --max-time 5 "${REST_URL}" 2>/dev/null || echo "000")
    HTTP_CODE="${HTTP_CODE: -3}"
    if [ "$HTTP_CODE" = "200" ]; then
        echo -e "Agent REST 健康探针: ${GREEN}HTTP 200 OK${NC}"
        echo -e "返回报文内容       : $(cat /tmp/privshield_health_response.json 2>/dev/null || true)"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        echo -e "Agent REST 健康探针: ${RED}HTTP ${HTTP_CODE} (服务未启动或不可达)${NC}"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
else
    echo -e "${YELLOW}未检测到 curl，跳过 HTTP 端口探针。${NC}"
    WARNING_CHECKS=$((WARNING_CHECKS + 1))
fi

# 4. gRPC 服务端口连通性检测
echo -e "\n${YELLOW}[4/5] 检查核心 Agent gRPC 端口 (${GRPC_HOST}:${GRPC_PORT})...${NC}"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
if command -v nc &> /dev/null; then
    if nc -z -w 3 "$GRPC_HOST" "$GRPC_PORT" &> /dev/null; then
        echo -e "Agent gRPC 端口状态: ${GREEN}端口 ${GRPC_PORT} 开放且可达${NC}"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        echo -e "Agent gRPC 端口状态: ${RED}端口 ${GRPC_PORT} 无法连接${NC}"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
elif command -v timeout &> /dev/null && command -v bash &> /dev/null; then
    if timeout 3 bash -c "</dev/tcp/${GRPC_HOST}/${GRPC_PORT}" &> /dev/null; then
        echo -e "Agent gRPC 端口状态: ${GREEN}端口 ${GRPC_PORT} 开放且可达${NC}"
        PASSED_CHECKS=$((PASSED_CHECKS + 1))
    else
        echo -e "Agent gRPC 端口状态: ${RED}端口 ${GRPC_PORT} 无法连接${NC}"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
else
    echo -e "${YELLOW}缺少 nc/tcp 工具，跳过端口侦听检查。${NC}"
    WARNING_CHECKS=$((WARNING_CHECKS + 1))
fi

# 可选探测微服务群
if [ "$CHECK_ALL" = true ]; then
    echo -e "\n${YELLOW}[扩展] 巡检中台微服务群与 BFF 网关...${NC}"
    check_http_svc() {
        local name="$1"
        local url="$2"
        local code
        TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
        code=$(curl --noproxy "*" -s -o /dev/null -w "%{http_code}" --max-time 3 "$url" 2>/dev/null || echo "000")
        code="${code: -3}"
        if [ "$code" = "200" ]; then
            echo -e "  • ${name} ($url): ${GREEN}HTTP 200 OK${NC}"
            PASSED_CHECKS=$((PASSED_CHECKS + 1))
        else
            echo -e "  • ${name} ($url): ${RED}HTTP ${code} (未就绪)${NC}"
            FAILED_CHECKS=$((FAILED_CHECKS + 1))
        fi
    }
    check_http_svc "BFF-Go 网关" "http://127.0.0.1:8081/api/health"
    check_http_svc "BFF-Py 网关" "http://127.0.0.1:8080/api/health"
    check_http_svc "Service Hub 调度中枢" "http://127.0.0.1:8082/api/health"
    check_http_svc "Datasource Mgr 数据源" "http://127.0.0.1:8083/api/health"
    check_http_svc "Audit Log 审计日志" "http://127.0.0.1:8084/api/health"
fi

# 5. 本地持久化数据库文件巡检
echo -e "\n${YELLOW}[5/5] 巡检持久化数据库文件...${NC}"
BUDGET_DB="${PRIVACY_BUDGET_DB:-privacy_budget.db}"
if [[ ! -f "$BUDGET_DB" && -f "$PROJECT_ROOT/$BUDGET_DB" ]]; then
    BUDGET_DB="$PROJECT_ROOT/$BUDGET_DB"
fi

if [ -f "$BUDGET_DB" ]; then
    echo -e "隐私预算数据库 : ${GREEN}存在 (${BUDGET_DB}, 大小: $(du -h "$BUDGET_DB" | cut -f1))${NC}"
else
    echo -e "隐私预算数据库 : ${YELLOW}未发现 (${BUDGET_DB}，当前可能使用内存预算模式)${NC}"
fi

# ── 步骤 6：巡检结果汇总报告 ──────────────────────────────────────────────
echo -e "\n${BLUE}====================================================${NC}"
echo -e "${BLUE}               健康诊断结果汇总                     ${NC}"
echo -e "${BLUE}====================================================${NC}"
echo -e "  • 检查总项: ${CYAN}${TOTAL_CHECKS}${NC}"
echo -e "  • 通过项目: ${GREEN}${PASSED_CHECKS}${NC}"
echo -e "  • 警告项目: ${YELLOW}${WARNING_CHECKS}${NC}"
echo -e "  • 失败项目: ${RED}${FAILED_CHECKS}${NC}"
echo -e "${BLUE}----------------------------------------------------${NC}"

if [ "$FAILED_CHECKS" -eq 0 ]; then
    echo -e "${GREEN}✅ 基础服务与运行环境巡检全部通过！${NC}"
    echo -e "${BLUE}====================================================${NC}"
    exit 0
else
    echo -e "${RED}❌ 存在 ${FAILED_CHECKS} 项未通过的检查，请排查相关服务或端口！${NC}"
    echo -e "${BLUE}====================================================${NC}"
    exit 1
fi
