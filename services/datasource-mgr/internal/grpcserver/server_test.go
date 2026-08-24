package grpcserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	pb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
)

func setupTestGRPCServer(t *testing.T) (pb.DataSourceManagerServiceClient, func()) {
	t.Helper()

	cfg := config.Load()
	logger := pkgconfig.SetupLogger("text", "debug")
	srvImpl := New(cfg, logger)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterDataSourceManagerServiceServer(s, srvImpl)

	go func() {
		_ = s.Serve(lis)
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	client := pb.NewDataSourceManagerServiceClient(conn)

	cleanup := func() {
		_ = conn.Close()
		s.Stop()
		srvImpl.Shutdown()
		_ = lis.Close()
	}

	return client, cleanup
}

func TestGRPCHealth(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if resp.Status != "ok" || resp.Via != "datasource-mgr" {
		t.Errorf("unexpected health response: %+v", resp)
	}
}

func TestGRPCApis(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// API 1: Yibao
	yibaoResp, err := client.GetYibaoData(ctx, &pb.DataQueryRequest{Limit: 5, Offset: 0})
	if err != nil {
		t.Fatalf("GetYibaoData failed: %v", err)
	}
	if yibaoResp.SourceId != "ds_yibao" || yibaoResp.Limit != 5 {
		t.Errorf("unexpected yibao response: %+v", yibaoResp)
	}

	// API 2: Kangyang
	kangResp, err := client.GetKangyangData(ctx, &pb.DataQueryRequest{Limit: 5, Offset: 0})
	if err != nil {
		t.Fatalf("GetKangyangData failed: %v", err)
	}
	if kangResp.SourceId != "ds_kangyang" || kangResp.Limit != 5 {
		t.Errorf("unexpected kangyang response: %+v", kangResp)
	}

	// API 3: Mock3
	m3Resp, err := client.GetMockData3(ctx, &pb.DataQueryRequest{Limit: 5})
	if err != nil {
		t.Fatalf("GetMockData3 failed: %v", err)
	}
	if m3Resp.SourceId != "ds_mock3" || len(m3Resp.Records) == 0 {
		t.Errorf("unexpected mock3 response: %+v", m3Resp)
	}

	// API 4: Mock4
	m4Resp, err := client.GetMockData4(ctx, &pb.DataQueryRequest{Limit: 5})
	if err != nil {
		t.Fatalf("GetMockData4 failed: %v", err)
	}
	if m4Resp.SourceId != "ds_mock4" || len(m4Resp.Records) == 0 {
		t.Errorf("unexpected mock4 response: %+v", m4Resp)
	}

	// GetDataBySource
	bySrcResp, err := client.GetDataBySource(ctx, &pb.SourceDataQueryRequest{SourceId: "ds_yibao", Limit: 3})
	if err != nil {
		t.Fatalf("GetDataBySource failed: %v", err)
	}
	if bySrcResp.SourceId != "ds_yibao" {
		t.Errorf("unexpected GetDataBySource response: %+v", bySrcResp)
	}

	// ListMockSources
	listResp, err := client.ListMockSources(ctx, &pb.ListMockSourcesRequest{})
	if err != nil {
		t.Fatalf("ListMockSources failed: %v", err)
	}
	if listResp.Total < 2 {
		t.Errorf("expected at least 2 sources, got %d", listResp.Total)
	}

	// GetDataSource
	dsResp, err := client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: "ds_yibao"})
	if err != nil {
		t.Fatalf("GetDataSource failed: %v", err)
	}
	if dsResp.Id != "ds_yibao" {
		t.Errorf("unexpected GetDataSource response: %+v", dsResp)
	}

	// TestConnection
	connResp, err := client.TestConnection(ctx, &pb.TestConnectionRequest{Id: "ds_kangyang"})
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if !connResp.Success || connResp.DatasourceId != "ds_kangyang" {
		t.Errorf("unexpected TestConnection response: %+v", connResp)
	}
}

func TestGRPCValidationErrors(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// GetDataBySource empty source_id
	_, err := client.GetDataBySource(ctx, &pb.SourceDataQueryRequest{SourceId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty source_id, got: %v", err)
	}

	// GetDataBySource non-existent
	_, err = client.GetDataBySource(ctx, &pb.SourceDataQueryRequest{SourceId: "unknown_123"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for unknown source_id, got: %v", err)
	}

	// GetDataSource empty id
	_, err = client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty id, got: %v", err)
	}

	// GetDataSource not found
	_, err = client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: "non_existent"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for non-existent id, got: %v", err)
	}

	// TestConnection empty id
	_, err = client.TestConnection(ctx, &pb.TestConnectionRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty test id, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────
// mTLS Credentials and Key Pinning Tests
// ─────────────────────────────────────────────────────────────

func generateTestCertAndKey(t *testing.T, tmpDir string) (string, string, string, string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "PrivShield-DS-CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}

	caFile := filepath.Join(tmpDir, "ca.pem")
	_ = os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes}), 0600)

	srvKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate srv key: %v", err)
	}
	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "127.0.0.1",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	srvBytes, err := x509.CreateCertificate(rand.Reader, srvTemplate, caTemplate, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	srvCertFile := filepath.Join(tmpDir, "server.crt")
	srvKeyFile := filepath.Join(tmpDir, "server.key")
	_ = os.WriteFile(srvCertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvBytes}), 0600)
	_ = os.WriteFile(srvKeyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(srvKey)}), 0600)

	pubBytes, err := x509.MarshalPKIXPublicKey(&srvKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal pubkey: %v", err)
	}
	pubKeyFile := filepath.Join(tmpDir, "client_pub.pem")
	_ = os.WriteFile(pubKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}), 0600)

	return caFile, srvCertFile, srvKeyFile, pubKeyFile
}

func TestBuildServerCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	caFile, srvCert, srvKey, pubKey := generateTestCertAndKey(t, tmpDir)

	// 1. TLS disabled
	cfg := &config.Config{TLSEnabled: false}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Errorf("expected error when TLS is disabled")
	}

	// 2. Missing cert/key
	cfg = &config.Config{TLSEnabled: true}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Errorf("expected error when cert/key missing")
	}

	// 3. Valid TLS
	cfg = &config.Config{
		TLSEnabled:  true,
		TLSCertFile: srvCert,
		TLSKeyFile:  srvKey,
	}
	creds, err := BuildServerCredentials(cfg)
	if err != nil || creds == nil {
		t.Fatalf("failed to build simple TLS credentials: %v", err)
	}

	// 4. Valid mTLS with client cert verification
	cfg = &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   srvCert,
		TLSKeyFile:    srvKey,
		TLSCAFile:     caFile,
		TLSClientAuth: "require",
	}
	creds, err = BuildServerCredentials(cfg)
	if err != nil || creds == nil {
		t.Fatalf("failed to build mTLS credentials: %v", err)
	}

	// 5. Valid mTLS with public key pinning
	cfg.TLSPinnedPubKeyFile = pubKey
	creds, err = BuildServerCredentials(cfg)
	if err != nil || creds == nil {
		t.Fatalf("failed to build mTLS credentials with public key pinning: %v", err)
	}
}
