// Package grpcserver implements the gRPC service for the mock datasource-mgr module with mTLS support.
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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
)

const moduleVia = "datasource-mgr"

// GRPCServer implements pb.DataSourceManagerServiceServer.
type GRPCServer struct {
	pb.UnimplementedDataSourceManagerServiceServer

	cfg    *config.Config
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new GRPCServer instance.
func New(cfg *config.Config, logger *slog.Logger) *GRPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GRPCServer{
		cfg:    cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Shutdown gracefully stops server tasks.
func (s *GRPCServer) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// Health returns self health status.
func (s *GRPCServer) Health(ctx context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Status:    "ok",
		LatencyMs: 0,
		Via:       moduleVia,
	}, nil
}

// API 1: GetYibaoData
func (s *GRPCServer) GetYibaoData(ctx context.Context, req *pb.DataQueryRequest) (*pb.DataQueryResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	rows, total, err := handlers.GetYibaoRecords(limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get yibao records: %v", err)
	}

	return toDataQueryResponse("ds_yibao", "医保就医与结算模拟数据库 (yibao.csv)", total, limit, offset, rows), nil
}

// API 2: GetKangyangData
func (s *GRPCServer) GetKangyangData(ctx context.Context, req *pb.DataQueryRequest) (*pb.DataQueryResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	rows, total, err := handlers.GetKangyangRecords(limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get kangyang records: %v", err)
	}

	return toDataQueryResponse("ds_kangyang", "康养体检与慢病模拟数据库 (kangyang.csv)", total, limit, offset, rows), nil
}

// API 3: GetMockData3
func (s *GRPCServer) GetMockData3(ctx context.Context, req *pb.DataQueryRequest) (*pb.DataQueryResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	rows, total, err := handlers.GetMock3Records(limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get mock3 records: %v", err)
	}

	return toDataQueryResponse("ds_mock3", "预留政务数据源 3", total, limit, offset, rows), nil
}

// API 4: GetMockData4
func (s *GRPCServer) GetMockData4(ctx context.Context, req *pb.DataQueryRequest) (*pb.DataQueryResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	rows, total, err := handlers.GetMock4Records(limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get mock4 records: %v", err)
	}

	return toDataQueryResponse("ds_mock4", "预留政务数据源 4", total, limit, offset, rows), nil
}

// GetDataBySource
func (s *GRPCServer) GetDataBySource(ctx context.Context, req *pb.SourceDataQueryRequest) (*pb.DataQueryResponse, error) {
	if strings.TrimSpace(req.SourceId) == "" {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	rows, total, name, err := handlers.GetDataBySource(req.SourceId, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}

	return toDataQueryResponse(req.SourceId, name, total, limit, offset, rows), nil
}

// ListMockSources
func (s *GRPCServer) ListMockSources(ctx context.Context, _ *pb.ListMockSourcesRequest) (*pb.ListMockSourcesResponse, error) {
	list := handlers.ListMockDataSources()
	protos := make([]*pb.DataSourceProto, 0, len(list))
	for _, d := range list {
		protos = append(protos, &pb.DataSourceProto{
			Id:          d.ID,
			Name:        d.Name,
			Type:        d.Type,
			Description: d.Description,
			Status:      d.Status,
			RowCount:    int32(d.RowCount),
			Tags:        d.Tags,
		})
	}
	return &pb.ListMockSourcesResponse{
		Total:   int32(len(protos)),
		Sources: protos,
		Via:     moduleVia,
	}, nil
}

// GetDataSource
func (s *GRPCServer) GetDataSource(ctx context.Context, req *pb.GetDataSourceRequest) (*pb.DataSourceProto, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	ds, err := handlers.GetMockDataSource(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.DataSourceProto{
		Id:          ds.ID,
		Name:        ds.Name,
		Type:        ds.Type,
		Description: ds.Description,
		Status:      ds.Status,
		RowCount:    int32(ds.RowCount),
		Tags:        ds.Tags,
	}, nil
}

// TestConnection
func (s *GRPCServer) TestConnection(ctx context.Context, req *pb.TestConnectionRequest) (*pb.TestConnectionResponse, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	_, err := handlers.GetMockDataSource(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.TestConnectionResponse{
		DatasourceId: req.Id,
		Success:      true,
		LatencyMs:    1,
		Via:          moduleVia,
	}, nil
}

func toDataQueryResponse(id, name string, total, limit, offset int, rows []map[string]any) *pb.DataQueryResponse {
	recordsProto := make([]*pb.DataRowProto, 0, len(rows))
	for _, row := range rows {
		fieldMap := make(map[string]string, len(row))
		for k, v := range row {
			fieldMap[k] = fmt.Sprintf("%v", v)
		}
		recordsProto = append(recordsProto, &pb.DataRowProto{Fields: fieldMap})
	}
	return &pb.DataQueryResponse{
		SourceId:   id,
		SourceName: name,
		Total:      int32(total),
		Limit:      int32(limit),
		Offset:     int32(offset),
		Records:    recordsProto,
		Via:        moduleVia,
	}
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
