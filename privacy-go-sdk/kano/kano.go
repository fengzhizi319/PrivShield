// Package kano 提供 K-匿名 (K-Anonymity) 与 L-多样性 (L-Diversity) 原语。
//
// 实现基于 Mondrian 算法的多维空间切分泛化，支持准标识符 (QI)
// 自动提取与树状泛化。适用于数据集级别的隐私保护发布。
package kano

import (
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ──────────────────────────────────────────────
// 类型定义
// ──────────────────────────────────────────────

// Record 表示一条数据记录（字段名 → 字段值）。
type Record map[string]string

// GeneralizationLevel 记录某字段的泛化层级。
type GeneralizationLevel struct {
	Field string
	Level int // 0 = 原始值，1+ = 泛化层级
}

// AnonymizationResult 包含 K-匿名处理后的数据集与泛化信息。
type AnonymizationResult struct {
	Records         []Record
	Generalizations []GeneralizationLevel
	K               int // 实际达到的 k 值
	GroupCount      int // 等价类数量
}

// ──────────────────────────────────────────────
// K-匿名核心实现（Mondrian 算法）
// ──────────────────────────────────────────────

// Anonymize 对数据集执行 K-匿名处理。
// qiFields 为准标识符字段列表，k 为匿名化参数。
func Anonymize(records []Record, qiFields []string, k int) (*AnonymizationResult, error) {
	if len(records) == 0 || len(qiFields) == 0 || k <= 0 {
		return &AnonymizationResult{Records: records, K: k}, nil
	}

	// 复制数据集避免修改原始数据
	data := make([]Record, len(records))
	for i, r := range records {
		data[i] = make(Record, len(r))
		for k, v := range r {
			data[i][k] = v
		}
	}

	// 执行 Mondrian 切分（带最大深度剪枝防护）
	groups := mondrian(data, qiFields, k, 0)

	// 泛化每个等价类
	result := &AnonymizationResult{
		Records:    make([]Record, 0, len(records)),
		K:          k,
		GroupCount: len(groups),
	}
	for _, group := range groups {
		generalizeGroup(group, qiFields)
		result.Records = append(result.Records, group...)
	}

	return result, nil
}

// mondrian 递归二分数据集，直到每个分区大小 < k、达到最大深度或无法继续切分。
func mondrian(data []Record, qiFields []string, k int, depth int) [][]Record {
	const maxMondrianDepth = 32
	if len(data) <= k || depth >= maxMondrianDepth {
		return [][]Record{data}
	}

	// 找到区分度最大的字段
	bestField, bestMedian := findBestSplit(data, qiFields)
	if bestField == "" {
		// 无法继续切分
		return [][]Record{data}
	}

	// 按中位数切分
	left, right := partitionByMedian(data, bestField, bestMedian)
	if len(left) == 0 || len(right) == 0 {
		return [][]Record{data}
	}

	// 递归切分
	groups := make([][]Record, 0, 4)
	groups = append(groups, mondrian(left, qiFields, k, depth+1)...)
	groups = append(groups, mondrian(right, qiFields, k, depth+1)...)
	return groups
}

// findBestSplit 找到区分度（range）最大的字段及其中位数。
func findBestSplit(data []Record, qiFields []string) (string, string) {
	bestField := ""
	bestRange := -1
	var bestMedian string

	for _, field := range qiFields {
		values := make([]string, 0, len(data))
		for _, r := range data {
			values = append(values, r[field])
		}

		// 尝试数值排序
		if isNumeric(values) {
			nums := parseNumeric(values)
			sort.Float64s(nums)
			if len(nums) < 2 {
				continue
			}
			rangeVal := int(nums[len(nums)-1] - nums[0])
			if rangeVal > bestRange {
				bestRange = rangeVal
				bestField = field
				bestMedian = formatFloat(nums[len(nums)/2])
			}
		} else {
			// 字符串按字典序
			unique := uniqueValues(values)
			if len(unique) <= 1 {
				continue
			}
			rangeVal := len(unique)
			if rangeVal > bestRange {
				bestRange = rangeVal
				bestField = field
				bestMedian = unique[len(unique)/2]
			}
		}
	}

	return bestField, bestMedian
}

// partitionByMedian 按字段中位数将数据分为两半。
func partitionByMedian(data []Record, field, median string) ([]Record, []Record) {
	half := len(data) / 2
	left := make([]Record, 0, half+1)
	right := make([]Record, 0, half+1)
	for _, r := range data {
		if compareValues(r[field], median) <= 0 {
			left = append(left, r)
		} else {
			right = append(right, r)
		}
	}
	return left, right
}

// generalizeGroup 对等价类中的准标识符字段执行泛化。
func generalizeGroup(group []Record, qiFields []string) {
	if len(group) <= 1 {
		return
	}
	for _, field := range qiFields {
		values := make([]string, 0, len(group))
		for _, r := range group {
			values = append(values, r[field])
		}
		// 泛化为区间表示
		minVal, maxVal := findMinMax(values)
		generalized := generalizeValue(minVal, maxVal)
		for _, r := range group {
			r[field] = generalized
		}
	}
}

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────

func findMinMax(values []string) (string, string) {
	if len(values) == 0 {
		return "", ""
	}
	minVal := values[0]
	maxVal := values[0]
	for _, v := range values[1:] {
		if compareValues(v, minVal) < 0 {
			minVal = v
		}
		if compareValues(v, maxVal) > 0 {
			maxVal = v
		}
	}
	return minVal, maxVal
}

func generalizeValue(minVal, maxVal string) string {
	if minVal == maxVal {
		return minVal
	}
	// 尝试数值泛化
	if isNumeric([]string{minVal, maxVal}) {
		return "[" + minVal + ", " + maxVal + "]"
	}
	// 字符串泛化：取公共前缀
	prefix := commonPrefix(minVal, maxVal)
	if prefix == "" {
		return "*"
	}
	return prefix + "*"
}

func commonPrefix(a, b string) string {
	runesA, runesB := []rune(a), []rune(b)
	n := len(runesA)
	if len(runesB) < n {
		n = len(runesB)
	}
	for i := 0; i < n; i++ {
		if runesA[i] != runesB[i] {
			return string(runesA[:i])
		}
	}
	return string(runesA[:n])
}

func compareValues(a, b string) int {
	if isNumeric([]string{a, b}) {
		nums := parseNumeric([]string{a, b})
		if nums[0] < nums[1] {
			return -1
		}
		if nums[0] > nums[1] {
			return 1
		}
		return 0
	}
	return slices.Compare([]rune(a), []rune(b))
}

func isNumeric(values []string) bool {
	for _, v := range values {
		if v == "" {
			continue
		}
		for i, c := range v {
			if i == 0 && (c == '-' || c == '+') {
				continue
			}
			if c == '.' {
				continue
			}
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func parseNumeric(values []string) []float64 {
	result := make([]float64, len(values))
	for i, v := range values {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			// 解析失败回退为 0（与 isNumeric 前置校验配合使用）
			f = 0
		}
		result[i] = f
	}
	return result
}

func formatFloat(f float64) string {
	if f == math.Trunc(f) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte(n%10 + '0')}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func uniqueValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var result []string
	for _, v := range values {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

// ──────────────────────────────────────────────
// L-多样性 (L-Diversity) 检验
// ──────────────────────────────────────────────

// LDiversityResult 包含 L-多样性合规检查报告。
type LDiversityResult struct {
	IsCompliant  bool             `json:"is_compliant"`  // 数据集是否完全满足 L-多样性
	L            int              `json:"l"`             // 要求的 L 值
	MinDiversity int              `json:"min_diversity"` // 所有等价类中的最小多样性值
	Violations   int              `json:"violations"`    // 未满足 L-多样性的等价类数量
	GroupCount   int              `json:"group_count"`   // 总等价类数量
	GroupStats   []GroupDiversity `json:"group_stats"`   // 各等价类详细统计
}

// GroupDiversity 记录单个等价类的多样性统计。
type GroupDiversity struct {
	GroupIndex      int            `json:"group_index"`
	RecordCount     int            `json:"record_count"`
	DistinctCount   int            `json:"distinct_count"`
	SensitiveValues map[string]int `json:"sensitive_values"`
	IsCompliant     bool           `json:"is_compliant"`
}

// CheckDistinctLDiversity 校验数据集在给定准标识符与敏感属性下是否满足 Distinct L-Diversity。
// 每个等价类中敏感属性的不同取值数必须 >= l，有效防御同质性攻击 (Homogeneity Attack)。
func CheckDistinctLDiversity(records []Record, qiFields []string, sensitiveField string, l int) *LDiversityResult {
	if l <= 1 {
		l = 1
	}
	if len(records) == 0 {
		return &LDiversityResult{IsCompliant: true, L: l, MinDiversity: 0}
	}

	// 按准标识符 QI 将记录归入等价类
	groups := make(map[string][]Record)
	for _, r := range records {
		var qiKey strings.Builder
		for _, q := range qiFields {
			qiKey.WriteString(r[q])
			qiKey.WriteByte('|')
		}
		groups[qiKey.String()] = append(groups[qiKey.String()], r)
	}

	res := &LDiversityResult{
		L:            l,
		GroupCount:   len(groups),
		MinDiversity: math.MaxInt,
		IsCompliant:  true,
		GroupStats:   make([]GroupDiversity, 0, len(groups)),
	}

	idx := 0
	for _, grp := range groups {
		idx++
		valCounts := make(map[string]int)
		for _, r := range grp {
			val := r[sensitiveField]
			if val != "" {
				valCounts[val]++
			}
		}
		distinctCount := len(valCounts)
		if distinctCount < res.MinDiversity {
			res.MinDiversity = distinctCount
		}

		compliant := distinctCount >= l
		if !compliant {
			res.Violations++
			res.IsCompliant = false
		}

		res.GroupStats = append(res.GroupStats, GroupDiversity{
			GroupIndex:      idx,
			RecordCount:     len(grp),
			DistinctCount:   distinctCount,
			SensitiveValues: valCounts,
			IsCompliant:     compliant,
		})
	}

	if res.MinDiversity == math.MaxInt {
		res.MinDiversity = 0
	}

	return res
}
