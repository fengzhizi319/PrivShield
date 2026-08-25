#!/usr/bin/env bash
# ==============================================================================
# 脚本名称: clean_privacy_budget_db.sh
# 脚本说明: 差分隐私 (Differential Privacy) 预算数据库巡检、清理与重置运维工具。
#
# 执行步骤总览：
#   1. 解析命令行参数（--db、--namespace、--info-only、--reset-all）
#   2. 检查 SQLite 隐私预算数据库文件是否存在
#   3. 查询并格式化打印各 Namespace 的 ε/δ 已消耗配额明细
#   4. 若指定 --info-only 则查询完毕后退出
#   5. 根据参数执行单 Namespace 清空、或全局重置并执行 SQLite VACUUM 空间回收
#
# 用法 / Usage:
#   ./scripts/dev/clean_privacy_budget_db.sh [选项]
# ==============================================================================

set -euo pipefail

# ANSI 终端颜色代码
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ── 步骤 1：定位默认数据库路径与运行模式 ──────────────────────────────────
BUDGET_DB="${PRIVACY_BUDGET_DB:-privacy_budget.db}"
TARGET_NAMESPACE=""
INFO_ONLY=false
RESET_ALL=false

# ── 步骤 2：帮助说明与命令行解析 ──────────────────────────────────────────
usage() {
    cat <<EOF
使用说明: $(basename "$0") [选项]

选项:
  -d, --db PATH            SQLite 预算数据库路径 (默认: privacy_budget.db 或 PRIVACY_BUDGET_DB)
  -n, --namespace NAME     指定需要重置预算的 Namespace
  --info-only              仅查询打印各 Namespace 的当前已用预算，不执行清理
  --reset-all              完全重置所有 Namespace 的预算并清空/压缩数据库
  -h, --help               显示本帮助信息并退出

使用示例:
  # 仅查看预算消耗统计
  ./scripts/dev/clean_privacy_budget_db.sh --info-only

  # 重置 'default' 命名空间的预算
  ./scripts/dev/clean_privacy_budget_db.sh --namespace default

  # 重置所有预算
  ./scripts/dev/clean_privacy_budget_db.sh --reset-all
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -d|--db)
            BUDGET_DB="$2"
            shift 2
            ;;
        -n|--namespace)
            TARGET_NAMESPACE="$2"
            shift 2
            ;;
        --info-only)
            INFO_ONLY=true
            shift
            ;;
        --reset-all)
            RESET_ALL=true
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# 若相对路径在当前目录不存在，尝试回退到项目根目录
if [[ ! -f "$BUDGET_DB" && -f "$PROJECT_ROOT/$BUDGET_DB" ]]; then
    BUDGET_DB="$PROJECT_ROOT/$BUDGET_DB"
fi

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE} 差分隐私预算数据库运维工具${NC}"
echo -e "${BLUE} 数据库文件路径: ${BUDGET_DB}${NC}"
echo -e "${BLUE}====================================================${NC}"

if [ ! -f "$BUDGET_DB" ]; then
    echo -e "${YELLOW}提示: 未找到数据库文件 ${BUDGET_DB}。系统当前可能运行在内存预算模式。${NC}"
    exit 0
fi

# 1. 打印当前数据库中的预算消耗列表
echo -e "\n${YELLOW}[1/2] 正在查询隐私预算消耗表...${NC}"
python3 -c "
import sqlite3
import sys

db_path = '${BUDGET_DB}'
try:
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    # 查询是否存在预算表
    cursor.execute(\"SELECT name FROM sqlite_master WHERE type='table' AND name='privacy_budget';\")
    if not cursor.fetchone():
        print('数据库中尚未创建 privacy_budget 表。')
        sys.exit(0)
        
    cursor.execute('SELECT namespace, epsilon_used, delta_used, updated_at FROM privacy_budget;')
    rows = cursor.fetchall()
    
    print('----------------------------------------------------------------------')
    print(f'{\"Namespace\":<20} | {\"已用 Epsilon (ε)\":<18} | {\"已用 Delta (δ)\":<14} | {\"最后更新时间\"}')
    print('----------------------------------------------------------------------')
    for row in rows:
        ns, eps, delt, up_at = row
        print(f'{ns:<20} | {eps:<18.4f} | {delt:<14.6f} | {up_at}')
    print('----------------------------------------------------------------------')
    conn.close()
except Exception as e:
    print('查询 SQLite 数据库出错:', e)
"

if [ "$INFO_ONLY" = true ]; then
    exit 0
fi

# 2. 执行清理与重置逻辑
echo -e "\n${YELLOW}[2/2] 执行预算重置...${NC}"

python3 -c "
import sqlite3
import sys

db_path = '${BUDGET_DB}'
target_ns = '${TARGET_NAMESPACE}'
reset_all = ${RESET_ALL}

if not target_ns and not reset_all:
    print('[提示] 未指定 --namespace 或 --reset-all，跳过清理。如需重置请传参。')
    sys.exit(0)

try:
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    
    if reset_all:
        print('[*] 正在清空所有 Namespace 预算...')
        cursor.execute('DELETE FROM privacy_budget;')
        conn.commit()
        cursor.execute('VACUUM;')
        print('[+] 数据库已成功全量重置并执行 VACUUM 压缩！')
    elif target_ns:
        print(f'[*] 正在重置 Namespace [{target_ns}] 的预算...')
        cursor.execute('UPDATE privacy_budget SET epsilon_used = 0.0, delta_used = 0.0 WHERE namespace = ?;', (target_ns,))
        conn.commit()
        print(f'[+] Namespace [{target_ns}] 预算已重置为 0.0！')
        
    conn.close()
except Exception as e:
    print('[-] 执行重置失败:', e)
    sys.exit(1)
"

echo -e "\n${GREEN}预算管理操作完成！${NC}"
