package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env vars to test defaults
	os.Unsetenv("SERVICE_HUB_HOST")
	os.Unsetenv("SERVICE_HUB_PORT")
	os.Unsetenv("PRIVACY_AGENT_REST_HOST")
	os.Unsetenv("PRIVACY_REST_PORT")
	os.Unsetenv("PRIVACY_AGENT_API_KEY")

	cfg := Load()

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

func TestAddress(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: 8082}
	if addr := cfg.Address(); addr != "127.0.0.1:8082" {
		t.Errorf("expected 127.0.0.1:8082, got %s", addr)
	}
}

func TestAgentBaseURL(t *testing.T) {
	cfg := &Config{AgentRESTHost: "10.0.0.1", AgentRESTPort: 8079}
	if url := cfg.AgentBaseURL(); url != "http://10.0.0.1:8079" {
		t.Errorf("expected http://10.0.0.1:8079, got %s", url)
	}
}

func TestGRPCAddress(t *testing.T) {
	cfg := &Config{GRPCHost: "127.0.0.1", GRPCPort: 50052}
	if addr := cfg.GRPCAddress(); addr != "127.0.0.1:50052" {
		t.Errorf("expected 127.0.0.1:50052, got %s", addr)
	}
}

func TestLoadProductionHardeningDefaults(t *testing.T) {
	// Clear env vars to test defaults
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


