package grpcserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
	pb "github.com/fengzhizi319/PrivShield/services/service-hub/proto"
)

// testCerts holds paths to test certificate files.
// testCerts 保存测试证书文件路径。
type testCerts struct {
	caFile     string
	serverCert string
	serverKey  string
	clientCert string
	clientKey  string
	clientPub  string
}

// genTestCerts generates a complete test certificate chain in a temp directory.
// genTestCerts 在临时目录中生成完整的测试证书链。
func genTestCerts(t *testing.T) testCerts {
	t.Helper()
	dir := t.TempDir()

	// Generate CA key and self-signed CA cert
	// 生成 CA 私钥和自签名 CA 证书
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caFile := writePEM(t, dir, "ca.crt", "CERTIFICATE", caDER)
	writePEM(t, dir, "ca.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caKey))

	// Parse CA for signing
	// 解析 CA 用于签发
	caCert, _ := x509.ParseCertificate(caDER)

	// Generate server cert signed by CA
	// 生成由 CA 签发的服务端证书
	serverKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, _ := x509.CreateCertificate(rand.Reader, serverTmpl, caCert, &serverKey.PublicKey, caKey)
	serverCert := writePEM(t, dir, "server.crt", "CERTIFICATE", serverDER)
	serverKeyFile := writePEM(t, dir, "server.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey))

	// Generate client cert signed by CA
	// 生成由 CA 签发的客户端证书
	clientKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, _ := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	clientCert := writePEM(t, dir, "client.crt", "CERTIFICATE", clientDER)
	clientKeyFile := writePEM(t, dir, "client.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(clientKey))

	// Extract client public key
	// 提取客户端公钥
	clientPubDER, _ := x509.MarshalPKIXPublicKey(&clientKey.PublicKey)
	clientPub := writePEM(t, dir, "client.pub", "PUBLIC KEY", clientPubDER)

	return testCerts{
		caFile:     caFile,
		serverCert: serverCert,
		serverKey:  serverKeyFile,
		clientCert: clientCert,
		clientKey:  clientKeyFile,
		clientPub:  clientPub,
	}
}

// writePEM writes a DER-encoded block as PEM to a file.
// writePEM 将 DER 编码的块以 PEM 格式写入文件。
func writePEM(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatalf("encode PEM %s: %v", path, err)
	}
	return path
}

// ─────────────────────────────────────────────────────────────
// Tests / 测试用例
// ─────────────────────────────────────────────────────────────

// TestBuildServerCredentialsTLSDisabled verifies that disabled TLS returns error.
// TestBuildServerCredentialsTLSDisabled 验证禁用 TLS 时返回错误。
func TestBuildServerCredentialsTLSDisabled(t *testing.T) {
	cfg := &config.Config{TLSEnabled: false}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Fatal("expected error when TLS is disabled")
	}
}

// TestBuildServerCredentialsMissingCert verifies error when cert is missing.
// TestBuildServerCredentialsMissingCert 验证缺少证书时报错。
func TestBuildServerCredentialsMissingCert(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:  true,
		TLSCertFile: "",
		TLSKeyFile:  "/some/key.pem",
	}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Fatal("expected error when cert file is missing")
	}
}

// TestBuildServerCredentialsInvalidCertPath verifies error with invalid cert path.
// TestBuildServerCredentialsInvalidCertPath 验证使用无效证书路径时报错。
func TestBuildServerCredentialsInvalidCertPath(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:  true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}
	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Fatal("expected error when cert path is invalid")
	}
}

// TestBuildServerCredentialsMTLS verifies successful mTLS credential building.
// TestBuildServerCredentialsMTLS 验证成功构建 mTLS 凭证。
func TestBuildServerCredentialsMTLS(t *testing.T) {
	certs := genTestCerts(t)
	cfg := &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   certs.serverCert,
		TLSKeyFile:    certs.serverKey,
		TLSCAFile:     certs.caFile,
		TLSClientAuth: "require",
	}

	creds, err := BuildServerCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
	if creds.Info().SecurityProtocol != "tls" {
		t.Errorf("security protocol = %q, want tls", creds.Info().SecurityProtocol)
	}
}

// TestBuildServerCredentialsMTLSWithPinnedKey verifies mTLS with public key pinning.
// TestBuildServerCredentialsMTLSWithPinnedKey 验证带公钥固定的 mTLS。
func TestBuildServerCredentialsMTLSWithPinnedKey(t *testing.T) {
	certs := genTestCerts(t)
	cfg := &config.Config{
		TLSEnabled:          true,
		TLSCertFile:         certs.serverCert,
		TLSKeyFile:          certs.serverKey,
		TLSCAFile:           certs.caFile,
		TLSClientAuth:       "require",
		TLSPinnedPubKeyFile: certs.clientPub,
	}

	creds, err := BuildServerCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
}

// TestBuildServerCredentialsMTLSMissingCA verifies error when CA is missing for mTLS.
// TestBuildServerCredentialsMTLSMissingCA 验证 mTLS 缺少 CA 证书时报错。
func TestBuildServerCredentialsMTLSMissingCA(t *testing.T) {
	certs := genTestCerts(t)
	cfg := &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   certs.serverCert,
		TLSKeyFile:    certs.serverKey,
		TLSCAFile:     "", // Missing CA
		TLSClientAuth: "require",
	}

	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Fatal("expected error when CA file is missing for mTLS")
	}
}

// TestBuildServerCredentialsInvalidClientAuthMode verifies error with unknown auth mode.
// TestBuildServerCredentialsInvalidClientAuthMode 验证使用未知认证模式时报错。
func TestBuildServerCredentialsInvalidClientAuthMode(t *testing.T) {
	certs := genTestCerts(t)
	cfg := &config.Config{
		TLSEnabled:    true,
		TLSCertFile:   certs.serverCert,
		TLSKeyFile:    certs.serverKey,
		TLSCAFile:     certs.caFile,
		TLSClientAuth: "unknown-mode",
	}

	if _, err := BuildServerCredentials(cfg); err == nil {
		t.Fatal("expected error with unknown client auth mode")
	}
}

// TestLoadPublicKey verifies public key loading from PEM file.
// TestLoadPublicKey 验证从 PEM 文件加载公钥。
func TestLoadPublicKey(t *testing.T) {
	certs := genTestCerts(t)

	key, err := loadPublicKey(certs.clientPub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil public key")
	}
}

// TestLoadPublicKeyInvalidPath verifies error with invalid path.
// TestLoadPublicKeyInvalidPath 验证使用无效路径时报错。
func TestLoadPublicKeyInvalidPath(t *testing.T) {
	if _, err := loadPublicKey("/nonexistent/key.pub"); err == nil {
		t.Fatal("expected error with invalid path")
	}
}

// TestLoadPublicKeyInvalidPEM verifies error with invalid PEM content.
// TestLoadPublicKeyInvalidPEM 验证使用无效 PEM 内容时报错。
func TestLoadPublicKeyInvalidPEM(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.pub")
	if err := os.WriteFile(badFile, []byte("not a PEM file"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	if _, err := loadPublicKey(badFile); err == nil {
		t.Fatal("expected error with invalid PEM content")
	}
}

// TestPublicKeysEqual verifies public key comparison.
// TestPublicKeysEqual 验证公钥比较。
func TestPublicKeysEqual(t *testing.T) {
	certs := genTestCerts(t)

	key1, _ := loadPublicKey(certs.clientPub)
	key2, _ := loadPublicKey(certs.clientPub)

	if !publicKeysEqual(key1, key2) {
		t.Error("same public keys should be equal")
	}

	// Generate a different key
	dir := t.TempDir()
	diffKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	diffPubDER, _ := x509.MarshalPKIXPublicKey(&diffKey.PublicKey)
	diffPub := writePEM(t, dir, "diff.pub", "PUBLIC KEY", diffPubDER)
	key3, _ := loadPublicKey(diffPub)

	if publicKeysEqual(key1, key3) {
		t.Error("different public keys should not be equal")
	}
}

// TestPublicKeysEqualDifferentTypes verifies comparison of different key types.
// TestPublicKeysEqualDifferentTypes 验证不同类型公钥的比较。
func TestPublicKeysEqualDifferentTypes(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	if publicKeysEqual(&rsaKey.PublicKey, "not a key") {
		t.Error("RSA key should not equal string")
	}
}

// ─────────────────────────────────────────────────────────────
// gRPC Server Method Tests / gRPC 服务方法单元测试
// ─────────────────────────────────────────────────────────────

func setupTestGRPCServer(t *testing.T, agentHandler http.HandlerFunc) (*GRPCServer, *httptest.Server, store.TaskStore) {
	t.Helper()
	var mockServer *httptest.Server
	if agentHandler != nil {
		mockServer = httptest.NewServer(agentHandler)
	} else {
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/health":
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			case "/v1/dynclassification/eval_record":
				_ = json.NewEncoder(w).Encode(map[string]any{"level": "L3", "tags": []string{"PII"}})
			case "/v1/privacy/mask", "/v1/privacy/mask_record":
				_ = json.NewEncoder(w).Encode(map[string]any{"result": "masked"})
			default:
				http.NotFound(w, r)
			}
		}))
	}

	t.Setenv("PRIVACY_AGENT_URLS", mockServer.URL)
	cfg := config.Load()
	ag := agent.New(cfg)
	taskStore := memory.NewTaskStore()
	logger := slog.Default()

	srv := New(ag, cfg, taskStore, logger)
	return srv, mockServer, taskStore
}

func TestGRPCServer_Health(t *testing.T) {
	t.Run("Reachable", func(t *testing.T) {
		srv, mockServer, _ := setupTestGRPCServer(t, nil)
		defer mockServer.Close()
		defer srv.Shutdown()

		resp, err := srv.Health(context.Background(), &pb.HealthRequest{})
		if err != nil {
			t.Fatalf("Health failed: %v", err)
		}
		if resp.Backend != "ok" || resp.Agent != "ok" {
			t.Errorf("Health unexpected response: %+v", resp)
		}
	})

	t.Run("Unreachable", func(t *testing.T) {
		srv, mockServer, _ := setupTestGRPCServer(t, nil)
		mockServer.Close() // Close upstream
		defer srv.Shutdown()

		resp, err := srv.Health(context.Background(), &pb.HealthRequest{})
		if err != nil {
			t.Fatalf("Health returned gRPC error: %v", err)
		}
		if resp.Agent != "unreachable" || resp.Error == "" {
			t.Errorf("Health expected unreachable status, got: %+v", resp)
		}
	})
}

func TestGRPCServer_HubStatus(t *testing.T) {
	srv, mockServer, taskStore := setupTestGRPCServer(t, nil)
	defer mockServer.Close()
	defer srv.Shutdown()

	now := time.Now()
	_ = taskStore.Save(&store.Task{ID: "t-1", Status: "running", Stage: "classify", CreatedAt: now})
	_ = taskStore.Save(&store.Task{ID: "t-2", Status: "pending", Stage: "queued", CreatedAt: now})
	_ = taskStore.Save(&store.Task{ID: "t-3", Status: "completed", Stage: "done", CreatedAt: now})

	resp, err := srv.HubStatus(context.Background(), &pb.HubStatusRequest{})
	if err != nil {
		t.Fatalf("HubStatus failed: %v", err)
	}
	if resp.Status != "running" || resp.ActiveTasks != 1 || resp.QueuedTasks != 1 || resp.CompletedTotal != 1 {
		t.Errorf("HubStatus unexpected counts: %+v", resp)
	}
}

func TestGRPCServer_Dispatch(t *testing.T) {
	srv, mockServer, _ := setupTestGRPCServer(t, nil)
	defer mockServer.Close()
	defer srv.Shutdown()

	ctx := context.Background()

	t.Run("Validation_EmptySource", func(t *testing.T) {
		_, err := srv.Dispatch(ctx, &pb.DispatchRequest{Operation: "mask"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got: %v", err)
		}
	})

	t.Run("Validation_EmptyOperation", func(t *testing.T) {
		_, err := srv.Dispatch(ctx, &pb.DispatchRequest{Source: "test.csv"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got: %v", err)
		}
	})

	t.Run("Validation_OversizedSource", func(t *testing.T) {
		_, err := srv.Dispatch(ctx, &pb.DispatchRequest{
			Source:    strings.Repeat("a", 1025),
			Operation: "mask",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for oversized source, got: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		resp, err := srv.Dispatch(ctx, &pb.DispatchRequest{
			Source:      "yibao.csv",
			Operation:   "mask",
			PayloadJson: `{"name":"张三"}`,
			Priority:    1,
		})
		if err != nil {
			t.Fatalf("Dispatch failed: %v", err)
		}
		if resp.TaskId == "" || resp.Status != "accepted" {
			t.Errorf("unexpected dispatch response: %+v", resp)
		}

		// Wait briefly and check task was saved
		time.Sleep(50 * time.Millisecond)
		task, err := srv.GetTask(ctx, &pb.GetTaskRequest{TaskId: resp.TaskId})
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if task.Id != resp.TaskId || task.Source != "yibao.csv" {
			t.Errorf("task retrieved mismatch: %+v", task)
		}
	})
}

func TestGRPCServer_ClassifyAndDispatch(t *testing.T) {
	srv, mockServer, _ := setupTestGRPCServer(t, nil)
	defer mockServer.Close()
	defer srv.Shutdown()

	ctx := context.Background()

	t.Run("Validation_EmptySource", func(t *testing.T) {
		_, err := srv.ClassifyAndDispatch(ctx, &pb.ClassifyAndDispatchRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got: %v", err)
		}
	})

	t.Run("Validation_OversizedSource", func(t *testing.T) {
		_, err := srv.ClassifyAndDispatch(ctx, &pb.ClassifyAndDispatchRequest{
			Source: strings.Repeat("a", 1025),
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got: %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		resp, err := srv.ClassifyAndDispatch(ctx, &pb.ClassifyAndDispatchRequest{
			Source:      "medical.csv",
			PayloadJson: `{"diagnosis":"C78.0"}`,
		})
		if err != nil {
			t.Fatalf("ClassifyAndDispatch failed: %v", err)
		}
		if resp.TaskId == "" || resp.Level != "L3" || resp.AutoOperation != "k_anon" {
			t.Errorf("unexpected ClassifyAndDispatch response: %+v", resp)
		}
	})
}

func TestGRPCServer_GetAndListTasks(t *testing.T) {
	srv, mockServer, taskStore := setupTestGRPCServer(t, nil)
	defer mockServer.Close()
	defer srv.Shutdown()

	ctx := context.Background()

	// Seed tasks
	now := time.Now()
	_ = taskStore.Save(&store.Task{ID: "t-10", Status: "running", Stage: "fetch", Source: "src1", CreatedAt: now})
	_ = taskStore.Save(&store.Task{ID: "t-20", Status: "completed", Stage: "done", Source: "src2", CreatedAt: now.Add(time.Second)})

	t.Run("GetTask_NotFound", func(t *testing.T) {
		_, err := srv.GetTask(ctx, &pb.GetTaskRequest{TaskId: "nonexistent"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got: %v", err)
		}
	})

	t.Run("GetTask_EmptyID", func(t *testing.T) {
		_, err := srv.GetTask(ctx, &pb.GetTaskRequest{TaskId: ""})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got: %v", err)
		}
	})

	t.Run("GetTask_Success", func(t *testing.T) {
		task, err := srv.GetTask(ctx, &pb.GetTaskRequest{TaskId: "t-10"})
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if task.Id != "t-10" || task.Status != "running" {
			t.Errorf("unexpected task: %+v", task)
		}
	})

	t.Run("ListTasks_All", func(t *testing.T) {
		resp, err := srv.ListTasks(ctx, &pb.ListTasksRequest{})
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}
		if resp.Total != 2 || len(resp.Tasks) != 2 {
			t.Errorf("unexpected list response: %+v", resp)
		}
	})

	t.Run("ListTasks_FilterStatus", func(t *testing.T) {
		resp, err := srv.ListTasks(ctx, &pb.ListTasksRequest{StatusFilter: "completed"})
		if err != nil {
			t.Fatalf("ListTasks filter failed: %v", err)
		}
		if resp.Total != 1 || len(resp.Tasks) != 1 || resp.Tasks[0].Id != "t-20" {
			t.Errorf("unexpected filtered list response: %+v", resp)
		}
	})

	t.Run("ListTasks_InvalidFilter", func(t *testing.T) {
		_, err := srv.ListTasks(ctx, &pb.ListTasksRequest{StatusFilter: "invalid_status"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for invalid filter, got: %v", err)
		}
	})
}

func TestGRPCServer_PipelineStatus(t *testing.T) {
	srv, mockServer, taskStore := setupTestGRPCServer(t, nil)
	defer mockServer.Close()
	defer srv.Shutdown()

	now := time.Now()
	_ = taskStore.Save(&store.Task{ID: "t-1", Status: "running", Stage: "ingest", CreatedAt: now})
	_ = taskStore.Save(&store.Task{ID: "t-2", Status: "running", Stage: "classify", CreatedAt: now})

	resp, err := srv.PipelineStatus(context.Background(), &pb.PipelineStatusRequest{})
	if err != nil {
		t.Fatalf("PipelineStatus failed: %v", err)
	}
	if !resp.AgentOk || len(resp.Stages) != 6 {
		t.Errorf("unexpected PipelineStatus response: %+v", resp)
	}

	for _, stage := range resp.Stages {
		if stage.Name == "ingest" && (stage.Status != "processing" || stage.ActiveCount != 1) {
			t.Errorf("expected ingest processing with 1 active, got: %+v", stage)
		}
	}
}

func TestGRPCServer_ProcessTask_FailureBranches(t *testing.T) {
	t.Run("ClassifyFails", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/dynclassification/eval_record" {
				http.Error(w, "internal model error", http.StatusInternalServerError)
				return
			}
			http.NotFound(w, r)
		}))
		defer mockServer.Close()

		t.Setenv("PRIVACY_AGENT_URLS", mockServer.URL)
		cfg := config.Load()
		ag := agent.New(cfg)
		taskStore := memory.NewTaskStore()
		srv := New(ag, cfg, taskStore, slog.Default())
		defer srv.Shutdown()

		task := &store.Task{
			ID:        "task-fail-1",
			Status:    "pending",
			Source:    "test.csv",
			Operation: "classify",
			CreatedAt: time.Now(),
		}
		_ = taskStore.Save(task)

		// Run processTask synchronously to verify completion and failure state
		srv.processTask(task, "classify", `{"record":"test"}`)

		updated, err := taskStore.Get("task-fail-1")
		if err != nil {
			t.Fatalf("Get task failed: %v", err)
		}
		if updated.Status != "failed" || !strings.Contains(updated.Error, "classify failed") {
			t.Errorf("expected failed status with classify error, got: %+v", updated)
		}
	})

	t.Run("MaskFails", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/privacy/mask" {
				http.Error(w, "masking engine down", http.StatusInternalServerError)
				return
			}
			http.NotFound(w, r)
		}))
		defer mockServer.Close()

		t.Setenv("PRIVACY_AGENT_URLS", mockServer.URL)
		cfg := config.Load()
		ag := agent.New(cfg)
		taskStore := memory.NewTaskStore()
		srv := New(ag, cfg, taskStore, slog.Default())
		defer srv.Shutdown()

		task := &store.Task{
			ID:        "task-fail-2",
			Status:    "pending",
			Source:    "test.csv",
			Operation: "mask",
			CreatedAt: time.Now(),
		}
		_ = taskStore.Save(task)

		srv.processTask(task, "mask", `{"name":"test"}`)

		updated, err := taskStore.Get("task-fail-2")
		if err != nil {
			t.Fatalf("Get task failed: %v", err)
		}
		if updated.Status != "failed" || !strings.Contains(updated.Error, "desensitize failed") {
			t.Errorf("expected failed status with desensitize error, got: %+v", updated)
		}
	})

	t.Run("CancellationOnShutdown", func(t *testing.T) {
		srv, mockServer, taskStore := setupTestGRPCServer(t, nil)
		defer mockServer.Close()

		task := &store.Task{
			ID:        "task-cancel",
			Status:    "pending",
			Source:    "test.csv",
			Operation: "mask",
			CreatedAt: time.Now(),
		}
		_ = taskStore.Save(task)

		srv.wg.Add(1)
		go func() {
			defer srv.wg.Done()
			srv.processTask(task, "mask", `{}`)
		}()

		time.Sleep(20 * time.Millisecond)
		srv.Shutdown()

		updated, err := taskStore.Get("task-cancel")
		if err != nil {
			t.Fatalf("Get task failed: %v", err)
		}
		if updated.Status != "failed" || !strings.Contains(updated.Error, "server shutting down") {
			t.Errorf("expected failed status with shutting down error, got: %+v", updated)
		}
	})
}


