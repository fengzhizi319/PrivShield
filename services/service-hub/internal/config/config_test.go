package config

import (
	"os"
	"testing"
)

// TestLoadDefaults tests that Load() populates expected default values when no environment variables are set.
// TestLoadDefaults 测试在未设置任何环境变量时，Load() 能正确赋予安全的默认配置值。
func TestLoadDefaults(t *testing.T) {
	// 清理可能存在的环境变量，确保测试默认值不受外部干扰
	os.Unsetenv("SERVICE_HUB_HOST")
	os.Unsetenv("SERVICE_HUB_PORT")
	os.Unsetenv("PRIVACY_AGENT_REST_HOST")
	os.Unsetenv("PRIVACY_REST_PORT")
	os.Unsetenv("PRIVACY_AGENT_API_KEY")

	// 执行配置加载
	cfg := Load()

	// 校验默认 HTTP 主机、端口与上游 Agent 地址
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host=127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 8082 {
		t.Errorf("expected port=8082, got %d", cfg.Port)
	}
	if cfg.AgentRESTHost != "127.0.0.1" {
		t.Errorf("expected agent host=127.0.0.1, got %s", cfg.AgentRESTHost)
	}
	if cfg.AgentRESTPort != 8079 {
		t.Errorf("expected agent port=8079, got %d", cfg.AgentRESTPort)
	}
	if cfg.AgentAPIKey != "" {
		t.Errorf("expected empty API key, got %s", cfg.AgentAPIKey)
	}
}

// TestLoadFromEnv tests that custom environment variables correctly override the default configuration.
// TestLoadFromEnv 测试当注入自定义环境变量时，Load() 能够精确读取并覆盖默认配置。
func TestLoadFromEnv(t *testing.T) {
	os.Setenv("SERVICE_HUB_HOST", "0.0.0.0")
	os.Setenv("SERVICE_HUB_PORT", "9090")
	os.Setenv("PRIVACY_AGENT_REST_HOST", "10.0.0.1")
	os.Setenv("PRIVACY_REST_PORT", "9079")
	os.Setenv("PRIVACY_AGENT_API_KEY", "test-key")
	defer func() {
		os.Unsetenv("SERVICE_HUB_HOST")
		os.Unsetenv("SERVICE_HUB_PORT")
		os.Unsetenv("PRIVACY_AGENT_REST_HOST")
		os.Unsetenv("PRIVACY_REST_PORT")
		os.Unsetenv("PRIVACY_AGENT_API_KEY")
	}()

	cfg := Load()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host=0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("expected port=9090, got %d", cfg.Port)
	}
	if cfg.AgentRESTHost != "10.0.0.1" {
		t.Errorf("expected agent host=10.0.0.1, got %s", cfg.AgentRESTHost)
	}
	if cfg.AgentRESTPort != 9079 {
		t.Errorf("expected agent port=9079, got %d", cfg.AgentRESTPort)
	}
	if cfg.AgentAPIKey != "test-key" {
		t.Errorf("expected API key=test-key, got %s", cfg.AgentAPIKey)
	}
}

// TestAddress tests the Address() helper string formatting.
// TestAddress 测试 Address() 方法能正确输出 "host:port" 格式的 HTTP 监听地址字符串。
func TestAddress(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: 8082}
	if addr := cfg.Address(); addr != "127.0.0.1:8082" {
		t.Errorf("expected 127.0.0.1:8082, got %s", addr)
	}
}

// TestAgentBaseURL tests the AgentBaseURL() helper method.
// TestAgentBaseURL 测试 AgentBaseURL() 方法能正确拼接上游 Agent 的 HTTP REST 基础 URL。
func TestAgentBaseURL(t *testing.T) {
	cfg := &Config{AgentRESTHost: "10.0.0.1", AgentRESTPort: 8079}
	if url := cfg.AgentBaseURL(); url != "http://10.0.0.1:8079" {
		t.Errorf("expected http://10.0.0.1:8079, got %s", url)
	}
}

// TestGRPCAddress tests the GRPCAddress() helper method.
// TestGRPCAddress 测试 GRPCAddress() 方法能正确输出 gRPC 监听网络地址。
func TestGRPCAddress(t *testing.T) {
	cfg := &Config{GRPCHost: "127.0.0.1", GRPCPort: 50052}
	if addr := cfg.GRPCAddress(); addr != "127.0.0.1:50052" {
		t.Errorf("expected 127.0.0.1:50052, got %s", addr)
	}
}

// TestLoadProductionHardeningDefaults tests the production hardening defaults (DB, API Key, Log).
// TestLoadProductionHardeningDefaults 测试生产加固相关的配置默认值（空 API Key、空 DB 路径、json 日志格式、info 级别）。
func TestLoadProductionHardeningDefaults(t *testing.T) {
	os.Unsetenv("SERVICE_HUB_API_KEY")
	os.Unsetenv("SERVICE_HUB_CORS_ORIGINS")
	os.Unsetenv("SERVICE_HUB_DB_PATH")
	os.Unsetenv("SERVICE_HUB_LOG_FORMAT")
	os.Unsetenv("SERVICE_HUB_LOG_LEVEL")

	cfg := Load()

	if cfg.APIKey != "" {
		t.Errorf("expected empty API key, got %s", cfg.APIKey)
	}
	if len(cfg.CORSOrigins) != 0 {
		t.Errorf("expected empty CORS origins, got %v", cfg.CORSOrigins)
	}
	if cfg.DBPath != "" {
		t.Errorf("expected empty DB path, got %s", cfg.DBPath)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("expected log format=json, got %s", cfg.LogFormat)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected log level=info, got %s", cfg.LogLevel)
	}
}

// TestAgentBaseURLs tests single URL fallback and multiple upstream agent URLs parsing.
// TestAgentBaseURLs 测试上游 Agent URL 列表获取：
// 1) 未设置 PRIVACY_AGENT_URLS 时回退为单个 AgentBaseURL；
// 2) 设置了以逗号分隔的多个 URL 时能正确拆分为切片，供多活/故障转移调用。
func TestAgentBaseURLs(t *testing.T) {
	t.Run("DefaultSingleURL", func(t *testing.T) {
		t.Setenv("PRIVACY_AGENT_URLS", "")
		cfg := &Config{AgentRESTHost: "127.0.0.1", AgentRESTPort: 8079}
		urls := cfg.AgentBaseURLs()
		if len(urls) != 1 || urls[0] != "http://127.0.0.1:8079" {
			t.Errorf("expected [http://127.0.0.1:8079], got %v", urls)
		}
	})

	t.Run("MultipleURLsFromEnv", func(t *testing.T) {
		t.Setenv("PRIVACY_AGENT_URLS", "http://node1:8079,http://node2:8079")
		cfg := &Config{AgentRESTHost: "127.0.0.1", AgentRESTPort: 8079}
		urls := cfg.AgentBaseURLs()
		if len(urls) != 2 || urls[0] != "http://node1:8079" || urls[1] != "http://node2:8079" {
			t.Errorf("expected 2 URLs, got %v", urls)
		}
	})
}

// TestLoadAllEnvVariables tests that all service-hub environment variables are mapped accurately.
// TestLoadAllEnvVariables 综合测试所有环境变量（gRPC、队列、超时、mTLS 证书/私钥/CA/ClientAuth/公钥固定、跨域、DB 路径、日志）的完整映射。
func TestLoadAllEnvVariables(t *testing.T) {
	t.Setenv("SERVICE_HUB_GRPC_HOST", "0.0.0.0")
	t.Setenv("SERVICE_HUB_GRPC_PORT", "50059")
	t.Setenv("SERVICE_HUB_MAX_QUEUE", "500")
	t.Setenv("SERVICE_HUB_SCHEDULE_TIMEOUT", "60")
	t.Setenv("SERVICE_HUB_TLS_ENABLED", "true")
	t.Setenv("SERVICE_HUB_TLS_CERT_FILE", "/path/server.crt")
	t.Setenv("SERVICE_HUB_TLS_KEY_FILE", "/path/server.key")
	t.Setenv("SERVICE_HUB_TLS_CA_FILE", "/path/ca.crt")
	t.Setenv("SERVICE_HUB_TLS_CLIENT_AUTH", "require")
	t.Setenv("SERVICE_HUB_TLS_PINNED_PUBKEY_FILE", "/path/client.pub")
	t.Setenv("SERVICE_HUB_CORS_ORIGINS", "http://localhost:3000,http://localhost:5173")
	t.Setenv("SERVICE_HUB_DB_PATH", "/tmp/hub.db")
	t.Setenv("SERVICE_HUB_LOG_FORMAT", "text")
	t.Setenv("SERVICE_HUB_LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.GRPCHost != "0.0.0.0" || cfg.GRPCPort != 50059 {
		t.Errorf("gRPC host/port mismatch: %s:%d", cfg.GRPCHost, cfg.GRPCPort)
	}
	if cfg.MaxQueueDepth != 500 || cfg.ScheduleTimeout != 60 {
		t.Errorf("queue depth / timeout mismatch: depth=%d, timeout=%d", cfg.MaxQueueDepth, cfg.ScheduleTimeout)
	}
	if !cfg.TLSEnabled || cfg.TLSCertFile != "/path/server.crt" || cfg.TLSClientAuth != "require" {
		t.Errorf("TLS config mismatch: %+v", cfg)
	}
	if cfg.TLSPinnedPubKeyFile != "/path/client.pub" {
		t.Errorf("pinned pubkey mismatch: %s", cfg.TLSPinnedPubKeyFile)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.DBPath != "/tmp/hub.db" || cfg.LogFormat != "text" || cfg.LogLevel != "debug" {
		t.Errorf("hardening configs mismatch: %+v", cfg)
	}
}

// TestDatasourceConfig tests datasource URL and gRPC address formatting methods.
// TestDatasourceConfig 测试 DatasourceBaseURL() 与 DatasourceGRPCAddress() 方法的格式化正确性。
func TestDatasourceConfig(t *testing.T) {
	cfg := &Config{
		DatasourceRESTHost: "127.0.0.1",
		DatasourceRESTPort: 8083,
		DatasourceGRPCHost: "127.0.0.1",
		DatasourceGRPCPort: 50053,
	}
	if cfg.DatasourceBaseURL() != "http://127.0.0.1:8083" {
		t.Errorf("expected http://127.0.0.1:8083, got %s", cfg.DatasourceBaseURL())
	}
	if cfg.DatasourceGRPCAddress() != "127.0.0.1:50053" {
		t.Errorf("expected 127.0.0.1:50053, got %s", cfg.DatasourceGRPCAddress())
	}
}
