package models

import (
	"encoding/json"
	"testing"
)

func TestMockDataSourceSerialization(t *testing.T) {
	ds := MockDataSource{
		ID:          "ds_yibao",
		Name:        "医保就医与结算模拟数据库",
		Type:        "file",
		Description: "模拟医保数据",
		Status:      "connected",
		RowCount:    50,
		Tags:        []string{"医保", "门诊"},
	}

	data, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("failed to marshal MockDataSource: %v", err)
	}

	var parsed MockDataSource
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal MockDataSource: %v", err)
	}

	if parsed.ID != ds.ID || parsed.RowCount != 50 || len(parsed.Tags) != 2 {
		t.Errorf("unmarshaled MockDataSource mismatch: %+v", parsed)
	}
}

func TestDataQueryResponseSerialization(t *testing.T) {
	resp := DataQueryResponse{
		SourceID:   "ds_kangyang",
		SourceName: "康养体检与慢病模拟数据库",
		Total:      50,
		Limit:      20,
		Offset:     0,
		Records: []map[string]any{
			{"name": "张三", "age": 65},
		},
		Via: "datasource-mgr",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal DataQueryResponse: %v", err)
	}

	var parsed DataQueryResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DataQueryResponse: %v", err)
	}

	if parsed.SourceID != resp.SourceID || len(parsed.Records) != 1 {
		t.Errorf("unmarshaled DataQueryResponse mismatch: %+v", parsed)
	}
}

func TestMetadataResponseSerialization(t *testing.T) {
	meta := MetadataResponse{
		DataSourceID: "ds_yibao",
		Tables: []TableMetadata{
			{
				Name:     "yibao_settlement",
				RowCount: 50,
				Fields: []MetadataField{
					{Name: "person_id", Type: "string"},
					{Name: "amount", Type: "float"},
				},
			},
		},
		Via: "datasource-mgr",
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal MetadataResponse: %v", err)
	}

	var parsed MetadataResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal MetadataResponse: %v", err)
	}

	if parsed.DataSourceID != "ds_yibao" || len(parsed.Tables[0].Fields) != 2 {
		t.Errorf("unmarshaled MetadataResponse mismatch: %+v", parsed)
	}
}
