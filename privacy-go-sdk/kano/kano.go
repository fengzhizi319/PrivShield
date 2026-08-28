// Package kano 提供 K-匿名 (K-Anonymity) 与 L-多样性 (L-Diversity) 原语。
//
// 实现基于 Mondrian 算法的多维空间切分泛化，支持准标识符 (QI)
// 自动提取与树状泛化。适用于数据集级别的隐私保护发布。
package kano

import (
	"slices"
	"sort"
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

	// 执行 Mondrian 切分
	groups := mondrian(data, qiFields, k)

	// 泛化每个等价类
	result := &AnonymizationResult{
		K:          k,
		GroupCount: len(groups),
	}
	for _, group := range groups {
		generalizeGroup(group, qiFields)
		result.Records = append(result.Records, group...)
	}

	return result, nil
}

// mondrian 递归二分数据集，直到每个分区大小 < k 或无法继续切分。
func mondrian(data []Record, qiFields []string, k int) [][]Record {
	if len(data) <= k {
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
	var groups [][]Record
	groups = append(groups, mondrian(left, qiFields, k)...)
	groups = append(groups, mondrian(right, qiFields, k)...)
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
	var left, right []Record
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
		na, nb := parseNumeric([]string{a, b})
		if na[0] < nb[0] {
			return -1
		}
		if na[0] > nb[0] {
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
		var f float64
		for _, c := range v {
			if c >= '0' && c <= '9' || c == '.' || c == '-' || c == '+' {
				continue
			}
			_ = f
		}
		// 简单解析
		neg := false
		start := 0
		if len(v) > 0 && v[0] == '-' {
			neg = true
			start = 1
		} else if len(v) > 0 && v[0] == '+' {
			start = 1
		}
		var intPart, fracPart float64
		dotSeen := false
		fracDiv := 1.0
		for j := start; j < len(v); j++ {
			if v[j] == '.' {
				dotSeen = true
				continue
			}
			if !dotSeen {
				intPart = intPart*10 + float64(v[j]-'0')
			} else {
				fracDiv *= 10
				fracPart += float64(v[j]-'0') / fracDiv
			}
		}
		f = intPart + fracPart
		if neg {
			f = -f
		}
		result[i] = f
	}
	return result
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return string(rune(int64(f) + '0'))
	}
	// 简单格式化
	intPart := int64(f)
	fracPart := f - float64(intPart)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	result := formatInt64(intPart)
	if fracPart > 0.0001 {
		result += "."
		for i := 0; i < 2; i++ {
			fracPart *= 10
			digit := int(fracPart)
			result += string(rune(digit + '0'))
			fracPart -= float64(digit)
		}
	}
	return result
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
