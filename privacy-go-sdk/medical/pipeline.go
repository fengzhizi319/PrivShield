// Package medical 提供医疗数据隐私处理流水线。
//
// 实现医保 18 字段与康养 27 字段的特化脱敏流水线，
// 支持字段级自动识别与分级脱敏策略。
package medical

import (
	"strings"

	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/masking"
)

// ──────────────────────────────────────────────
// 字段分类
// ──────────────────────────────────────────────

// FieldCategory 字段敏感类别
type FieldCategory string

const (
	CategoryIdentity  FieldCategory = "identity"  // 身份标识
	CategoryContact   FieldCategory = "contact"   // 联系方式
	CategoryFinancial FieldCategory = "financial" // 财务信息
	CategoryMedical   FieldCategory = "medical"   // 医疗记录
	CategoryLocation  FieldCategory = "location"  // 地理位置
	CategoryOther     FieldCategory = "other"     // 其他
)

// FieldSpec 字段规格
type FieldSpec struct {
	Name     string
	Category FieldCategory
	Level    int // 1=公开, 2=内部, 3=机密, 4=秘密, 5=绝密
}

// ──────────────────────────────────────────────
// 医保 18 字段定义
// ──────────────────────────────────────────────

var YibaoFields = []FieldSpec{
	{Name: "name", Category: CategoryIdentity, Level: 4},
	{Name: "id_card_no", Category: CategoryIdentity, Level: 5},
	{Name: "gender", Category: CategoryIdentity, Level: 2},
	{Name: "date_of_birth", Category: CategoryIdentity, Level: 3},
	{Name: "age", Category: CategoryIdentity, Level: 2},
	{Name: "phone", Category: CategoryContact, Level: 4},
	{Name: "address", Category: CategoryLocation, Level: 4},
	{Name: "medical_record_no", Category: CategoryMedical, Level: 4},
	{Name: "social_security_no", Category: CategoryFinancial, Level: 5},
	{Name: "insurance_type", Category: CategoryFinancial, Level: 2},
	{Name: "diagnosis", Category: CategoryMedical, Level: 4},
	{Name: "icd_code", Category: CategoryMedical, Level: 3},
	{Name: "admission_date", Category: CategoryMedical, Level: 2},
	{Name: "discharge_date", Category: CategoryMedical, Level: 2},
	{Name: "total_cost", Category: CategoryFinancial, Level: 3},
	{Name: "reimbursement", Category: CategoryFinancial, Level: 3},
	{Name: "chief_complaint", Category: CategoryMedical, Level: 4},
	{Name: "doctor_name", Category: CategoryIdentity, Level: 3},
}

// ──────────────────────────────────────────────
// 康养 27 字段定义
// ──────────────────────────────────────────────

var KangyangFields = []FieldSpec{
	{Name: "name", Category: CategoryIdentity, Level: 4},
	{Name: "id_card_no", Category: CategoryIdentity, Level: 5},
	{Name: "gender", Category: CategoryIdentity, Level: 2},
	{Name: "date_of_birth", Category: CategoryIdentity, Level: 3},
	{Name: "age", Category: CategoryIdentity, Level: 2},
	{Name: "phone", Category: CategoryContact, Level: 4},
	{Name: "emergency_contact", Category: CategoryContact, Level: 4},
	{Name: "emergency_phone", Category: CategoryContact, Level: 4},
	{Name: "address", Category: CategoryLocation, Level: 4},
	{Name: "health_record_no", Category: CategoryMedical, Level: 4},
	{Name: "blood_type", Category: CategoryMedical, Level: 2},
	{Name: "allergies", Category: CategoryMedical, Level: 3},
	{Name: "chronic_diseases", Category: CategoryMedical, Level: 4},
	{Name: "medication_history", Category: CategoryMedical, Level: 4},
	{Name: "vital_signs", Category: CategoryMedical, Level: 3},
	{Name: "assessment_score", Category: CategoryMedical, Level: 3},
	{Name: "care_level", Category: CategoryMedical, Level: 2},
	{Name: "admission_date", Category: CategoryMedical, Level: 2},
	{Name: "bed_no", Category: CategoryLocation, Level: 3},
	{Name: "room_no", Category: CategoryLocation, Level: 3},
	{Name: "nurse_name", Category: CategoryIdentity, Level: 3},
	{Name: "doctor_name", Category: CategoryIdentity, Level: 3},
	{Name: "family_contact", Category: CategoryContact, Level: 4},
	{Name: "payment_method", Category: CategoryFinancial, Level: 3},
	{Name: "monthly_fee", Category: CategoryFinancial, Level: 3},
	{Name: "dietary_restrictions", Category: CategoryMedical, Level: 3},
	{Name: "special_notes", Category: CategoryMedical, Level: 4},
}

// ──────────────────────────────────────────────
// 医疗隐私流水线
// ──────────────────────────────────────────────

// Pipeline 医疗数据隐私处理流水线
type Pipeline struct {
	fieldMap map[string]*FieldSpec
}

// NewPipeline 创建医疗流水线实例
func NewPipeline(fields []FieldSpec) *Pipeline {
	p := &Pipeline{
		fieldMap: make(map[string]*FieldSpec, len(fields)),
	}
	for i := range fields {
		p.fieldMap[fields[i].Name] = &fields[i]
	}
	return p
}

// NewYibaoPipeline 创建医保 18 字段流水线
func NewYibaoPipeline() *Pipeline {
	return NewPipeline(YibaoFields)
}

// NewKangyangPipeline 创建康养 27 字段流水线
func NewKangyangPipeline() *Pipeline {
	return NewPipeline(KangyangFields)
}

// SanitizeRecord 对整条记录执行脱敏
func (p *Pipeline) SanitizeRecord(record map[string]string) map[string]string {
	result := make(map[string]string, len(record))
	for k, v := range record {
		result[k] = p.SanitizeField(k, v)
	}
	return result
}

// SanitizeBatch 批量脱敏
func (p *Pipeline) SanitizeBatch(records []map[string]string) []map[string]string {
	results := make([]map[string]string, len(records))
	for i, r := range records {
		results[i] = p.SanitizeRecord(r)
	}
	return results
}

// SanitizeField 对单个字段执行脱敏
func (p *Pipeline) SanitizeField(fieldName, value string) string {
	if value == "" {
		return ""
	}

	spec, ok := p.fieldMap[fieldName]
	if !ok {
		// 未知字段：根据值内容启发式匹配
		return p.sanitizeByHeuristic(fieldName, value)
	}

	return p.sanitizeBySpec(spec, value)
}

func (p *Pipeline) sanitizeBySpec(spec *FieldSpec, value string) string {
	switch spec.Category {
	case CategoryIdentity:
		return p.sanitizeIdentity(spec.Name, value)
	case CategoryContact:
		return p.sanitizeContact(spec.Name, value)
	case CategoryFinancial:
		return p.sanitizeFinancial(spec.Name, value)
	case CategoryMedical:
		return p.sanitizeMedical(spec.Name, value)
	case CategoryLocation:
		return p.sanitizeLocation(spec.Name, value)
	default:
		return value
	}
}

func (p *Pipeline) sanitizeIdentity(name, value string) string {
	switch name {
	case "id_card_no", "social_security_no":
		return masking.MaskIdCard(value)
	case "name", "doctor_name", "nurse_name":
		return masking.MaskChineseName(value)
	case "gender", "age", "blood_type":
		return value // 低敏感保留
	case "date_of_birth":
		if len(value) >= 8 {
			return value[:4] + "-**-**"
		}
		return "****-**-**"
	default:
		return masking.MaskChineseName(value)
	}
}

func (p *Pipeline) sanitizeContact(name, value string) string {
	switch name {
	case "phone", "emergency_phone":
		return masking.MaskPhone(value)
	case "emergency_contact", "family_contact":
		return masking.MaskChineseName(value)
	default:
		return masking.MaskPhone(value)
	}
}

func (p *Pipeline) sanitizeFinancial(name, value string) string {
	switch name {
	case "total_cost", "reimbursement", "monthly_fee":
		return value // 数值保留
	case "payment_method", "insurance_type":
		return value // 类别保留
	default:
		return "***"
	}
}

func (p *Pipeline) sanitizeMedical(name, value string) string {
	switch name {
	case "diagnosis", "chief_complaint", "chronic_diseases",
		"medication_history", "allergies", "special_notes",
		"dietary_restrictions":
		// 临床长文本：保留首尾字符，中间掩码
		return maskClinicalText(value)
	case "icd_code":
		return value // ICD 编码保留
	case "admission_date", "discharge_date":
		return value // 日期保留
	case "medical_record_no", "health_record_no":
		if len(value) > 4 {
			return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
		}
		return strings.Repeat("*", len(value))
	case "vital_signs", "assessment_score", "care_level":
		return value // 临床指标保留
	case "bed_no", "room_no":
		return value // 床位号保留
	default:
		return value
	}
}

func (p *Pipeline) sanitizeLocation(name, value string) string {
	switch name {
	case "address":
		return masking.MaskAddress(value)
	default:
		return value
	}
}

func (p *Pipeline) sanitizeByHeuristic(fieldName, value string) string {
	lower := strings.ToLower(fieldName)
	switch {
	case strings.Contains(lower, "id") || strings.Contains(lower, "card"):
		return masking.MaskIdCard(value)
	case strings.Contains(lower, "phone") || strings.Contains(lower, "mobile"):
		return masking.MaskPhone(value)
	case strings.Contains(lower, "name"):
		return masking.MaskChineseName(value)
	case strings.Contains(lower, "email"):
		return masking.MaskEmail(value)
	case strings.Contains(lower, "address"):
		return masking.MaskAddress(value)
	default:
		return value
	}
}

// maskClinicalText 对临床文本脱敏：保留首尾字符
func maskClinicalText(text string) string {
	runes := []rune(text)
	n := len(runes)
	if n <= 2 {
		return strings.Repeat("*", n)
	}
	// 保留前 2 后 2，中间掩码
	kept := 2
	maskLen := n - kept*2
	if maskLen <= 0 {
		return text
	}
	sb := &strings.Builder{}
	sb.WriteString(string(runes[:kept]))
	sb.WriteString(strings.Repeat("*", maskLen))
	sb.WriteString(string(runes[n-kept:]))
	return sb.String()
}

// GetFieldSpec 获取字段规格
func (p *Pipeline) GetFieldSpec(fieldName string) *FieldSpec {
	return p.fieldMap[fieldName]
}

// FieldCount 返回已注册字段数
func (p *Pipeline) FieldCount() int {
	return len(p.fieldMap)
}
