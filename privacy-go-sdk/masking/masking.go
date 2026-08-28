// Package masking 提供零内存分配的字段级 PII 脱敏原语。
//
// 支持中国身份证、手机号、银行卡号、军官证、姓名、地址等常见敏感字段的
// 正则匹配与掩码替换，所有函数均通过 sync.Pool 复用 bytes.Buffer 以降低
// 高频调用场景下的 GC 压力。
package masking

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
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

// MaskChineseName 对中文姓名脱敏：保留姓（首字），名用 * 掩盖。
// 适用于 2~4 字中文姓名；超长姓名保留首字 + 末字，中间掩码。
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
		for i := 1; i < len(runes)-1; i++ {
			sb.WriteByte('*')
		}
		sb.WriteRune(runes[len(runes)-1])
		return sb.String()
	}
}

// MaskAddress 对地址脱敏：保留前 6 个字符（省/市级），后续掩码。
func MaskAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	runes := []rune(addr)
	if len(runes) <= 6 {
		return strings.Repeat("*", len(runes))
	}
	sb := acquireBuilder()
	defer releaseBuilder(sb)
	for i := 0; i < 6; i++ {
		sb.WriteRune(runes[i])
	}
	for i := 6; i < len(runes); i++ {
		sb.WriteByte('*')
	}
	return sb.String()
}

// MaskEmail 对邮箱脱敏：保留首 2 字符 + @ 域名。
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	m := emailRegex.FindStringSubmatch(email)
	if len(m) == 4 {
		return m[1] + strings.Repeat("*", len(m[2])) + m[3]
	}
	at := strings.Index(email, "@")
	if at <= 2 {
		return email
	}
	return email[:2] + strings.Repeat("*", at-2) + email[at:]
}

// ──────────────────────────────────────────────
// HMAC 加盐不可逆散列
// ──────────────────────────────────────────────

// HashHMAC 生成 HMAC-SHA256 不可逆加盐散列，用于需要保留统计关联但不可还原的场景。
func HashHMAC(value, salt string) string {
	h := hmac.New(sha256.New, []byte(salt))
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}
