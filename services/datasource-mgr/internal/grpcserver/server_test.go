package grpcserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	pb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
)

func setupTestGRPCServer(t *testing.T, agentMux http.Handler) (pb.DataSourceManagerServiceClient, *GRPCServer, func()) {
	t.Helper()

	var agentURL string
	if agentMux != nil {
		agentSrv := httptest.NewServer(agentMux)
		agentURL = agentSrv.URL
	}

	t.Setenv("PRIVACY_AGENT_URLS", agentURL)
	cfg := config.Load()
	logger := pkgconfig.SetupLogger("text", "debug")
	dsStore := memory.NewDataSourceStore()
	agentClient := agent.New(cfg)

	srvImpl := New(agentClient, cfg, dsStore, logger)

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

	return client, srvImpl, cleanup
}

func TestGRPCHealth(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	client, _, cleanup := setupTestGRPCServer(t, mux)
	defer cleanup()

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if resp.Backend != "ok" || resp.Agent != "ok" {
		t.Errorf("unexpected health response: %+v", resp)
	}
}

func TestGRPCHealthAgentUnreachable(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t, nil)
	defer cleanup()

	resp, err := client.Health(context.Background(), &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if resp.Backend != "ok" || resp.Agent != "unreachable" {
		t.Errorf("unexpected health response when agent unreachable: %+v", resp)
	}
}

func TestGRPCDataSourceCRUD(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t, nil)
	defer cleanup()

	ctx := context.Background()

	// 1. Create
	createResp, err := client.CreateDataSource(ctx, &pb.CreateDataSourceRequest{
		Name:          "Test Hospital DB",
		Type:          "database",
		Host:          "192.168.1.100",
		Port:          3306,
		Database:      "hospital_ehr",
		SecurityLevel: "high",
		Tags:          []string{"hospital", "ehr"},
	})
	if err != nil {
		t.Fatalf("CreateDataSource failed: %v", err)
	}
	if createResp.Id == "" || createResp.Name != "Test Hospital DB" {
		t.Errorf("unexpected created datasource: %+v", createResp)
	}

	dsID := createResp.Id

	// 2. Get
	getResp, err := client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: dsID})
	if err != nil {
		t.Fatalf("GetDataSource failed: %v", err)
	}
	if getResp.Id != dsID || getResp.Port != 3306 {
		t.Errorf("unexpected get datasource: %+v", getResp)
	}

	// 3. List
	listResp, err := client.ListDataSources(ctx, &pb.ListDataSourcesRequest{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListDataSources failed: %v", err)
	}
	if listResp.Total < 1 || len(listResp.Datasources) < 1 {
		t.Errorf("unexpected list datasources: %+v", listResp)
	}

	// 4. Update
	updateResp, err := client.UpdateDataSource(ctx, &pb.UpdateDataSourceRequest{
		Id:            dsID,
		Name:          "Updated Hospital EHR",
		SecurityLevel: "high",
		Port:          3307,
	})
	if err != nil {
		t.Fatalf("UpdateDataSource failed: %v", err)
	}
	if updateResp.Name != "Updated Hospital EHR" || updateResp.Port != 3307 {
		t.Errorf("unexpected updated datasource: %+v", updateResp)
	}

	// 5. TestConnection
	testResp, err := client.TestConnection(ctx, &pb.TestConnectionRequest{Id: dsID})
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if !testResp.Success {
		t.Errorf("expected successful connection test, got: %+v", testResp)
	}

	// 6. GetMetadata (database type fallback)
	metaResp, err := client.GetMetadata(ctx, &pb.GetMetadataRequest{Id: dsID})
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if len(metaResp.Tables) == 0 {
		t.Errorf("expected tables metadata, got none: %+v", metaResp)
	}

	// 7. GetAccessAudit
	auditResp, err := client.GetAccessAudit(ctx, &pb.GetAccessAuditRequest{Id: dsID})
	if err != nil {
		t.Fatalf("GetAccessAudit failed: %v", err)
	}
	if auditResp.Total < 1 || len(auditResp.Records) < 1 {
		t.Errorf("unexpected access audit records: %+v", auditResp)
	}

	// 8. Delete
	delResp, err := client.DeleteDataSource(ctx, &pb.DeleteDataSourceRequest{Id: dsID})
	if err != nil {
		t.Fatalf("DeleteDataSource failed: %v", err)
	}
	if !delResp.Success {
		t.Errorf("expected successful delete: %+v", delResp)
	}

	// 9. Get after delete should return NotFound
	_, err = client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: dsID})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for deleted datasource, got code: %v, err: %v", status.Code(err), err)
	}
}

func TestGRPCValidationErrors(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t, nil)
	defer cleanup()

	ctx := context.Background()

	// Empty name
	_, err := client.CreateDataSource(ctx, &pb.CreateDataSourceRequest{
		Name: "",
		Type: "database",
		Host: "localhost",
		Port: 3306,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty name, got: %v", err)
	}

	// Invalid type
	_, err = client.CreateDataSource(ctx, &pb.CreateDataSourceRequest{
		Name: "Valid Name",
		Type: "unknown_type",
		Host: "localhost",
		Port: 3306,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for invalid type, got: %v", err)
	}

	// Invalid port
	_, err = client.CreateDataSource(ctx, &pb.CreateDataSourceRequest{
		Name: "Valid Name",
		Type: "database",
		Host: "localhost",
		Port: 70000,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for invalid port, got: %v", err)
	}

	// Get with empty ID
	_, err = client.GetDataSource(ctx, &pb.GetDataSourceRequest{Id: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty id, got: %v", err)
	}
}

func TestGRPCSeedAndCSVOperations(t *testing.T) {
	client, _, cleanup := setupTestGRPCServer(t, nil)
	defer cleanup()

	ctx := context.Background()

	// 1. Seed
	seedResp, err := client.SeedDataSources(ctx, &pb.SeedDataSourcesRequest{})
	if err != nil {
		t.Fatalf("SeedDataSources failed: %v", err)
	}
	if seedResp.SeededCount < 2 {
		t.Errorf("expected at least 2 seeded datasets, got %d", seedResp.SeededCount)
	}

	// 2. Get Records from seeded yibao
	recResp, err := client.GetDataSourceRecords(ctx, &pb.GetRecordsRequest{
		Id:     "ds_yibao",
		Limit:  5,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("GetDataSourceRecords failed: %v", err)
	}
	if recResp.Total == 0 || len(recResp.Records) == 0 {
		t.Errorf("expected CSV records from ds_yibao, got total %d", recResp.Total)
	}

	// 3. Get Metadata from seeded kangyang
	metaResp, err := client.GetMetadata(ctx, &pb.GetMetadataRequest{
		Id: "ds_kangyang",
	})
	if err != nil {
		t.Fatalf("GetMetadata failed for ds_kangyang: %v", err)
	}
	if len(metaResp.Tables) == 0 || len(metaResp.Tables[0].Fields) == 0 {
		t.Errorf("expected table metadata for ds_kangyang: %+v", metaResp)
	}
}

// ─────────────────────────────────────────────────────────────
// mTLS Credentials and Key Pinning Tests
// ─────────────────────────────────────────────────────────────

func generateTestCertAndKey(t *testing.T, tmpDir string) (string, string, string, string) {
	t.Helper()

	// 1. CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "PrivShield-Test-CA",
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

	// 2. Server Cert
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

	// 3. Client PubKey for Pinning
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

	// 3. Valid TLS without client auth
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

	// 6. Missing CA when client auth enabled
	cfg = &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   srvCert,
		TLSKeyFile:    srvKey,
		TLSClientAuth: "require",
	}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Errorf("expected error when CA file missing with client auth enabled")
	}

	// 7. Invalid client auth mode
	cfg = &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   srvCert,
		TLSKeyFile:    srvKey,
		TLSCAFile:     caFile,
		TLSClientAuth: "invalid_mode",
	}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Errorf("expected error for invalid client auth mode")
	}
}
