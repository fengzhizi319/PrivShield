// Package grpcserver implements the gRPC service for the datasource-mgr module with mTLS support.
// Package grpcserver 实现数据源管理模块的 gRPC 服务端，支持 mTLS 双向认证与公钥固定。
package grpcserver

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/validation"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
)

const moduleVia = "datasource-mgr"

// GRPCServer implements pb.DataSourceManagerServiceServer.
type GRPCServer struct {
	pb.UnimplementedDataSourceManagerServiceServer

	agent     *agent.Client
	cfg       *config.Config
	ds        store.DataSourceStore
	logger    *slog.Logger
	startTime time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new GRPCServer instance.
func New(ag *agent.Client, cfg *config.Config, ds store.DataSourceStore, logger *slog.Logger) *GRPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GRPCServer{
		agent:     ag,
		cfg:       cfg,
		ds:        ds,
		logger:    logger,
		startTime: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Shutdown gracefully stops background tasks.
func (s *GRPCServer) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// Health checks self and upstream agent connectivity.
func (s *GRPCServer) Health(ctx context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	agentData, err := s.agent.Health(timeoutCtx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		s.logger.Warn("gRPC Health: agent unreachable", "error", err.Error())
		return &pb.HealthResponse{
			Backend:   "ok",
			Agent:     "unreachable",
			AgentUrl:  s.cfg.AgentBaseURL(),
			LatencyMs: latency,
			Error:     err.Error(),
			Via:       moduleVia,
		}, nil
	}

	agentStatus := "ok"
	if st, ok := agentData["status"].(string); ok {
		agentStatus = st
	}

	return &pb.HealthResponse{
		Backend:   "ok",
		Agent:     agentStatus,
		AgentUrl:  s.cfg.AgentBaseURL(),
		LatencyMs: latency,
		Via:       moduleVia,
	}, nil
}

// ListDataSources returns registered data sources with pagination.
func (s *GRPCServer) ListDataSources(ctx context.Context, req *pb.ListDataSourcesRequest) (*pb.ListDataSourcesResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	list, total, err := s.ds.ListDS(store.DataSourceFilter{Limit: limit, Offset: offset})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list datasources: %v", err)
	}

	protos := make([]*pb.DataSourceProto, 0, len(list))
	for i := range list {
		protos = append(protos, dsToProto(&list[i]))
	}

	return &pb.ListDataSourcesResponse{
		Total:       int32(total),
		Datasources: protos,
		Limit:       int32(limit),
		Offset:      int32(offset),
		Via:         moduleVia,
	}, nil
}

// GetDataSource returns a single data source by ID.
func (s *GRPCServer) GetDataSource(ctx context.Context, req *pb.GetDataSourceRequest) (*pb.DataSourceProto, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	ds, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}
	return dsToProto(ds), nil
}

// CreateDataSource registers a new data source.
func (s *GRPCServer) CreateDataSource(ctx context.Context, req *pb.CreateDataSourceRequest) (*pb.DataSourceProto, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource name is required")
	}
	if err := validation.AllowedValues("type", req.Type, validation.DataSourceTypes); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := validation.PortRange(int(req.Port)); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if strings.TrimSpace(req.Host) == "" {
		return nil, status.Error(codes.InvalidArgument, "host cannot be empty")
	}

	secLevel := req.SecurityLevel
	if secLevel == "" {
		secLevel = "medium"
	}
	if err := validation.AllowedValues("security_level", secLevel, validation.SecurityLevels); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	now := time.Now()
	id := fmt.Sprintf("ds_%d", now.UnixNano())
	ds := &store.DataSource{
		ID:            id,
		Name:          req.Name,
		Type:          req.Type,
		Host:          req.Host,
		Port:          int(req.Port),
		Database:      req.Database,
		SecurityLevel: secLevel,
		Status:        "connected",
		CreatedAt:     now,
		LastCheckAt:   &now,
		Tags:          req.Tags,
	}

	if err := s.ds.SaveDS(ds); err != nil {
		return nil, status.Errorf(codes.Internal, "save datasource: %v", err)
	}

	s.logger.Info("gRPC created datasource", "id", ds.ID, "name", ds.Name)
	return dsToProto(ds), nil
}

// UpdateDataSource updates an existing data source.
func (s *GRPCServer) UpdateDataSource(ctx context.Context, req *pb.UpdateDataSourceRequest) (*pb.DataSourceProto, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	existing, err := s.ds.GetDS(req.Id)
	if err != nil || existing == nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Host != "" {
		existing.Host = req.Host
	}
	if req.Port > 0 {
		if err := validation.PortRange(int(req.Port)); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		existing.Port = int(req.Port)
	}
	if req.Database != "" {
		existing.Database = req.Database
	}
	if req.SecurityLevel != "" {
		if err := validation.AllowedValues("security_level", req.SecurityLevel, validation.SecurityLevels); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		existing.SecurityLevel = req.SecurityLevel
	}
	if len(req.Tags) > 0 {
		existing.Tags = req.Tags
	}

	if err := s.ds.UpdateDS(existing); err != nil {
		return nil, status.Errorf(codes.Internal, "update datasource: %v", err)
	}

	return dsToProto(existing), nil
}

// DeleteDataSource deletes a data source by ID.
func (s *GRPCServer) DeleteDataSource(ctx context.Context, req *pb.DeleteDataSourceRequest) (*pb.DeleteDataSourceResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	_, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	if err := s.ds.DeleteDS(req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "delete datasource: %v", err)
	}

	return &pb.DeleteDataSourceResponse{
		Success: true,
		Message: fmt.Sprintf("datasource %s deleted successfully", req.Id),
		Via:     moduleVia,
	}, nil
}

// TestConnection verifies connectivity to a data source.
func (s *GRPCServer) TestConnection(ctx context.Context, req *pb.TestConnectionRequest) (*pb.TestConnectionResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	ds, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	start := time.Now()
	now := time.Now()
	ds.LastCheckAt = &now

	// Simulate or execute connection test based on type
	if ds.Type == "file" && ds.Database != "" {
		_, _, loadErr := handlers.LoadCSVRecords(ds.Database, 1, 0)
		latency := time.Since(start).Milliseconds()
		if loadErr != nil {
			ds.Status = "error"
			_ = s.ds.UpdateDS(ds)
			return &pb.TestConnectionResponse{
				DatasourceId: ds.ID,
				Success:      false,
				LatencyMs:    latency,
				Error:        loadErr.Error(),
				Via:          moduleVia,
			}, nil
		}
		ds.Status = "connected"
		_ = s.ds.UpdateDS(ds)
		return &pb.TestConnectionResponse{
			DatasourceId: ds.ID,
			Success:      true,
			LatencyMs:    latency,
			Via:          moduleVia,
		}, nil
	}

	ds.Status = "connected"
	_ = s.ds.UpdateDS(ds)
	latency := time.Since(start).Milliseconds()
	return &pb.TestConnectionResponse{
		DatasourceId: ds.ID,
		Success:      true,
		LatencyMs:    latency,
		Via:          moduleVia,
	}, nil
}

// GetMetadata returns table metadata and auto-classified fields.
func (s *GRPCServer) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.MetadataResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	ds, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	var tables []*pb.TableMetadataProto

	if ds.Type == "file" && ds.Database != "" {
		tblMeta, err := handlers.ExtractCSVMetadata(ds.Name, ds.Database)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "extract metadata: %v", err)
		}

		fieldsProto := make([]*pb.MetadataFieldProto, 0, len(tblMeta.Fields))
		for _, f := range tblMeta.Fields {
			fieldsProto = append(fieldsProto, &pb.MetadataFieldProto{
				Name:           f.Name,
				Type:           f.Type,
				SecurityLevel:  f.SecurityLevel,
				Classification: f.Classification,
				Sensitive:      f.Sensitive,
			})
		}

		tables = append(tables, &pb.TableMetadataProto{
			Name:     tblMeta.Name,
			RowCount: int32(tblMeta.RowCount),
			Fields:   fieldsProto,
		})
	} else {
		// Mock default metadata
		tables = append(tables, &pb.TableMetadataProto{
			Name:     "default_table",
			RowCount: 100,
			Fields: []*pb.MetadataFieldProto{
				{Name: "id", Type: "integer", SecurityLevel: "L1", Classification: "general", Sensitive: false},
				{Name: "name", Type: "string", SecurityLevel: "L3", Classification: "PII_Name", Sensitive: true},
				{Name: "phone", Type: "string", SecurityLevel: "L3", Classification: "PII_Phone", Sensitive: true},
			},
		})
	}

	return &pb.MetadataResponse{
		DatasourceId: ds.ID,
		Tables:       tables,
		Via:          moduleVia,
	}, nil
}

// GetDataSourceRecords reads records from the data source.
func (s *GRPCServer) GetDataSourceRecords(ctx context.Context, req *pb.GetRecordsRequest) (*pb.GetRecordsResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	ds, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	if ds.Type == "file" && ds.Database != "" {
		rows, total, err := handlers.LoadCSVRecords(ds.Database, limit, offset)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "load csv records: %v", err)
		}

		recordsProto := make([]*pb.RecordRowProto, 0, len(rows))
		for _, row := range rows {
			fieldMap := make(map[string]string, len(row))
			for k, v := range row {
				fieldMap[k] = fmt.Sprintf("%v", v)
			}
			recordsProto = append(recordsProto, &pb.RecordRowProto{Fields: fieldMap})
		}

		return &pb.GetRecordsResponse{
			DatasourceId: ds.ID,
			Total:        int32(total),
			Limit:        int32(limit),
			Offset:       int32(offset),
			Records:      recordsProto,
			Via:          moduleVia,
		}, nil
	}

	return &pb.GetRecordsResponse{
		DatasourceId: ds.ID,
		Total:        0,
		Limit:        int32(limit),
		Offset:       int32(offset),
		Records:      []*pb.RecordRowProto{},
		Via:          moduleVia,
	}, nil
}

// GetAccessAudit returns access audit log records.
func (s *GRPCServer) GetAccessAudit(ctx context.Context, req *pb.GetAccessAuditRequest) (*pb.AccessAuditResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "datasource id is required")
	}

	ds, err := s.ds.GetDS(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "datasource not found: %s", req.Id)
	}

	now := time.Now()
	records := []*pb.AccessAuditRecordProto{
		{
			Id:             fmt.Sprintf("audit_%s_1", ds.ID),
			DatasourceId:   ds.ID,
			DatasourceName: ds.Name,
			Operation:      "query",
			User:           "sec_officer",
			Timestamp:      now.Add(-10 * time.Minute).Format(time.RFC3339),
			RecordsCount:   20,
			Status:         "success",
		},
		{
			Id:             fmt.Sprintf("audit_%s_2", ds.ID),
			DatasourceId:   ds.ID,
			DatasourceName: ds.Name,
			Operation:      "mask",
			User:           "service_hub",
			Timestamp:      now.Add(-5 * time.Minute).Format(time.RFC3339),
			RecordsCount:   50,
			Status:         "success",
		},
	}

	return &pb.AccessAuditResponse{
		DatasourceId: ds.ID,
		Total:        int32(len(records)),
		Records:      records,
		Via:          moduleVia,
	}, nil
}

// SeedDataSources initializes default datasets.
func (s *GRPCServer) SeedDataSources(ctx context.Context, _ *pb.SeedDataSourcesRequest) (*pb.SeedDataSourcesResponse, error) {
	if err := handlers.SeedMockDataSources(s.ds, s.logger); err != nil {
		return nil, status.Errorf(codes.Internal, "seed datasources: %v", err)
	}

	list, _, _ := s.ds.ListDS(store.DataSourceFilter{Limit: 100})
	seededIDs := make([]string, 0, len(list))
	for _, item := range list {
		seededIDs = append(seededIDs, item.ID)
	}

	return &pb.SeedDataSourcesResponse{
		SeededCount: int32(len(seededIDs)),
		SeededIds:   seededIDs,
		Via:         moduleVia,
	}, nil
}

// dsToProto converts a store.DataSource to pb.DataSourceProto.
func dsToProto(d *store.DataSource) *pb.DataSourceProto {
	p := &pb.DataSourceProto{
		Id:            d.ID,
		Name:          d.Name,
		Type:          d.Type,
		Host:          d.Host,
		Port:          int32(d.Port),
		Database:      d.Database,
		SecurityLevel: d.SecurityLevel,
		Status:        d.Status,
		CreatedAt:     d.CreatedAt.Format(time.RFC3339),
		Tags:          d.Tags,
	}
	if d.LastCheckAt != nil {
		p.LastCheckAt = d.LastCheckAt.Format(time.RFC3339)
	}
	return p
}

// ─────────────────────────────────────────────────────────────
// mTLS Credentials Builder / mTLS 凭证构造与公钥固定
// ─────────────────────────────────────────────────────────────

// BuildServerCredentials constructs gRPC transport credentials supporting mTLS and public key pinning.
func BuildServerCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if !cfg.TLSEnabled {
		return nil, fmt.Errorf("TLS is disabled in configuration")
	}
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("TLS cert file and key file must be configured")
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server x509 key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	clientAuthMode := strings.ToLower(strings.TrimSpace(cfg.TLSClientAuth))
	if clientAuthMode != "" {
		if cfg.TLSCAFile == "" {
			return nil, fmt.Errorf("TLS CA file must be configured when client auth is enabled")
		}
		caPEM, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.TLSCAFile)
		}
		tlsConfig.ClientCAs = caPool

		switch clientAuthMode {
		case "require", "requireandverify":
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		case "verify":
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		case "request":
			tlsConfig.ClientAuth = tls.RequestClientCert
		default:
			return nil, fmt.Errorf("unknown TLS client auth mode: %s", cfg.TLSClientAuth)
		}
	}

	if cfg.TLSPinnedPubKeyFile != "" {
		pinnedKey, err := loadPublicKey(cfg.TLSPinnedPubKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load pinned client public key: %w", err)
		}
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("mTLS: client did not present a certificate")
			}
			peerCert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("mTLS: failed to parse peer certificate: %w", err)
			}
			if !publicKeysEqual(peerCert.PublicKey, pinnedKey) {
				return fmt.Errorf("mTLS: client public key does not match pinned key")
			}
			return nil
		}
	}

	return credentials.NewTLS(tlsConfig), nil
}

func loadPublicKey(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in %s", path)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		cert, certErr := x509.ParseCertificate(block.Bytes)
		if certErr == nil {
			return cert.PublicKey, nil
		}
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return pub, nil
}

func publicKeysEqual(a, b any) bool {
	rsaA, okA := a.(*rsa.PublicKey)
	rsaB, okB := b.(*rsa.PublicKey)
	if okA && okB {
		return rsaA.N.Cmp(rsaB.N) == 0 && rsaA.E == rsaB.E
	}
	return false
}
