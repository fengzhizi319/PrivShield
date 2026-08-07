// Package fileparse parses uploaded CSV/JSON data files into a unified records + schema structure.
// Package fileparse 把上传的 CSV/JSON 数据文件解析为统一的 records + schema 结构。
//
// The Go backend's /api/upload endpoint receives files from the frontend, then uses this
// package to parse them into []map[string]string (each record, values unified as strings)
// and []string (column name order), for further construction of gRPC RecordEntry messages
// (whose Fields field is map[string]string).
// 控制台 Go 后端的 /api/upload 端点收到前端上传的文件后，用本包解析为
// []map[string]string（每条记录，值统一为字符串）与 []string（列名顺序），
// 以便进一步构造 gRPC 的 RecordEntry（其 Fields 即 map[string]string）。
// Values are unified to strings to stay consistent with the agent's records API semantics.
// 值统一转字符串是为了与 agent 的 records 接口语义保持一致。
package fileparse

import (
	// bytes：用于从 []byte 创建 io.Reader 供 csv.Reader 读取
	// bytes: creates io.Reader from []byte for csv.Reader consumption
	"bytes"
	// encoding/csv：标准库 CSV 解析器，支持 RFC 4180 格式
	// encoding/csv: stdlib CSV parser, supports RFC 4180 format
	"encoding/csv"
	// encoding/json：用于解析 JSON 记录数组与序列化嵌套对象
	// encoding/json: parses JSON record arrays and serializes nested objects
	"encoding/json"
	// fmt：用于格式化错误信息
	// fmt: formats error messages
	"fmt"
	// sort：用于对 JSON 解析后的 schema 列名排序，保证结果确定性
	// sort: sorts JSON-parsed schema column names for deterministic output
	"sort"
	// strconv：用于将 float64/bool 类型转换为字符串表示
	// strconv: converts float64/bool types to string representation
	"strconv"
)

// ParseCSV parses CSV bytes into records and schema.
// ParseCSV 把 CSV 字节解析为 records 与 schema。
//
// The first row is treated as the header (schema); remaining rows are mapped
// to records by header column names. Rows with fewer fields than the header
// are padded with empty strings, allowing inconsistent field counts per row.
// 首行视为表头（schema），其余行按表头列名映射为 record；
// 某行字段数不足时以空字符串补齐，允许各行字段数不一致。
//
// Parameters / 参数：
//   - data: raw CSV file content bytes / 原始 CSV 文件内容字节
//
// Returns / 返回：
//   - []map[string]string: parsed records (column→value) / 解析后的记录
//   - []string: schema (ordered column names) / 列名顺序
//   - error: parse failure / 解析失败错误
func ParseCSV(data []byte) ([]map[string]string, []string, error) {
	// 剥离 UTF-8 BOM（若有），否则首列表头会带 \ufeff 前缀
	// Strip UTF-8 BOM (if present) so the first header key is not polluted
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	// 从字节切片创建 CSV 读取器
	// Create CSV reader from byte slice
	reader := csv.NewReader(bytes.NewReader(data))
	// 允许各行字段数与表头不一致，缺失字段以空串补齐。
	// Allow rows to have different field counts; missing fields padded with empty string.
	reader.FieldsPerRecord = -1
	// 一次性读取所有行（包含表头）
	// Read all rows at once (including header)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	// 空文件检查：至少需要一行表头
	// Empty file check: at least one header row required
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("CSV 文件为空")
	}

	// 首行为表头，作为 schema（列名顺序）
	// First row is the header, used as schema (column name order)
	schema := rows[0]
	// 预分配 records 切片，容量为数据行数（总行数 - 1 行表头）
	// Pre-allocate records slice with capacity = data rows (total - 1 header)
	records := make([]map[string]string, 0, len(rows)-1)
	// 遍历数据行（跳过表头），逐行构建 record map
	// Iterate data rows (skip header), build record map per row
	for _, row := range rows[1:] {
		// 每行创建一个 map，容量为 schema 列数
		// Create a map per row, capacity = schema column count
		record := make(map[string]string, len(schema))
		// 按 schema 列名逐列映射值
		// Map values column-by-column according to schema names
		for i, col := range schema {
			if i < len(row) {
				// 字段存在：直接映射
				// Field exists: map directly
				record[col] = row[i]
			} else {
				// 字段缺失：以空字符串补齐
				// Field missing: pad with empty string
				record[col] = ""
			}
		}
		records = append(records, record)
	}
	return records, schema, nil
}

// ParseJSON parses a JSON record array (list of objects) into records and schema.
// ParseJSON 把 JSON 记录数组（list of objects）解析为 records 与 schema。
//
// Schema collects all keys appearing across all records, sorted alphabetically
// to ensure deterministic output (Go map iteration is unordered).
// schema 取所有记录中出现过的键并按字母序排序，保证结果确定（Go map 遍历无序）；
// Each value is uniformly converted to string (numbers, booleans, null, nested
// objects all have corresponding handling).
// 每个值统一转换为字符串（数字、布尔、null、嵌套对象等均有对应处理）。
//
// Parameters / 参数：
//   - data: raw JSON file content (must be an array of objects)
//     原始 JSON 文件内容（必须为对象数组）
//
// Returns / 返回：
//   - []map[string]string: parsed records / 解析后的记录
//   - []string: sorted schema (all unique keys) / 排序后的列名
//   - error: parse failure / 解析失败错误
func ParseJSON(data []byte) ([]map[string]string, []string, error) {
	// 解析 JSON 为通用 map 数组（值类型为 any）
	// Parse JSON into generic map array (value type is any)
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("JSON 解析失败（需为记录数组）: %w", err)
	}

	// 收集所有记录中出现过的键名，用于构建 schema
	// Collect all keys appearing across records to build schema
	seen := make(map[string]bool)
	for _, obj := range raw {
		for k := range obj {
			seen[k] = true // 标记该键名已出现 / mark key as seen
		}
	}
	// 将 map 键转为切片并排序，保证输出确定性
	// Convert map keys to slice and sort for deterministic output
	schema := make([]string, 0, len(seen))
	for k := range seen {
		schema = append(schema, k)
	}
	sort.Strings(schema) // 字母序排序 / alphabetical sort

	// 遍历每条记录，将所有值统一转为字符串
	// Iterate each record, convert all values to strings uniformly
	records := make([]map[string]string, 0, len(raw))
	for _, obj := range raw {
		record := make(map[string]string, len(obj))
		for k, v := range obj {
			// 调用 toString 将任意 JSON 值转为字符串
			// Call toString to convert any JSON value to string
			record[k] = toString(v)
		}
		records = append(records, record)
	}
	return records, schema, nil
}

// toString converts any JSON value to its string representation.
// toString 把任意 JSON 值统一转换为字符串表示。
//
// Conversion rules / 转换规则：
//   - string → returned as-is / 原样返回
//   - float64 → formatted without trailing zeros (e.g. "3.14", "42")
//     格式化为无尾随零的字符串
//   - bool → "true" or "false"
//   - nil → empty string ""
//   - nested object/array → compact JSON serialization
//     嵌套对象/数组 → 紧凑 JSON 序列化
func toString(v any) string {
	switch t := v.(type) {
	case string:
		// 字符串类型直接返回
		// String type returned directly
		return t
	case float64:
		// JSON 数字默认解码为 float64，用 'f' 格式避免科学计数法
		// JSON numbers decode as float64; use 'f' format to avoid scientific notation
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		// 布尔值转为 "true"/"false" 字符串
		// Boolean converted to "true"/"false" string
		return strconv.FormatBool(t)
	case nil:
		// null 值转为空字符串
		// null value converted to empty string
		return ""
	default:
		// 嵌套对象 / 数组：序列化为紧凑 JSON 字符串。
		// Nested objects/arrays: serialize to compact JSON string.
		b, _ := json.Marshal(t)
		return string(b)
	}
}
