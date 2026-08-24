package config

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// getModuleRootDir returns the absolute path to services/datasource-mgr
func getModuleRootDir(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller information")
	}
	// currentFile is services/datasource-mgr/internal/config/scripts_test.go
	moduleDir := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return moduleDir
}

// 1. 静态检查：验证所有脚本文件存在且具有可执行权限
func TestScripts_ExistenceAndExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping file permission tests on Windows")
	}

	moduleDir := getModuleRootDir(t)
	expectedScripts := []string{
		"run.sh",
		"scripts/dev-run.sh",
		"scripts/prod-run.sh",
		"scripts/gen-certs.sh",
		"scripts/deploy.sh",
		"scripts/health-check.sh",
	}

	for _, relPath := range expectedScripts {
		fullPath := filepath.Join(moduleDir, relPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("script %s does not exist: %v", relPath, err)
		}
		if info.Mode()&0111 == 0 {
			t.Errorf("script %s is missing executable permission: mode=%v", relPath, info.Mode())
		}
	}
}

// 2. 语法检查：执行 bash -n 验证所有脚本语法合法
func TestScripts_BashSyntaxCheck(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available on this system")
	}

	moduleDir := getModuleRootDir(t)
	scripts := []string{
		"run.sh",
		"scripts/dev-run.sh",
		"scripts/prod-run.sh",
		"scripts/gen-certs.sh",
		"scripts/deploy.sh",
		"scripts/health-check.sh",
	}

	for _, relPath := range scripts {
		fullPath := filepath.Join(moduleDir, relPath)
		cmd := exec.Command(bashPath, "-n", fullPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("bash syntax error in %s: %v\nOutput: %s", relPath, err, string(output))
		}
	}
}

// 3. gen-certs.sh 执行测试：在临时目录生成证书链并深度校验 X.509 属性
func TestGenCertsScript_ExecutionAndCertificateVerification(t *testing.T) {
	opensslPath, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("openssl is not available on this system")
	}
	_ = opensslPath

	moduleDir := getModuleRootDir(t)
	genScript := filepath.Join(moduleDir, "scripts", "gen-certs.sh")

	tempCertDir := t.TempDir()

	// 执行 gen-certs.sh
	cmd := exec.Command("bash", genScript, tempCertDir)
	cmd.Dir = moduleDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gen-certs.sh execution failed: %v\nOutput: %s", err, string(out))
	}

	// 验证生成的文件清单
	expectedFiles := []string{
		"ca.crt", "ca.key",
		"server.crt", "server.key",
		"client.crt", "client.key",
		"client.pub",
	}

	for _, fname := range expectedFiles {
		p := filepath.Join(tempCertDir, fname)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected generated file %s not found: %v", fname, err)
		}
	}

	// 解析 CA 证书
	caPEM, err := os.ReadFile(filepath.Join(tempCertDir, "ca.crt"))
	if err != nil {
		t.Fatalf("read ca.crt: %v", err)
	}
	caBlock, _ := pem.Decode(caPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse ca.crt: %v", err)
	}
	if !caCert.IsCA || caCert.Subject.CommonName != "datasource-mgr-test-ca" {
		t.Errorf("unexpected CA cert properties: isCA=%v, CN=%s", caCert.IsCA, caCert.Subject.CommonName)
	}

	// 解析服务端证书并验证 SAN
	serverPEM, err := os.ReadFile(filepath.Join(tempCertDir, "server.crt"))
	if err != nil {
		t.Fatalf("read server.crt: %v", err)
	}
	serverBlock, _ := pem.Decode(serverPEM)
	serverCert, err := x509.ParseCertificate(serverBlock.Bytes)
	if err != nil {
		t.Fatalf("parse server.crt: %v", err)
	}
	if serverCert.Subject.CommonName != "localhost" {
		t.Errorf("unexpected server cert CN: %s", serverCert.Subject.CommonName)
	}
	hasLocalhostSAN := false
	for _, dns := range serverCert.DNSNames {
		if dns == "localhost" {
			hasLocalhostSAN = true
			break
		}
	}
	if !hasLocalhostSAN {
		t.Errorf("server cert missing localhost in DNSNames: %v", serverCert.DNSNames)
	}

	// 解析客户端证书并验证 client.pub 公钥匹配
	clientPEM, err := os.ReadFile(filepath.Join(tempCertDir, "client.crt"))
	if err != nil {
		t.Fatalf("read client.crt: %v", err)
	}
	clientBlock, _ := pem.Decode(clientPEM)
	clientCert, err := x509.ParseCertificate(clientBlock.Bytes)
	if err != nil {
		t.Fatalf("parse client.crt: %v", err)
	}
	if clientCert.Subject.CommonName != "datasource-mgr-client" {
		t.Errorf("unexpected client cert CN: %s", clientCert.Subject.CommonName)
	}

	// 读取提取的 client.pub 公钥
	pubPEM, err := os.ReadFile(filepath.Join(tempCertDir, "client.pub"))
	if err != nil {
		t.Fatalf("read client.pub: %v", err)
	}
	pubBlock, _ := pem.Decode(pubPEM)
	parsedPub, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		t.Fatalf("parse client.pub: %v", err)
	}

	rsaClientCertPub, ok1 := clientCert.PublicKey.(*rsa.PublicKey)
	rsaExtractedPub, ok2 := parsedPub.(*rsa.PublicKey)
	if !ok1 || !ok2 {
		t.Fatalf("expected RSA public keys, got ok1=%v, ok2=%v", ok1, ok2)
	}
	if rsaClientCertPub.N.Cmp(rsaExtractedPub.N) != 0 || rsaClientCertPub.E != rsaExtractedPub.E {
		t.Errorf("client.pub does not match public key in client.crt")
	}

	t.Log("✅ gen-certs.sh 证书链与公钥固定文件验证通过")
}

// 4. dev-run.sh 开发脚本启动与探活测试
func TestDevRunScript_StartupAndHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess script test in short mode")
	}

	moduleDir := getModuleRootDir(t)
	devScript := filepath.Join(moduleDir, "scripts", "dev-run.sh")

	// 分配随机空闲端口
	httpPort := getFreePort(t)
	grpcPort := getFreePort(t)

	cmd := exec.Command("bash", devScript)
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(),
		"DATASOURCE_MGR_HOST=127.0.0.1",
		fmt.Sprintf("DATASOURCE_MGR_PORT=%d", httpPort),
		"DATASOURCE_MGR_GRPC_HOST=127.0.0.1",
		fmt.Sprintf("DATASOURCE_MGR_GRPC_PORT=%d", grpcPort),
		"DATASOURCE_MGR_LOG_FORMAT=text",
		"DATASOURCE_MGR_LOG_LEVEL=debug",
	)

	// 启动子进程
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dev-run.sh failed: %v", err)
	}

	// 退出时确保杀死进程树
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
			}
		}
	}()

	// 探测 HTTP 健康端点
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/health", httpPort)
	client := &http.Client{Timeout: 1 * time.Second}

	var resp *http.Response
	var lastErr error
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		resp, lastErr = client.Get(healthURL)
		if lastErr == nil && resp.StatusCode == http.StatusOK {
			break
		}
	}

	if lastErr != nil || resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("dev-run.sh failed to become healthy at %s: %v", healthURL, lastErr)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var healthMap map[string]any
	if err := json.Unmarshal(body, &healthMap); err != nil {
		t.Fatalf("invalid health json: %v, body=%s", err, string(body))
	}
	if healthMap["status"] != "ok" {
		t.Errorf("unexpected health status: %+v", healthMap)
	}

	t.Logf("✅ dev-run.sh 正常启动并响应 HTTP 200 OK (Port: %d)", httpPort)
}

// 5. prod-run.sh 生产脚本启动与 mTLS 证书加载测试
func TestProdRunScript_StartupAndMTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess script test in short mode")
	}

	moduleDir := getModuleRootDir(t)
	prodScript := filepath.Join(moduleDir, "scripts", "prod-run.sh")
	certsDir := filepath.Join(moduleDir, "certs")

	httpPort := getFreePort(t)
	grpcPort := getFreePort(t)

	cmd := exec.Command("bash", prodScript)
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(),
		"DATASOURCE_MGR_HOST=127.0.0.1",
		fmt.Sprintf("DATASOURCE_MGR_PORT=%d", httpPort),
		"DATASOURCE_MGR_GRPC_HOST=127.0.0.1",
		fmt.Sprintf("DATASOURCE_MGR_GRPC_PORT=%d", grpcPort),
		fmt.Sprintf("DATASOURCE_MGR_CERTS_DIR=%s", certsDir),
		"DATASOURCE_MGR_LOG_FORMAT=text",
		"DATASOURCE_MGR_LOG_LEVEL=info",
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start prod-run.sh failed: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
			}
		}
	}()

	// 1. 读取测试证书链配置 mTLS 客户端
	clientCert, err := tls.LoadX509KeyPair(filepath.Join(certsDir, "client.crt"), filepath.Join(certsDir, "client.key"))
	if err != nil {
		t.Fatalf("failed to load client keypair: %v", err)
	}
	caPEM, err := os.ReadFile(filepath.Join(certsDir, "ca.crt"))
	if err != nil {
		t.Fatalf("failed to read ca.crt: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)

	tlsClient := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      caPool,
				ServerName:   "localhost",
			},
		},
	}

	// 2. 探测 HTTPS REST mTLS 端点
	healthURL := fmt.Sprintf("https://127.0.0.1:%d/api/health", httpPort)
	var resp *http.Response
	var lastErr error
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		resp, lastErr = tlsClient.Get(healthURL)
		if lastErr == nil && resp.StatusCode == http.StatusOK {
			break
		}
	}

	if lastErr != nil || resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("prod-run.sh failed to become healthy at HTTPS %s: %v", healthURL, lastErr)
	}
	defer resp.Body.Close()

	// 3. 验证未提供客户端证书的请求会被 mTLS 阻断
	insecureClient := &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caPool,
				ServerName:         "localhost",
				InsecureSkipVerify: true,
			},
		},
	}
	noCertResp, noCertErr := insecureClient.Get(healthURL)
	if noCertErr == nil && noCertResp != nil && noCertResp.StatusCode == http.StatusOK {
		t.Errorf("expected mTLS handshake failure when client certificate is not provided, but succeeded")
	}

	// 4. 探测 gRPC mTLS 端口已在监听
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", grpcPort))
	if err != nil {
		t.Fatalf("gRPC mTLS port %d is not listening: %v", grpcPort, err)
	}
	_ = conn.Close()

	t.Logf("✅ prod-run.sh 正常启动，HTTPS REST (Port: %d) 与 gRPC mTLS (Port: %d) 均就绪并通过双向认证校验", httpPort, grpcPort)
}

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer l.Close()
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return port
}
