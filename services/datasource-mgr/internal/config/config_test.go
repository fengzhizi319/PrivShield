package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	os.Unsetenv("DATASOURCE_MGR_HOST")
	os.Unsetenv("DATASOURCE_MGR_PORT")
	os.Unsetenv("DATASOURCE_MGR_GRPC_HOST")
	os.Unsetenv("DATASOURCE_MGR_GRPC_PORT")
	os.Unsetenv("DATASOURCE_MGR_TLS_ENABLED")

	cfg := Load()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default Host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 8083 {
		t.Errorf("expected default Port 8083, got %d", cfg.Port)
	}
	if cfg.GRPCHost != "127.0.0.1" {
		t.Errorf("expected default GRPCHost 127.0.0.1, got %s", cfg.GRPCHost)
	}
	if cfg.GRPCPort != 50053 {
		t.Errorf("expected default GRPCPort 50053, got %d", cfg.GRPCPort)
	}
	if cfg.TLSEnabled {
		t.Errorf("expected default TLSEnabled to be false")
	}
	if cfg.Address() != "127.0.0.1:8083" {
		t.Errorf("expected Address() 127.0.0.1:8083, got %s", cfg.Address())
	}
	if cfg.GRPCAddress() != "127.0.0.1:50053" {
		t.Errorf("expected GRPCAddress() 127.0.0.1:50053, got %s", cfg.GRPCAddress())
	}
}

func TestConfigCustomEnv(t *testing.T) {
	t.Setenv("DATASOURCE_MGR_HOST", "0.0.0.0")
	t.Setenv("DATASOURCE_MGR_PORT", "9083")
	t.Setenv("DATASOURCE_MGR_GRPC_HOST", "0.0.0.0")
	t.Setenv("DATASOURCE_MGR_GRPC_PORT", "60053")
	t.Setenv("DATASOURCE_MGR_TLS_ENABLED", "true")
	t.Setenv("DATASOURCE_MGR_TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("DATASOURCE_MGR_TLS_KEY_FILE", "/tmp/key.pem")
	t.Setenv("DATASOURCE_MGR_TLS_CA_FILE", "/tmp/ca.pem")
	t.Setenv("DATASOURCE_MGR_TLS_CLIENT_AUTH", "require")
	t.Setenv("DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE", "/tmp/pinned.pem")
	t.Setenv("DATASOURCE_MGR_API_KEY", "secret-key")
	t.Setenv("DATASOURCE_MGR_CORS_ORIGINS", "http://localhost:3000,https://example.com")
	t.Setenv("DATASOURCE_MGR_LOG_FORMAT", "text")
	t.Setenv("DATASOURCE_MGR_LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.Host != "0.0.0.0" || cfg.Port != 9083 {
		t.Errorf("custom address mismatch: %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.GRPCHost != "0.0.0.0" || cfg.GRPCPort != 60053 {
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
	if cfg.APIKey != "secret-key" {
		t.Errorf("custom api key mismatch: %s", cfg.APIKey)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(cfg.CORSOrigins))
	}
}
