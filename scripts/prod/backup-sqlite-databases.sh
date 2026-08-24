#!/usr/bin/env bash
# ============================================================================
# SQLite Database Backup Script / SQLite 数据库备份脚本
# ============================================================================
#
# 功能：
#   - 备份 service-hub 和 audit-log 的 SQLite 数据库
#   - 支持全量备份和增量备份（基于文件哈希）
#   - 自动清理过期备份（保留最近 N 天）
#   - 支持定时任务（cron）集成
#
# 用法：
#   bash scripts/prod/backup-sqlite-databases.sh [选项]
#
# 选项：
#   --full              全量备份（默认）
#   --incremental       增量备份（仅备份变化的文件）
#   --verify            验证模式：解压最新备份并执行 SQLite 完整性校验
#   --install-cron      安装定时任务（每天凌晨 2 点执行）
#   --help              显示帮助信息
#
# 环境变量：
#   BACKUP_DIR          备份目录（默认：/var/backups/privshield）
#   SERVICE_HUB_DB_PATH service-hub 数据库路径
#   AUDIT_LOG_DB_PATH   audit-log 数据库路径
#   DATASOURCE_MGR_DB_PATH datasource-mgr 数据库路径
#   RETENTION_DAYS      备份保留天数（默认：7）
#   COMPRESS_ENABLED    是否压缩备份（默认：true）
#
# 示例：
#   # 手动执行全量备份
#   bash scripts/prod/backup-sqlite-databases.sh --full
#
#   # 安装定时任务
#   bash scripts/prod/backup-sqlite-databases.sh --install-cron
#
# ============================================================================

set -euo pipefail

# ============================================================================
# Color Output / 颜色输出
# ============================================================================
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC}  $(date '+%Y-%m-%d %H:%M:%S') $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $(date '+%Y-%m-%d %H:%M:%S') $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') $*"; }
log_debug() { echo -e "${BLUE}[DEBUG]${NC} $(date '+%Y-%m-%d %H:%M:%S') $*"; }

# ============================================================================
# Default Configuration / 默认配置
# ============================================================================
BACKUP_DIR="${BACKUP_DIR:-/var/backups/privshield}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
COMPRESS_ENABLED="${COMPRESS_ENABLED:-true}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
HASH_FILE="${BACKUP_DIR}/.db_hashes"

# 解析参数
BACKUP_TYPE="full"
INSTALL_CRON=false
VERIFY_MODE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --full)
            BACKUP_TYPE="full"
            shift
            ;;
        --incremental)
            BACKUP_TYPE="incremental"
            shift
            ;;
        --verify)
            VERIFY_MODE=true
            shift
            ;;
        --install-cron)
            INSTALL_CRON=true
            shift
            ;;
        --help|-h)
            head -n 45 "$0" | tail -n +3
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ============================================================================
# Create Backup Directory / 创建备份目录
# ============================================================================
mkdir -p "${BACKUP_DIR}"

# ============================================================================
# Verify Mode / 验证模式（#5）
# ============================================================================
if [[ "${VERIFY_MODE}" == "true" ]]; then
    log_info "Starting backup verification..."
    VERIFY_PASS=0
    VERIFY_FAIL=0
    VERIFY_TMPDIR=$(mktemp -d)
    trap "rm -rf '${VERIFY_TMPDIR}'" EXIT

    for db_name in service-hub audit-log datasource-mgr; do
        # 找到该数据库最新的备份文件
        latest_backup=$(ls -t "${BACKUP_DIR}"/${db_name}_*.db.gz 2>/dev/null | head -1)
        if [[ -z "${latest_backup}" ]]; then
            latest_backup=$(ls -t "${BACKUP_DIR}"/${db_name}_*.db 2>/dev/null | head -1)
        fi
        if [[ -z "${latest_backup}" ]]; then
            log_warn "${db_name}: no backup found, skipping"
            continue
        fi

        log_info "${db_name}: verifying backup ${latest_backup}"

        # 解压到临时目录
        if [[ "${latest_backup}" == *.gz ]]; then
            gunzip -c "${latest_backup}" > "${VERIFY_TMPDIR}/${db_name}.db"
        else
            cp "${latest_backup}" "${VERIFY_TMPDIR}/${db_name}.db"
        fi

        # 执行 SQLite 完整性校验
        result=$(sqlite3 "${VERIFY_TMPDIR}/${db_name}.db" "PRAGMA integrity_check;" 2>&1)
        if [[ "${result}" == "ok" ]]; then
            log_info "${db_name}: integrity check PASSED"
            ((VERIFY_PASS++))
        else
            log_error "${db_name}: integrity check FAILED: ${result}"
            ((VERIFY_FAIL++))
        fi

        rm -f "${VERIFY_TMPDIR}/${db_name}.db"
    done

    log_info "=========================================="
    log_info "Verification Summary / 验证汇总"
    log_info "=========================================="
    log_info "Passed: ${VERIFY_PASS}"
    log_info "Failed: ${VERIFY_FAIL}"
    log_info "=========================================="

    if [[ ${VERIFY_FAIL} -gt 0 ]]; then
        log_error "Verification FAILED — some backups are corrupted!"
        exit 1
    fi
    log_info "Verification PASSED — all backups are valid"
    exit 0
fi

log_info "Starting ${BACKUP_TYPE} backup..."
log_info "Backup directory: ${BACKUP_DIR}"
log_info "Retention days: ${RETENTION_DAYS}"
log_info "Compress enabled: ${COMPRESS_ENABLED}"

# ============================================================================
# Hash Management / 哈希管理（用于增量备份）
# ============================================================================
load_hash() {
    local db_name="$1"
    if [[ -f "${HASH_FILE}" ]]; then
        grep "^${db_name}=" "${HASH_FILE}" 2>/dev/null | cut -d'=' -f2 || echo ""
    else
        echo ""
    fi
}

save_hash() {
    local db_name="$1"
    local hash="$2"
    
    # 创建或更新哈希文件
    if [[ -f "${HASH_FILE}" ]]; then
        # 删除旧记录
        grep -v "^${db_name}=" "${HASH_FILE}" > "${HASH_FILE}.tmp" 2>/dev/null || true
        mv "${HASH_FILE}.tmp" "${HASH_FILE}"
    fi
    echo "${db_name}=${hash}" >> "${HASH_FILE}"
}

compute_hash() {
    local file_path="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${file_path}" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${file_path}" | awk '{print $1}'
    else
        # Fallback: use file modification time and size
        stat -f "%m%z" "${file_path}" 2>/dev/null || stat -c "%Y%s" "${file_path}" 2>/dev/null
    fi
}

# ============================================================================
# Backup Function / 备份函数
# ============================================================================
backup_database() {
    local db_path="$1"
    local db_name="$2"
    
    if [[ -z "${db_path}" ]]; then
        log_warn "${db_name} database path not configured, skipping"
        return 0
    fi
    
    if [[ ! -f "${db_path}" ]]; then
        log_warn "${db_name} database file not found: ${db_path}"
        return 0
    fi
    
    # 增量备份：检查文件是否变化
    if [[ "${BACKUP_TYPE}" == "incremental" ]]; then
        local current_hash
        current_hash=$(compute_hash "${db_path}")
        local previous_hash
        previous_hash=$(load_hash "${db_name}")
        
        if [[ "${current_hash}" == "${previous_hash}" ]]; then
            log_info "${db_name} database unchanged, skipping backup"
            return 0
        fi
        
        save_hash "${db_name}" "${current_hash}"
    fi
    
    local backup_file="${BACKUP_DIR}/${db_name}_${TIMESTAMP}.db"
    
    # 使用 SQLite 的 .backup 命令进行在线备份（不锁库）
    if sqlite3 "${db_path}" ".backup '${backup_file}'" 2>/dev/null; then
        local backup_size
        backup_size=$(stat -f%z "${backup_file}" 2>/dev/null || stat -c%s "${backup_file}" 2>/dev/null)
        log_info "${db_name} backup completed: ${backup_file} (${backup_size} bytes)"
        
        # 压缩备份文件
        if [[ "${COMPRESS_ENABLED}" == "true" ]] && command -v gzip >/dev/null 2>&1; then
            gzip "${backup_file}"
            log_info "${db_name} backup compressed: ${backup_file}.gz"
        fi
    else
        log_error "${db_name} backup failed"
        return 1
    fi
}

# ============================================================================
# Execute Backups / 执行备份
# ============================================================================
BACKUP_SUCCESS=0
BACKUP_FAILED=0

# 备份 service-hub 数据库
if backup_database "${SERVICE_HUB_DB_PATH:-}" "service-hub"; then
    ((BACKUP_SUCCESS++))
else
    ((BACKUP_FAILED++))
fi

# 备份 audit-log 数据库
if backup_database "${AUDIT_LOG_DB_PATH:-}" "audit-log"; then
    ((BACKUP_SUCCESS++))
else
    ((BACKUP_FAILED++))
fi

# 备份 datasource-mgr 数据库
if backup_database "${DATASOURCE_MGR_DB_PATH:-}" "datasource-mgr"; then
    ((BACKUP_SUCCESS++))
else
    ((BACKUP_FAILED++))
fi

# ============================================================================
# Cleanup Old Backups / 清理过期备份
# ============================================================================
log_info "Cleaning up backups older than ${RETENTION_DAYS} days..."
DELETED_COUNT=$(find "${BACKUP_DIR}" -type f -name "*.db*" -mtime "+${RETENTION_DAYS}" -delete -print | wc -l)
log_info "Deleted ${DELETED_COUNT} old backup files"

# ============================================================================
# Summary / 汇总
# ============================================================================
BACKUP_COUNT=$(find "${BACKUP_DIR}" -type f -name "*.db*" | wc -l)
BACKUP_SIZE=$(du -sh "${BACKUP_DIR}" 2>/dev/null | awk '{print $1}')

log_info "=========================================="
log_info "Backup Summary / 备份汇总"
log_info "=========================================="
log_info "Backup type: ${BACKUP_TYPE}"
log_info "Successful: ${BACKUP_SUCCESS}"
log_info "Failed: ${BACKUP_FAILED}"
log_info "Total backups: ${BACKUP_COUNT}"
log_info "Total size: ${BACKUP_SIZE}"
log_info "=========================================="

if [[ ${BACKUP_FAILED} -gt 0 ]]; then
    log_error "Some backups failed!"
    exit 1
fi

# ============================================================================
# Cron Installation / 安装定时任务
# ============================================================================
if [[ "${INSTALL_CRON}" == "true" ]]; then
    SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
    CRON_JOB="0 2 * * * ${SCRIPT_PATH} --full >> ${BACKUP_DIR}/backup.log 2>&1"
    
    # 检查是否已存在相同的 cron 任务
    if crontab -l 2>/dev/null | grep -qF "${SCRIPT_PATH}"; then
        log_warn "Cron job already exists for this script"
    else
        (crontab -l 2>/dev/null; echo "${CRON_JOB}") | crontab -
        log_info "Cron job installed: ${CRON_JOB}"
    fi
fi

exit 0
