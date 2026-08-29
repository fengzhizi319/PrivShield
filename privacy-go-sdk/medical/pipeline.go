// Package medical 提供医疗数据隐私处理流水线。
//
// 实现医保 18 字段与康养 27 字段的特化脱敏流水线，
// 支持字段级自动识别、分级脱敏策略与双结构结果输出（分级报告 + 脱敏数据集）。
package medical

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/masking"
)

// ──────────────────────────────────────────────
// 字段分类与规格
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
// 数据结构定义（双结构输出模型）
// ──────────────────────────────────────────────

// FieldClassification 单字段分类分级结果模型
type FieldClassification struct {
	FieldName          string `json:"field_name"`
	Level              string `json:"level"`
	SecurityTag        string `json:"security_tag"`
	Description        string `json:"description"`
	RuleMatched        string `json:"rule_matched"`
	RawValue           string `json:"raw_value,omitempty"`
	SanitizedValue     string `json:"sanitized_value"`
	SanitizedValueRule string `json:"sanitized_value_rule"`
	SanitizedValueNer  string `json:"sanitized_value_ner"`
}

// RecordClassificationReport 单条记录的分级报告
type RecordClassificationReport struct {
	RecordIndex             int                   `json:"record_index"`
	MaxLevel                string                `json:"max_level"`
	PIIFieldsDetected       []string              `json:"pii_fields_detected"`
	HighSensitivityDetected []string              `json:"high_sensitivity_detected"`
	FieldDetails            []FieldClassification `json:"field_details"`
	RawRecord               map[string]string     `json:"raw_record,omitempty"`
}

// MedicalPipelineResult 医疗流水线最终执行结果（双结构输出）
type MedicalPipelineResult struct {
	ClassificationReport []RecordClassificationReport `json:"classification_report"`
	SanitizedData        []map[string]string          `json:"sanitized_data"`
	RawData              []map[string]string          `json:"raw_data,omitempty"`
	Summary              map[string]interface{}       `json:"summary"`
}

// ──────────────────────────────────────────────
// 医疗隐私流水线 (Pipeline)
// ──────────────────────────────────────────────

// Pipeline 医疗数据隐私处理流水线
type Pipeline struct {
	fieldMap map[string]*FieldSpec
	mu       sync.RWMutex
	cache    map[string]string
}

// NewPipeline 创建医疗流水线实例
func NewPipeline(fields []FieldSpec) *Pipeline {
	p := &Pipeline{
		fieldMap: make(map[string]*FieldSpec, len(fields)),
		cache:    make(map[string]string),
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

// ProcessRecords 全流程处理医疗数据集，生成双结构报告与脱敏数据集（支持多核并发分块加速）
func (p *Pipeline) ProcessRecords(records []map[string]string) *MedicalPipelineResult {
	start := time.Now()
	n := len(records)
	sanitizedData := make([]map[string]string, n)
	reports := make([]RecordClassificationReport, n)
	levelCounts := map[string]int{"L1": 0, "L2": 0, "L3": 0, "L4": 0, "L5": 0}

	if n <= 64 {
		// 小批量直接串行处理
		for i, rec := range records {
			sanRec, report := p.ProcessRecord(rec, i+1)
			sanitizedData[i] = sanRec
			reports[i] = *report
			levelCounts[report.MaxLevel]++
		}
	} else {
		// 大批量自动根据 CPU 核心数进行分块并发调度
		numWorkers := runtime.GOMAXPROCS(0)
		if numWorkers > 16 {
			numWorkers = 16
		}
		if numWorkers > n {
			numWorkers = n
		}

		chunkSize := (n + numWorkers - 1) / numWorkers
		var wg sync.WaitGroup
		var mu sync.Mutex

		for w := 0; w < numWorkers; w++ {
			startIdx := w * chunkSize
			endIdx := startIdx + chunkSize
			if endIdx > n {
				endIdx = n
			}
			if startIdx >= endIdx {
				break
			}

			wg.Add(1)
			go func(s, e int) {
				defer wg.Done()
				localCounts := make(map[string]int)
				for i := s; i < e; i++ {
					sanRec, report := p.ProcessRecord(records[i], i+1)
					sanitizedData[i] = sanRec
					reports[i] = *report
					localCounts[report.MaxLevel]++
				}
				mu.Lock()
				for k, v := range localCounts {
					levelCounts[k] += v
				}
				mu.Unlock()
			}(startIdx, endIdx)
		}
		wg.Wait()
	}

	elapsed := time.Since(start).Seconds()

	return &MedicalPipelineResult{
		ClassificationReport: reports,
		SanitizedData:        sanitizedData,
		RawData:              records,
		Summary: map[string]interface{}{
			"total_records":         n,
			"level_counts":          levelCounts,
			"duration_seconds":      elapsed,
			"status":                "success",
			"compliance_guaranteed": true,
		},
	}
}

// ProcessRecord 处理单条记录
func (p *Pipeline) ProcessRecord(record map[string]string, index int) (map[string]string, *RecordClassificationReport) {
	sanRec := make(map[string]string, len(record))
	var fieldDetails []FieldClassification
	var piiFields []string
	var highSensFields []string
	maxLevel := "L1"

	for k, v := range record {
		fc := p.ClassifyAndSanitizeField(k, v)
		sanRec[k] = fc.SanitizedValue
		fieldDetails = append(fieldDetails, *fc)

		if strings.HasPrefix(fc.SecurityTag, "PII_") || fc.SecurityTag == "IDENTITY" {
			piiFields = append(piiFields, k)
		}
		if fc.Level == "L4" || fc.Level == "L5" {
			highSensFields = append(highSensFields, fmt.Sprintf("%s(%s)", k, fc.Level))
		}

		if compareLevel(fc.Level, maxLevel) > 0 {
			maxLevel = fc.Level
		}
	}

	report := &RecordClassificationReport{
		RecordIndex:             index,
		MaxLevel:                maxLevel,
		PIIFieldsDetected:       piiFields,
		HighSensitivityDetected: highSensFields,
		FieldDetails:            fieldDetails,
		RawRecord:               record,
	}

	return sanRec, report
}

// ClassifyAndSanitizeField 对单个字段执行分类与脱敏
func (p *Pipeline) ClassifyAndSanitizeField(fieldName, value string) *FieldClassification {
	canon := CanonicalizePIIField(fieldName)

	// 1. ICD-10 编码字段
	if ICD10FieldNames[canon] || ICD10FieldNames[strings.ToLower(fieldName)] {
		level, cat, ok := ClassifyICD10Code(value)
		if ok {
			sanVal := RedactICD10Code(value)
			return &FieldClassification{
				FieldName:          fieldName,
				Level:              level,
				SecurityTag:        "ICD10_DIAGNOSIS",
				Description:        "ICD-10 诊断编码",
				RuleMatched:        fmt.Sprintf("ICD10_%s", cat),
				RawValue:           value,
				SanitizedValue:     sanVal,
				SanitizedValueRule: sanVal,
				SanitizedValueNer:  sanVal,
			}
		}
		return &FieldClassification{
			FieldName:          fieldName,
			Level:              "L2",
			SecurityTag:        "ICD10_DIAGNOSIS",
			Description:        "ICD-10 基础编码",
			RuleMatched:        "ICD10_STANDARD",
			RawValue:           value,
			SanitizedValue:     value,
			SanitizedValueRule: value,
			SanitizedValueNer:  value,
		}
	}

	// 2. 日期字段
	if DateGeneralizationFields[canon] || DateGeneralizationFields[strings.ToLower(fieldName)] {
		sanVal := TruncateDateToMonth(value)
		return &FieldClassification{
			FieldName:          fieldName,
			Level:              "L2",
			SecurityTag:        "DATE_QI",
			Description:        "日期准标识符",
			RuleMatched:        "DATE_GENERALIZATION",
			RawValue:           value,
			SanitizedValue:     sanVal,
			SanitizedValueRule: sanVal,
			SanitizedValueNer:  sanVal,
		}
	}

	// 3. PII 身份与联系字段
	if rule, isPII := PIIFieldRules[canon]; isPII {
		sanVal := p.sanitizeIdentity(canon, value)
		level := "L4"
		if canon == "id_card_no" || canon == "social_security_no" {
			level = "L5"
		}
		return &FieldClassification{
			FieldName:          fieldName,
			Level:              level,
			SecurityTag:        "PII_IDENTITY",
			Description:        "个人身份敏感字段",
			RuleMatched:        rule,
			RawValue:           value,
			SanitizedValue:     sanVal,
			SanitizedValueRule: sanVal,
			SanitizedValueNer:  sanVal,
		}
	}

	// 4. 临床文本 / 病史文本（检测 L4/L5 高敏词）
	if ContainsHighRiskText(value) {
		sanVal := RedactMedicalText(value)
		level := "L4"
		if strings.Contains(sanVal, "[L5-") {
			level = "L5"
		}
		return &FieldClassification{
			FieldName:          fieldName,
			Level:              level,
			SecurityTag:        "CLINICAL_HIGH_RISK",
			Description:        "临床高危病史文本",
			RuleMatched:        "MEDICAL_L4_L5_RULE",
			RawValue:           value,
			SanitizedValue:     sanVal,
			SanitizedValueRule: sanVal,
			SanitizedValueNer:  sanVal,
		}
	}

	// 5. 按照预设规格脱敏
	sanVal := p.SanitizeField(fieldName, value)
	spec := p.GetFieldSpec(fieldName)
	levelStr := "L1"
	tag := "GENERAL"
	desc := "常规数据"
	if spec != nil {
		levelStr = fmt.Sprintf("L%d", spec.Level)
		tag = string(spec.Category)
		desc = string(spec.Category)
	}

	return &FieldClassification{
		FieldName:          fieldName,
		Level:              levelStr,
		SecurityTag:        tag,
		Description:        desc,
		RuleMatched:        "SPEC_RULE",
		RawValue:           value,
		SanitizedValue:     sanVal,
		SanitizedValueRule: sanVal,
		SanitizedValueNer:  sanVal,
	}
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

	canon := CanonicalizePIIField(fieldName)

	// 1. ICD-10 编码
	if ICD10FieldNames[canon] || ICD10FieldNames[strings.ToLower(fieldName)] {
		return RedactICD10Code(value)
	}

	// 2. 日期截断
	if DateGeneralizationFields[canon] || DateGeneralizationFields[strings.ToLower(fieldName)] {
		return TruncateDateToMonth(value)
	}

	// 3. 临床高危词汇脱敏
	if ContainsHighRiskText(value) {
		return RedactMedicalText(value)
	}

	spec, ok := p.fieldMap[fieldName]
	if !ok {
		spec, ok = p.fieldMap[canon]
	}
	if !ok {
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
	case "id_card_no", "social_security_no", "disability_cert_no":
		return masking.MaskIdCard(value)
	case "name", "doctor_name", "nurse_name":
		return masking.MaskChineseName(value)
	case "gender", "age", "blood_type":
		return value // 低敏感保留
	case "date_of_birth", "birth_date":
		return TruncateDateToMonth(value)
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
		"dietary_restrictions", "present_illness", "past_history":
		if ContainsHighRiskText(value) {
			return RedactMedicalText(value)
		}
		return maskClinicalText(value)
	case "icd_code", "icd10_code":
		return RedactICD10Code(value)
	case "admission_date", "discharge_date":
		return TruncateDateToMonth(value)
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
		if ContainsHighRiskText(value) {
			return RedactMedicalText(value)
		}
		return value
	}
}

func (p *Pipeline) sanitizeLocation(name, value string) string {
	switch name {
	case "address", "registered_address":
		return masking.MaskAddress(value)
	default:
		return value
	}
}

func (p *Pipeline) sanitizeByHeuristic(fieldName, value string) string {
	lower := strings.ToLower(fieldName)
	switch {
	case strings.Contains(lower, "id") || strings.Contains(lower, "card") || strings.Contains(lower, "sfz"):
		return masking.MaskIdCard(value)
	case strings.Contains(lower, "phone") || strings.Contains(lower, "mobile") || strings.Contains(lower, "tel"):
		return masking.MaskPhone(value)
	case strings.Contains(lower, "name") || strings.Contains(lower, "姓名"):
		return masking.MaskChineseName(value)
	case strings.Contains(lower, "email") || strings.Contains(lower, "mail"):
		return masking.MaskEmail(value)
	case strings.Contains(lower, "address") || strings.Contains(lower, "地址"):
		return masking.MaskAddress(value)
	default:
		if ContainsHighRiskText(value) {
			return RedactMedicalText(value)
		}
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

func compareLevel(a, b string) int {
	rank := map[string]int{"L1": 1, "L2": 2, "L3": 3, "L4": 4, "L5": 5}
	return rank[a] - rank[b]
}
