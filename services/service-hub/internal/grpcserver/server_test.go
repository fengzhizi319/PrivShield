package grpcserver

import (
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

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
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
