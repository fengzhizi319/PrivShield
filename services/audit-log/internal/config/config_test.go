package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	os.Unsetenv("AUDIT_LOG_HOST")
	os.Unsetenv("AUDIT_LOG_PORT")
	os.Unsetenv("AUDIT_LOG_GRPC_HOST")
	os.Unsetenv("AUDIT_LOG_GRPC_PORT")
	os.Unsetenv("PRIVACY_AGENT_REST_HOST")
	os.Unsetenv("PRIVACY_REST_PORT")
	os.Unsetenv("AUDIT_LOG_TLS_ENABLED")

	cfg := Load()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default Host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 8084 {
		t.Errorf("expected default Port 8084, got %d", cfg.Port)
	}
	if cfg.GRPCHost != "127.0.0.1" {
		t.Errorf("expected default GRPCHost 127.0.0.1, got %s", cfg.GRPCHost)
	}
	if cfg.GRPCPort != 50054 {
		t.Errorf("expected default GRPCPort 50054, got %d", cfg.GRPCPort)
	}
	if cfg.TLSEnabled {
		t.Errorf("expected default TLSEnabled to be false")
	}
	if cfg.Address() != "127.0.0.1:8084" {
		t.Errorf("expected Address() 127.0.0.1:8084, got %s", cfg.Address())
	}
	if cfg.GRPCAddress() != "127.0.0.1:50054" {
		t.Errorf("expected GRPCAddress() 127.0.0.1:50054, got %s", cfg.GRPCAddress())
	}
	if cfg.AgentBaseURL() != "http://127.0.0.1:8079" {
		t.Errorf("expected AgentBaseURL() http://127.0.0.1:8079, got %s", cfg.AgentBaseURL())
	}
}

func TestConfigCustomEnv(t *testing.T) {
	t.Setenv("AUDIT_LOG_HOST", "0.0.0.0")
	t.Setenv("AUDIT_LOG_PORT", "9084")
	t.Setenv("AUDIT_LOG_GRPC_HOST", "0.0.0.0")
	t.Setenv("AUDIT_LOG_GRPC_PORT", "60054")
	t.Setenv("AUDIT_LOG_TLS_ENABLED", "true")
	t.Setenv("AUDIT_LOG_TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("AUDIT_LOG_TLS_KEY_FILE", "/tmp/key.pem")
	t.Setenv("AUDIT_LOG_TLS_CA_FILE", "/tmp/ca.pem")
	t.Setenv("AUDIT_LOG_TLS_CLIENT_AUTH", "require")
	t.Setenv("AUDIT_LOG_TLS_PINNED_PUBKEY_FILE", "/tmp/pinned.pem")
	t.Setenv("AUDIT_LOG_API_KEY", "secret-audit-key")
	t.Setenv("AUDIT_LOG_CORS_ORIGINS", "http://localhost:3000,https://audit.example.com")
	t.Setenv("AUDIT_LOG_DB_PATH", "/tmp/audit_test.db")
	t.Setenv("AUDIT_LOG_LOG_FORMAT", "text")
	t.Setenv("AUDIT_LOG_LOG_LEVEL", "debug")
	t.Setenv("PRIVACY_AGENT_URLS", "http://agent1:8079,http://agent2:8079")

	cfg := Load()

	if cfg.Host != "0.0.0.0" || cfg.Port != 9084 {
		t.Errorf("custom address mismatch: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.GRPCHost != "0.0.0.0" || cfg.GRPCPort != 60054 {
		t.Errorf("custom grpc address mismatch: %s:%d", cfg.GRPCHost, cfg.GRPCPort)
	}
	if !cfg.TLSEnabled {
		t.Errorf("expected TLSEnabled true")
	}
	if cfg.TLSCertFile != "/tmp/cert.pem" || cfg.TLSKeyFile != "/tmp/key.pem" {
		t.Errorf("custom tls cert/key mismatch")
	}
	if cfg.TLSCAFile != "/tmp/ca.pem" || cfg.TLSClientAuth != "require" {
		t.Errorf("custom tls ca/client auth mismatch")
	}
	if cfg.TLSPinnedPubKeyFile != "/tmp/pinned.pem" {
		t.Errorf("custom pinned pub key mismatch")
	}
	if cfg.APIKey != "secret-audit-key" {
		t.Errorf("custom api key mismatch: %s", cfg.APIKey)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(cfg.CORSOrigins))
	}
	if cfg.DBPath != "/tmp/audit_test.db" || cfg.LogFormat != "text" || cfg.LogLevel != "debug" {
		t.Errorf("custom db/log mismatch")
	}
	urls := cfg.AgentBaseURLs()
	if len(urls) != 2 || urls[0] != "http://agent1:8079" {
		t.Errorf("custom AgentBaseURLs() mismatch: %v", urls)
	}
}
