// Package handlers provides data provider utilities for mock datasets.
package handlers

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/models"
)

// Pre-defined mock data sources
var MockDataSources = []models.MockDataSource{
	{
		ID:          "ds_yibao",
		Name:        "医保就医与结算模拟数据库 (yibao.csv)",
		Type:        "file",
		Description: "模拟医保局患者就医、诊断与费用结算明细数据",
		Status:      "connected",
		RowCount:    50,
		Tags:        []string{"医保", "门诊住院", "结算流水", "敏感数据"},
	},
	{
		ID:          "ds_kangyang",
		Name:        "康养体检与慢病模拟数据库 (kangyang.csv)",
		Type:        "file",
		Description: "模拟民政/卫健康养中心体检、慢病随访与残疾评估数据",
		Status:      "connected",
		RowCount:    50,
		Tags:        []string{"康养", "慢病随访", "体检报告", "健康档案"},
	},
	{
		ID:          "ds_mock3",
		Name:        "预留政务数据源 3 (Reserved Mock Source 3)",
		Type:        "mock",
		Description: "预留扩展模拟数据源 3，用于后续政务跨部门联合调试",
		Status:      "connected",
		RowCount:    10,
		Tags:        []string{"预留", "政务流通", "扩展接口"},
	},
	{
		ID:          "ds_mock4",
		Name:        "预留政务数据源 4 (Reserved Mock Source 4)",
		Type:        "mock",
		Description: "预留扩展模拟数据源 4，用于后续企业端数据合规流转调试",
		Status:      "connected",
		RowCount:    10,
		Tags:        []string{"预留", "金融统计", "扩展接口"},
	},
}

// ListMockDataSources returns all registered mock sources.
func ListMockDataSources() []models.MockDataSource {
	return MockDataSources
}

// GetMockDataSource returns a mock datasource by ID.
func GetMockDataSource(id string) (*models.MockDataSource, error) {
	for _, ds := range MockDataSources {
		if ds.ID == id || (id == "yibao" && ds.ID == "ds_yibao") || (id == "kangyang" && ds.ID == "ds_kangyang") {
			return &ds, nil
		}
	}
	return nil, fmt.Errorf("mock datasource not found: %s", id)
}

// candidateDirs for finding mock CSV files
var candidateDirs = []string{
	"samples",
	"services/datasource-mgr/samples",
	"data",
	"../../data",
	"../../services/datasource-mgr/samples",
	"console/bff-go/internal/samples",
}

func findCSVFile(filename string) (string, error) {
	cleanName := filepath.Clean(filename)
	baseName := filepath.Base(cleanName)

	for _, dir := range candidateDirs {
		cand := filepath.Join(dir, baseName)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand, nil
		}
	}

	if curr, err := os.Getwd(); err == nil {
		for i := 0; i < 6; i++ {
			for _, sub := range []string{"samples", "services/datasource-mgr/samples", "data", "engine/medical_pipeline/samples"} {
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

	return "", fmt.Errorf("csv file not found: %s", baseName)
}

// LoadCSVRecords loads CSV records with limit and offset.
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
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("read csv header: %w", err)
	}

	var allRows []map[string]any
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		rowMap := make(map[string]any, len(header))
		for i, col := range header {
			colName := strings.TrimSpace(col)
			if i < len(record) {
				val := strings.TrimSpace(record[i])
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

// GetYibaoRecords (API 1: 医保数据)
func GetYibaoRecords(limit, offset int) ([]map[string]any, int, error) {
	return LoadCSVRecords("yibao.csv", limit, offset)
}

// GetKangyangRecords (API 2: 康养数据)
func GetKangyangRecords(limit, offset int) ([]map[string]any, int, error) {
	return LoadCSVRecords("kangyang.csv", limit, offset)
}

// GetMock3Records (API 3: 预留数据 3)
func GetMock3Records(limit, offset int) ([]map[string]any, int, error) {
	rows := []map[string]any{
		{"id": 1, "service_code": "GOV_001", "name": "政务服务审批流水 1", "amount": 1000.0, "status": "approved"},
		{"id": 2, "service_code": "GOV_002", "name": "政务服务审批流水 2", "amount": 2500.0, "status": "pending"},
		{"id": 3, "service_code": "GOV_003", "name": "政务服务审批流水 3", "amount": 320.5, "status": "approved"},
	}
	return paginateSlice(rows, limit, offset), len(rows), nil
}

// GetMock4Records (API 4: 预留数据 4)
func GetMock4Records(limit, offset int) ([]map[string]any, int, error) {
	rows := []map[string]any{
		{"id": 101, "dept_code": "FIN_001", "report_name": "季度税收与财务报表 A", "value": 982000.0},
		{"id": 102, "dept_code": "FIN_002", "report_name": "季度税收与财务报表 B", "value": 431000.0},
	}
	return paginateSlice(rows, limit, offset), len(rows), nil
}

// GetDataBySource retrieves records by source ID.
func GetDataBySource(sourceID string, limit, offset int) ([]map[string]any, int, string, error) {
	switch strings.ToLower(sourceID) {
	case "ds_yibao", "yibao", "yibao.csv":
		rows, total, err := GetYibaoRecords(limit, offset)
		return rows, total, "医保就医与结算模拟数据库 (yibao.csv)", err
	case "ds_kangyang", "kangyang", "kangyang.csv":
		rows, total, err := GetKangyangRecords(limit, offset)
		return rows, total, "康养体检与慢病模拟数据库 (kangyang.csv)", err
	case "ds_mock3", "mock3":
		rows, total, err := GetMock3Records(limit, offset)
		return rows, total, "预留政务数据源 3", err
	case "ds_mock4", "mock4":
		rows, total, err := GetMock4Records(limit, offset)
		return rows, total, "预留政务数据源 4", err
	default:
		return nil, 0, "", fmt.Errorf("unknown mock source: %s", sourceID)
	}
}

func paginateSlice(rows []map[string]any, limit, offset int) []map[string]any {
	total := len(rows)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []map[string]any{}
	}
	end := offset + limit
	if end > total || limit <= 0 {
		end = total
	}
	return rows[offset:end]
}

// GetMetadata returns table schema for a mock source.
func GetMetadata(sourceID string) (*models.MetadataResponse, error) {
	ds, err := GetMockDataSource(sourceID)
	if err != nil {
		return nil, err
	}

	fields := []models.MetadataField{
		{Name: "id", Type: "string"},
		{Name: "name", Type: "string"},
		{Name: "created_at", Type: "timestamp"},
	}

	if ds.ID == "ds_yibao" {
		fields = []models.MetadataField{
			{Name: "insurance_settlement_id", Type: "string"},
			{Name: "person_id", Type: "string"},
			{Name: "gender", Type: "string"},
			{Name: "birth_date", Type: "string"},
			{Name: "diagnosis_name", Type: "string"},
			{Name: "settlement_amount", Type: "float"},
		}
	} else if ds.ID == "ds_kangyang" {
		fields = []models.MetadataField{
			{Name: "elder_id", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "age", Type: "integer"},
			{Name: "chronic_disease", Type: "string"},
			{Name: "blood_pressure", Type: "string"},
		}
	}

	return &models.MetadataResponse{
		DataSourceID: ds.ID,
		Tables: []models.TableMetadata{
			{
				Name:     ds.Name,
				RowCount: ds.RowCount,
				Fields:   fields,
			},
		},
		Via: "datasource-mgr",
	}, nil
}
