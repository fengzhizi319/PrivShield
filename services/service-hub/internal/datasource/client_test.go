package datasource

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"google.golang.org/grpc"

	dspb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
)

type mockDSPBServer struct {
	dspb.UnimplementedDataSourceManagerServiceServer
}

func (s *mockDSPBServer) Health(ctx context.Context, _ *dspb.HealthRequest) (*dspb.HealthResponse, error) {
	return &dspb.HealthResponse{Status: "ok", LatencyMs: 1, Via: "datasource-mgr"}, nil
}

func (s *mockDSPBServer) GetYibaoData(ctx context.Context, req *dspb.DataQueryRequest) (*dspb.DataQueryResponse, error) {
	return &dspb.DataQueryResponse{
		SourceId:   "ds_yibao",
		SourceName: "医保就医结算",
		Total:      50,
		Records:    []*dspb.DataRowProto{{Fields: map[string]string{"name": "张三", "id_card": "110101199001011234"}}},
		Via:        "datasource-mgr",
	}, nil
}

func (s *mockDSPBServer) GetKangyangData(ctx context.Context, req *dspb.DataQueryRequest) (*dspb.DataQueryResponse, error) {
	return &dspb.DataQueryResponse{
		SourceId:   "ds_kangyang",
		SourceName: "康养健康档案",
		Total:      50,
		Records:    []*dspb.DataRowProto{{Fields: map[string]string{"name": "李四", "phone": "13800138000"}}},
		Via:        "datasource-mgr",
	}, nil
}

func (s *mockDSPBServer) GetMockData3(ctx context.Context, req *dspb.DataQueryRequest) (*dspb.DataQueryResponse, error) {
	return &dspb.DataQueryResponse{SourceId: "ds_mock3", Total: 10, Via: "datasource-mgr"}, nil
}

func (s *mockDSPBServer) GetMockData4(ctx context.Context, req *dspb.DataQueryRequest) (*dspb.DataQueryResponse, error) {
	return &dspb.DataQueryResponse{SourceId: "ds_mock4", Total: 10, Via: "datasource-mgr"}, nil
}

func (s *mockDSPBServer) GetDataBySource(ctx context.Context, req *dspb.SourceDataQueryRequest) (*dspb.DataQueryResponse, error) {
	if req.SourceId == "ds_kangyang" {
		return s.GetKangyangData(ctx, &dspb.DataQueryRequest{})
	}
	return s.GetYibaoData(ctx, &dspb.DataQueryRequest{})
}

func (s *mockDSPBServer) ListMockSources(ctx context.Context, _ *dspb.ListMockSourcesRequest) (*dspb.ListMockSourcesResponse, error) {
	return &dspb.ListMockSourcesResponse{
		Total: 2,
		Sources: []*dspb.DataSourceProto{
			{Id: "ds_yibao", Name: "医保数据", Status: "connected"},
			{Id: "ds_kangyang", Name: "康养数据", Status: "connected"},
		},
		Via: "datasource-mgr",
	}, nil
}

func (s *mockDSPBServer) GetDataSource(ctx context.Context, req *dspb.GetDataSourceRequest) (*dspb.DataSourceProto, error) {
	return &dspb.DataSourceProto{Id: req.Id, Name: "测试数据源", Status: "connected"}, nil
}

func (s *mockDSPBServer) TestConnection(ctx context.Context, req *dspb.TestConnectionRequest) (*dspb.TestConnectionResponse, error) {
	return &dspb.TestConnectionResponse{DatasourceId: req.Id, Success: true, LatencyMs: 1, Via: "datasource-mgr"}, nil
}

func setupMockDatasourceServer(t *testing.T) (*httptest.Server, *grpc.Server, net.Listener, *config.Config) {
	t.Helper()

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "backend": "ok"})
	})

	// API 1: Yibao
	mux.HandleFunc("/api/v1/yibao", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DataQueryResult{
			SourceID:   "ds_yibao",
			SourceName: "医保就医结算",
			Total:      50,
			Records:    []map[string]any{{"person_id": "110101", "name": "张三"}},
			Via:        "datasource-mgr",
		})
	})

	// API 2: Kangyang
	mux.HandleFunc("/api/v1/kangyang", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DataQueryResult{
			SourceID:   "ds_kangyang",
			SourceName: "康养健康档案",
			Total:      50,
			Records:    []map[string]any{{"elder_id": "KY001", "name": "李四"}},
			Via:        "datasource-mgr",
		})
	})

	// API 3: Mock3
	mux.HandleFunc("/api/v1/mock3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DataQueryResult{
			SourceID: "ds_mock3",
			Total:    10,
			Records:  []map[string]any{{"service_code": "GOV_01"}},
		})
	})

	// API 4: Mock4
	mux.HandleFunc("/api/v1/mock4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DataQueryResult{
			SourceID: "ds_mock4",
			Total:    10,
			Records:  []map[string]any{{"dept_code": "FIN_01"}},
		})
	})

	// List datasources
	mux.HandleFunc("/api/datasources", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total":       2,
			"datasources": []string{"ds_yibao", "ds_kangyang"},
		})
	})

	// Test connection
	mux.HandleFunc("/api/datasources/ds_yibao/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"datasource_id": "ds_yibao",
			"success":       true,
		})
	})

	srv := httptest.NewServer(mux)

	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	// Start mock gRPC server
	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen gRPC failed: %v", err)
	}
	grpcHost, grpcPortStr, _ := net.SplitHostPort(grpcLis.Addr().String())
	grpcPort, _ := strconv.Atoi(grpcPortStr)

	grpcSrv := grpc.NewServer()
	dspb.RegisterDataSourceManagerServiceServer(grpcSrv, &mockDSPBServer{})

	go func() {
		_ = grpcSrv.Serve(grpcLis)
	}()

	cfg := &config.Config{
		DatasourceRESTHost: u.Hostname(),
		DatasourceRESTPort: port,
		DatasourceGRPCHost: grpcHost,
		DatasourceGRPCPort: grpcPort,
	}

	return srv, grpcSrv, grpcLis, cfg
}

func TestDatasourceClient(t *testing.T) {
	srv, grpcSrv, grpcLis, cfg := setupMockDatasourceServer(t)
	defer func() {
		grpcSrv.Stop()
		_ = grpcLis.Close()
		srv.Close()
	}()

	client := New(cfg)
	defer client.Close()
	ctx := context.Background()

	// ── REST Tests ──
	// 1. Health
	h, err := client.Health(ctx)
	if err != nil || h["status"] != "ok" {
		t.Fatalf("Health failed: %v, resp: %+v", err, h)
	}

	// 2. FetchYibaoData
	yb, err := client.FetchYibaoData(ctx, 10, 0)
	if err != nil || yb.SourceID != "ds_yibao" {
		t.Fatalf("FetchYibaoData failed: %v, resp: %+v", err, yb)
	}

	// 3. FetchKangyangData
	ky, err := client.FetchKangyangData(ctx, 10, 0)
	if err != nil || ky.SourceID != "ds_kangyang" {
		t.Fatalf("FetchKangyangData failed: %v, resp: %+v", err, ky)
	}

	// 4. FetchMockData3 & FetchMockData4
	m3, err := client.FetchMockData3(ctx, 5, 0)
	if err != nil || m3.SourceID != "ds_mock3" {
		t.Fatalf("FetchMockData3 failed: %v", err)
	}
	m4, err := client.FetchMockData4(ctx, 5, 0)
	if err != nil || m4.SourceID != "ds_mock4" {
		t.Fatalf("FetchMockData4 failed: %v", err)
	}

	// 5. FetchDataBySource dispatch
	bySrc, err := client.FetchDataBySource(ctx, "医保数据库", 5, 0)
	if err != nil || bySrc.SourceID != "ds_yibao" {
		t.Fatalf("FetchDataBySource dispatch failed: %v", err)
	}

	// 6. ListDataSources
	list, err := client.ListDataSources(ctx)
	if err != nil || list["total"].(float64) != 2 {
		t.Fatalf("ListDataSources failed: %v", err)
	}

	// 7. TestConnection
	conn, err := client.TestConnection(ctx, "ds_yibao")
	if err != nil || conn["success"] != true {
		t.Fatalf("TestConnection failed: %v", err)
	}

	// ── gRPC Tests ──
	// 8. HealthGRPC
	grpcHealth, err := client.HealthGRPC(ctx)
	if err != nil || grpcHealth.Status != "ok" {
		t.Fatalf("HealthGRPC failed: %v", err)
	}

	// 9. FetchYibaoDataGRPC
	ybGRPC, err := client.FetchYibaoDataGRPC(ctx, 10, 0)
	if err != nil || ybGRPC.SourceID != "ds_yibao" || len(ybGRPC.Records) == 0 {
		t.Fatalf("FetchYibaoDataGRPC failed: %v", err)
	}

	// 10. FetchKangyangDataGRPC
	kyGRPC, err := client.FetchKangyangDataGRPC(ctx, 10, 0)
	if err != nil || kyGRPC.SourceID != "ds_kangyang" || len(kyGRPC.Records) == 0 {
		t.Fatalf("FetchKangyangDataGRPC failed: %v", err)
	}

	// 11. FetchMockData3GRPC & FetchMockData4GRPC
	m3GRPC, err := client.FetchMockData3GRPC(ctx, 5, 0)
	if err != nil || m3GRPC.SourceID != "ds_mock3" {
		t.Fatalf("FetchMockData3GRPC failed: %v", err)
	}
	m4GRPC, err := client.FetchMockData4GRPC(ctx, 5, 0)
	if err != nil || m4GRPC.SourceID != "ds_mock4" {
		t.Fatalf("FetchMockData4GRPC failed: %v", err)
	}

	// 12. FetchDataBySourceGRPC
	bySrcGRPC, err := client.FetchDataBySourceGRPC(ctx, "ds_kangyang", 5, 0)
	if err != nil || bySrcGRPC.SourceID != "ds_kangyang" {
		t.Fatalf("FetchDataBySourceGRPC failed: %v", err)
	}

	// 13. ListMockSourcesGRPC
	listGRPC, err := client.ListMockSourcesGRPC(ctx)
	if err != nil || listGRPC.Total != 2 {
		t.Fatalf("ListMockSourcesGRPC failed: %v", err)
	}

	// 14. GetDataSourceGRPC
	dsInfo, err := client.GetDataSourceGRPC(ctx, "ds_yibao")
	if err != nil || dsInfo.Id != "ds_yibao" {
		t.Fatalf("GetDataSourceGRPC failed: %v", err)
	}

	// 15. TestConnectionGRPC
	connGRPC, err := client.TestConnectionGRPC(ctx, "ds_yibao")
	if err != nil || !connGRPC.Success {
		t.Fatalf("TestConnectionGRPC failed: %v", err)
	}
}
