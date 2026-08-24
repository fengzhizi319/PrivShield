// Package handlers provides CSV loading, auto-classification and mock seed datasource utilities.
package handlers

import (
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fengzhizi319/PrivShield/console/pkg/store"
)

// SeedMockDataSources initializes the two mock CSV data sources (yibao.csv & kangyang.csv).
// SeedMockDataSources 自动注入医保与康养两个模拟 CSV 数据源。
func SeedMockDataSources(dsStore store.DataSourceStore, logger *slog.Logger) error {
	now := time.Now()

	seedSources := []*store.DataSource{
		{
			ID:            "ds_yibao",
			Name:          "医保就医与结算模拟数据库 (yibao.csv)",
			Type:          "file",
			Host:          "127.0.0.1",
			Port:          8083,
			Database:      "yibao.csv",
			SecurityLevel: "high",
			Status:        "connected",
			CreatedAt:     now,
			LastCheckAt:   &now,
			Tags:          []string{"医保", "门诊住院", "结算流水", "敏感数据", "政务流通"},
		},
		{
			ID:            "ds_kangyang",
			Name:          "康养体检与慢病模拟数据库 (kangyang.csv)",
			Type:          "file",
			Host:          "127.0.0.1",
			Port:          8083,
			Database:      "kangyang.csv",
			SecurityLevel: "high",
			Status:        "connected",
			CreatedAt:     now,
			LastCheckAt:   &now,
			Tags:          []string{"康养", "慢病随访", "体检报告", "残疾评估", "健康档案"},
		},
	}

	for _, seed := range seedSources {
		existing, err := dsStore.GetDS(seed.ID)
		if err == nil && existing != nil {
			// Already exists, skip
			continue
		}
		if err := dsStore.SaveDS(seed); err != nil {
			logger.Warn("failed to seed mock datasource", "id", seed.ID, "error", err.Error())
			return err
		}
		logger.Info("successfully seeded mock datasource", "id", seed.ID, "name", seed.Name, "file", seed.Database)
	}
	return nil
}

// candidateDirs lists possible directories where samples or data files may reside.
var candidateDirs = []string{
	"samples",
	"services/datasource-mgr/samples",
	"data",
	"../../data",
	"../../services/datasource-mgr/samples",
	"console/bff-go/internal/samples",
	"../../console/bff-go/internal/samples",
	"../bff-go/internal/samples",
}

// findCSVFile attempts to locate a CSV file safely by extracting the base filename and checking known allowed candidate paths.
// 安全查找 CSV 文件，严格限制 .csv 扩展名并仅在预定义白名单目录中搜寻，防止路径穿越（Path Traversal / LFI）。
func findCSVFile(filename string) (string, error) {
	cleanName := filepath.Clean(filename)
	// 强制要求 .csv 扩展名
	if !strings.EqualFold(filepath.Ext(cleanName), ".csv") {
		return "", fmt.Errorf("invalid file format: only .csv files are allowed (got %s)", filename)
	}

	// 提取纯文件名，杜绝 ../ 路径穿越
	baseName := filepath.Base(cleanName)
	if baseName == "." || baseName == "/" || baseName == "" {
		return "", fmt.Errorf("invalid csv filename: %s", filename)
	}

	for _, dir := range candidateDirs {
		candidate := filepath.Join(dir, baseName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	// Search upward through parent directories (handles go test run from subdirectories)
	if curr, err := os.Getwd(); err == nil {
		for i := 0; i < 6; i++ {
			for _, sub := range []string{"samples", "services/datasource-mgr/samples", "data", "console/bff-go/internal/samples"} {
				cand := filepath.Join(curr, sub, baseName)
				if info, err := os.Stat(cand); err == nil && !info.IsDir() {
					return cand, nil
				}
			}
			parent := filepath.Dir(curr)
			if parent == curr {
				break
			}
			curr = parent
		}
	}

	return "", fmt.Errorf("csv file not found in allowed sample/data directories: %s", baseName)
}

// LoadCSVRecords reads a CSV file and parses records with limit and offset.
// 限制最大读取行数（50,000 行），防止过大文件导致内存耗尽（DoS 防护）。
func LoadCSVRecords(filename string, limit, offset int) ([]map[string]any, int, error) {
	filePath, err := findCSVFile(filename)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open csv file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable fields if any

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("read csv header: %w", err)
	}

	const maxRowsToRead = 50000
	var allRows []map[string]any
	for len(allRows) < maxRowsToRead {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed rows
		}

		rowMap := make(map[string]any, len(header))
		for i, col := range header {
			colName := strings.TrimSpace(col)
			if i < len(record) {
				val := strings.TrimSpace(record[i])
				// Try parse int or float
				if intVal, err := strconv.ParseInt(val, 10, 64); err == nil {
					rowMap[colName] = intVal
				} else if floatVal, err := strconv.ParseFloat(val, 64); err == nil && strings.Contains(val, ".") {
					rowMap[colName] = floatVal
				} else {
					rowMap[colName] = val
				}
			} else {
				rowMap[colName] = ""
			}
		}
		allRows = append(allRows, rowMap)
	}

	total := len(allRows)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []map[string]any{}, total, nil
	}

	end := offset + limit
	if end > total || limit <= 0 {
		end = total
	}

	return allRows[offset:end], total, nil
}

// FieldMetadata describes a single column's metadata and sensitivity.
type FieldMetadata struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	SecurityLevel  string `json:"security_level"`
	Classification string `json:"classification"`
	Sensitive      bool   `json:"sensitive"`
}

// TableMetadata describes the schema of a data table.
type TableMetadata struct {
	Name     string          `json:"name"`
	RowCount int             `json:"row_count"`
	Fields   []FieldMetadata `json:"fields"`
}

// ExtractCSVMetadata analyzes a CSV file's header and first rows to produce structured table metadata.
func ExtractCSVMetadata(tableName, filename string) (*TableMetadata, error) {
	filePath, err := findCSVFile(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open csv file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	var firstRow []string
	rowCount := 0
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err == nil {
			if firstRow == nil {
				firstRow = rec
			}
			rowCount++
		}
	}

	fields := make([]FieldMetadata, 0, len(header))
	for i, col := range header {
		colName := strings.TrimSpace(col)
		colLower := strings.ToLower(colName)
		sampleVal := ""
		if i < len(firstRow) {
			sampleVal = strings.TrimSpace(firstRow[i])
		}

		// Infer type
		colType := "string"
		if _, err := strconv.ParseInt(sampleVal, 10, 64); err == nil && sampleVal != "" {
			colType = "integer"
		} else if _, err := strconv.ParseFloat(sampleVal, 64); err == nil && strings.Contains(sampleVal, ".") {
			colType = "float"
		}

		// Infer sensitivity & classification level
		level := "L1"
		classification := "general"
		sensitive := false

		switch {
		case strings.Contains(colLower, "id_card") || strings.Contains(colLower, "idcard") || strings.Contains(colLower, "sfz"):
			level = "L4"
			classification = "PII_IDCard"
			sensitive = true
		case strings.Contains(colLower, "phone") || strings.Contains(colLower, "mobile") || strings.Contains(colLower, "tel"):
			level = "L3"
			classification = "PII_Phone"
			sensitive = true
		case strings.Contains(colLower, "name") || strings.Contains(colLower, "patient") || strings.Contains(colLower, "elder"):
			level = "L3"
			classification = "PII_Name"
			sensitive = true
		case strings.Contains(colLower, "address") || strings.Contains(colLower, "addr"):
			level = "L2"
			classification = "PII_Address"
			sensitive = true
		case strings.Contains(colLower, "diagnosis") || strings.Contains(colLower, "disease") || strings.Contains(colLower, "icd10") || strings.Contains(colLower, "illness"):
			level = "L4"
			classification = "Medical_Diagnosis"
			sensitive = true
		case strings.Contains(colLower, "insurance") || strings.Contains(colLower, "yibao") || strings.Contains(colLower, "settlement") || strings.Contains(colLower, "cert_no"):
			level = "L3"
			classification = "Medical_Insurance"
			sensitive = true
		case strings.Contains(colLower, "fee") || strings.Contains(colLower, "cost") || strings.Contains(colLower, "amount"):
			level = "L2"
			classification = "Financial_Fee"
			sensitive = true
		case strings.Contains(colLower, "birth") || strings.Contains(colLower, "age") || strings.Contains(colLower, "gender"):
			level = "L2"
			classification = "Demographics"
			sensitive = true
		}

		fields = append(fields, FieldMetadata{
			Name:           colName,
			Type:           colType,
			SecurityLevel:  level,
			Classification: classification,
			Sensitive:      sensitive,
		})
	}

	return &TableMetadata{
		Name:     tableName,
		RowCount: rowCount,
		Fields:   fields,
	}, nil
}
