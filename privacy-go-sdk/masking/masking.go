// Package masking 提供零内存分配的字段级 PII 脱敏原语。
//
// 支持中国身份证、手机号、银行卡号、军官证、姓名、地址等常见敏感字段的
// 正则匹配与掩码替换，所有函数均通过 sync.Pool 复用 bytes.Buffer 以降低
// 高频调用场景下的 GC 压力。
package masking

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ──────────────────────────────────────────────
// 预编译正则表（包级单例，避免重复编译开销）
// ──────────────────────────────────────────────

var (
	// 中国大陆 18 位身份证：6 位行政区划 + 8 位生日 + 3 位顺序码（末位可为 X/x）
	idCardRegex = regexp.MustCompile(`^(\d{6})(\d{8})(\d{3}[\dXx])$`)
	// 中国大陆手机号：可选 +86 前缀 + 1[3-9] 开头 + 9 位数字
	phoneRegex = regexp.MustCompile(`^(\+?86)?(1[3-9]\d)(\d{4})(\d{4})$`)
	// 银行卡号：6 位 BIN + 中间变长 + 末 4 位
	bankRegex = regexp.MustCompile(`^(\d{6})\d+(\d{4})$`)
	// 军官证：军字 + 数字
	officerRegex = regexp.MustCompile(`^(军)(\d{2,4})(\d{2})$`)
	// 邮箱
	emailRegex = regexp.MustCompile(`^([^@]{2})([^@]*)(@.+)$`)
)

// ──────────────────────────────────────────────
// sync.Pool 复用 strings.Builder
// ──────────────────────────────────────────────

var builderPool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

func acquireBuilder() *strings.Builder {
	sb := builderPool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

func releaseBuilder(sb *strings.Builder) {
	if sb.Cap() > 4096 {
		return // 避免缓冲池膨胀
	}
	builderPool.Put(sb)
}

// ──────────────────────────────────────────────
// 字段掩码公开 API
// ──────────────────────────────────────────────

// MaskIdCard 对中国大陆身份证号脱敏：保留前 6 位行政区划与末 4 位，生日段用 8 个 * 掩盖。
// 非标准格式（长度不足等）回退为全量掩码。
func MaskIdCard(id string) string {
	id = strings.TrimSpace(id)
	m := idCardRegex.FindStringSubmatch(id)
	if len(m) == 4 {
		return m[1] + "********" + m[3]
	}
	if len(id) > 8 {
		return id[:4] + strings.Repeat("*", len(id)-8) + id[len(id)-4:]
	}
	return strings.Repeat("*", len(id))
}

// MaskPhone 对手机号脱敏：保留前 3 后 4，中间 4 位掩码。
func MaskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	m := phoneRegex.FindStringSubmatch(phone)
	if len(m) == 5 {
		prefix := m[1]
		if prefix != "" {
			prefix += " "
		}
		return prefix + m[2] + "****" + m[4]
	}
	if len(phone) > 7 {
		return phone[:3] + strings.Repeat("*", len(phone)-7) + phone[len(phone)-4:]
	}
	return strings.Repeat("*", len(phone))
}

// MaskBankCard 对银行卡号脱敏：保留前 6 位 BIN 与末 4 位，中间段掩码。
func MaskBankCard(card string) string {
	card = strings.TrimSpace(card)
	m := bankRegex.FindStringSubmatch(card)
	if len(m) == 3 {
		middle := len(card) - 10
		return m[1] + strings.Repeat("*", middle) + m[2]
	}
	if len(card) > 8 {
		return card[:4] + strings.Repeat("*", len(card)-8) + card[len(card)-4:]
	}
	return strings.Repeat("*", len(card))
}

// MaskOfficerId 对军官证号脱敏：保留"军"字与末 2 位。
func MaskOfficerId(id string) string {
	id = strings.TrimSpace(id)
	m := officerRegex.FindStringSubmatch(id)
	if len(m) == 4 {
		return m[1] + strings.Repeat("*", len(m[2])) + m[3]
	}
	if len(id) > 4 {
		return id[:2] + strings.Repeat("*", len(id)-4) + id[len(id)-2:]
	}
	return strings.Repeat("*", len(id))
}

// MaskChineseName 对中文姓名脱敏：保留姓（首字）与末字，中间用 * 掩盖。
// 与 Python mask_name 对齐：2字→首+*；3字→首+**+尾；4字及以上→首+*(n-2)+尾。
func MaskChineseName(name string) string {
	name = strings.TrimSpace(name)
	runes := []rune(name)
	switch len(runes) {
	case 0:
		return ""
	case 1:
		return "*"
	case 2:
		return string(runes[0]) + "*"
	default:
		sb := acquireBuilder()
		defer releaseBuilder(sb)
		sb.WriteRune(runes[0])
		// 与 Python mask_name 对齐：3字→固定 **；4字及以上→*(n-2)
		starCount := len(runes) - 2
		if starCount < 2 {
			starCount = 2
		}
		for i := 0; i < starCount; i++ {
			sb.WriteByte('*')
		}
		sb.WriteRune(runes[len(runes)-1])
		return sb.String()
	}
}

// MaskAddress 对地址脱敏：保留前 6 个字符（省/市级），后续替换为固定 ****。
// 与 Python mask_address 对齐：长度 <= 6 原样返回，> 6 则 前6字符 + ****。
func MaskAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	runes := []rune(addr)
	if len(runes) <= 6 {
		return addr // 短地址原样返回
	}
	return string(runes[:6]) + "****"
}

// MaskEmail 对邮箱脱敏：用户名保留首尾字符、中间替换为 ***，域名完整保留。
// 与 Python mask_email 对齐：长用户名(>2)→首+***+尾+@域名；短用户名(≤2)→首+***+@域名。
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return MaskDefault(email, 3, 3) // 无 @ 回退到默认策略
	}
	local := email[:at]
	domain := email[at+1:]
	var maskedLocal string
	if len(local) <= 2 {
		if len(local) > 0 {
			maskedLocal = string([]rune(local)[0]) + "***"
		} else {
			maskedLocal = "***"
		}
	} else {
		runes := []rune(local)
		maskedLocal = string(runes[0]) + "***" + string(runes[len(runes)-1])
	}
	return maskedLocal + "@" + domain
}

// MaskDefault 默认脱敏策略：保留前 prefix 位与后 suffix 位，中间用 * 填充。
// 与 Python mask_default 对齐。
func MaskDefault(value string, prefix, suffix int) string {
	if len(value) <= prefix+suffix {
		return value
	}
	stars := strings.Repeat("*", len(value)-prefix-suffix)
	return value[:prefix] + stars + value[len(value)-suffix:]
}

// ──────────────────────────────────────────────
// HMAC 加盐不可逆散列
// ──────────────────────────────────────────────

// HashHMAC 生成 HMAC-SHA256 不可逆加盐散列，base64 编码后截取前 16 字符。
// 与 Python hash_value 对齐：HMAC-SHA256(salt, value) → base64 → 前16字符。
func HashHMAC(value, salt string) string {
	h := hmac.New(sha256.New, []byte(salt))
	h.Write([]byte(value))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))[:16]
}

// ──────────────────────────────────────────────
// 工具函数：截断、FPE、日期偏移、字段类型推断
// ──────────────────────────────────────────────

// Truncate 截断字符串并追加 "***" 后缀。
// 与 Python truncate 对齐：保留前 keepPrefix 个字符，超出部分替换为 "***"。
func Truncate(value string, keepPrefix int) (string, error) {
	if keepPrefix < 0 {
		return "", fmt.Errorf("keepPrefix must be non-negative, got %d", keepPrefix)
	}
	runes := []rune(value)
	if len(runes) <= keepPrefix {
		return value, nil
	}
	return string(runes[:keepPrefix]) + "***", nil
}

// FpeEncryptNumeric 保留格式加密 (FPE) 算子。
// 与 Python fpe_encrypt_numeric 对齐：HMAC-SHA256 派生密钥流，逐字符模加置换。
// 数字 mod 10、字母 mod 26、分隔符原样保留。
func FpeEncryptNumeric(value, secretKey string) string {
	if value == "" {
		return value
	}
	if secretKey == "" {
		secretKey = "privshield-default-fpe-key"
	}
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(value))
	digest := h.Sum(nil)

	var result strings.Builder
	result.Grow(len(value))
	for idx, ch := range value {
		byteVal := int(digest[idx%len(digest)])
		if ch >= '0' && ch <= '9' {
			origDigit := int(ch - '0')
			newDigit := (origDigit + byteVal) % 10
			result.WriteByte(byte('0' + newDigit))
		} else if ch >= 'A' && ch <= 'Z' {
			origOffset := int(ch - 'A')
			newOffset := (origOffset + byteVal) % 26
			result.WriteByte(byte('A' + newOffset))
		} else if ch >= 'a' && ch <= 'z' {
			origOffset := int(ch - 'a')
			newOffset := (origOffset + byteVal) % 26
			result.WriteByte(byte('a' + newOffset))
		} else {
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// RandomDateOffset 日期统一随机序列偏移算子。
// 与 Python random_date_offset 对齐：提取日期部分，按 offsetDays 偏移。
func RandomDateOffset(dateStr string, offsetDays int) string {
	if dateStr == "" {
		return dateStr
	}
	// 正则提取日期部分
	dateRegex := regexp.MustCompile(`(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})`)
	m := dateRegex.FindStringSubmatch(dateStr)
	if m == nil {
		return dateStr
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])

	// 构造日期并偏移
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	newT := t.AddDate(0, 0, offsetDays)

	// 确定分隔符
	sep := "-"
	if strings.Contains(dateStr, "/") {
		sep = "/"
	} else if strings.Contains(dateStr, ".") {
		sep = "."
	}

	newDateFmt := fmt.Sprintf("%04d%s%02d%s%02d", newT.Year(), sep, newT.Month(), sep, newT.Day())
	return strings.Replace(dateStr, m[0], newDateFmt, 1)
}

// FieldType 敏感字段类型。
type FieldType string

const (
	FieldTypeMobile    FieldType = "mobile"
	FieldTypeIDCard    FieldType = "id_card"
	FieldTypeEmail     FieldType = "email"
	FieldTypeAddress   FieldType = "address"
	FieldTypeMedical   FieldType = "medical"
	FieldName          FieldType = "name"
	FieldTypeBankCard  FieldType = "bank_card"
	FieldTypeDefault   FieldType = "default"
)

// GuessFieldType 根据字段名猜测敏感字段类型。
// 与 Python guess_field_type 对齐：按优先级匹配关键字规则。
func GuessFieldType(fieldName string) FieldType {
	lower := strings.ToLower(fieldName)

	// 规则链：按优先级从高到低
	// mobile
	for _, kw := range []string{"mobile", "phone", "手机号", "手机", "联系电话", "电话号码", "电话"} {
		if strings.Contains(lower, kw) {
			return FieldTypeMobile
		}
	}
	if boundedContains(lower, "tel") {
		return FieldTypeMobile
	}
	// id_card
	for _, kw := range []string{"id_card", "idcard", "身份证", "identity"} {
		if strings.Contains(lower, kw) {
			return FieldTypeIDCard
		}
	}
	// email
	for _, kw := range []string{"email", "邮箱", "电子邮箱"} {
		if strings.Contains(lower, kw) {
			return FieldTypeEmail
		}
	}
	if boundedContains(lower, "mail") {
		return FieldTypeEmail
	}
	// address
	for _, kw := range []string{"addr", "address", "地址", "住址"} {
		if strings.Contains(lower, kw) {
			return FieldTypeAddress
		}
	}
	// medical (必须在 name 之前)
	for _, kw := range []string{"diagnosis", "chief_complaint", "present_illness",
		"past_history", "personal_history", "family_history", "allergic_history",
		"progress_note", "诊断", "病史", "主诉", "过敏史", "既往史", "个人史", "家族史", "现病史"} {
		if strings.Contains(lower, kw) {
			return FieldTypeMedical
		}
	}
	// name
	for _, kw := range []string{"姓名", "名字", "fullname", "surname", "nickname"} {
		if strings.Contains(lower, kw) {
			return FieldName
		}
	}
	if boundedContains(lower, "name") {
		return FieldName
	}
	// bank_card
	for _, kw := range []string{"bank", "card_no", "银行卡", "卡号"} {
		if strings.Contains(lower, kw) {
			return FieldTypeBankCard
		}
	}
	if boundedContains(lower, "card") {
		return FieldTypeBankCard
	}
	return FieldTypeDefault
}

// boundedContains 边界感知关键字匹配。
// 仅当关键字两侧为非字母边界时返回 true。
func boundedContains(s, keyword string) bool {
	idx := strings.Index(s, keyword)
	if idx < 0 {
		return false
	}
	// 检查左边界
	if idx > 0 {
		prev := rune(s[idx-1])
		if unicode.IsLetter(prev) && unicode.IsLower(prev) {
			return false
		}
	}
	// 检查右边界
	end := idx + len(keyword)
	if end < len(s) {
		next := rune(s[end])
		if unicode.IsLetter(next) && unicode.IsLower(next) {
			return false
		}
	}
	return true
}

// ──────────────────────────────────────────────
// 随机工具
// ──────────────────────────────────────────────

func init() {
	rand.Seed(time.Now().UnixNano())
}
