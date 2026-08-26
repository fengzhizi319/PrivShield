package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	os.Unsetenv("APP_LZ_HOST")
	os.Unsetenv("APP_LZ_PORT")
	os.Unsetenv("APP_LZ_HUB_URL")

	cfg := Load()
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != "8085" {
		t.Errorf("expected port 8085, got %s", cfg.Port)
	}
	if cfg.HubURL != "http://127.0.0.1:8082" {
		t.Errorf("expected hub url http://127.0.0.1:8082, got %s", cfg.HubURL)
	}
	if cfg.TLSEnabled != false {
		t.Errorf("expected tls disabled by default")
	}
}

func TestConfigCustomEnv(t *testing.T) {
	os.Setenv("APP_LZ_PORT", "9095")
	os.Setenv("APP_LZ_TLS_ENABLED", "true")
	defer func() {
		os.Unsetenv("APP_LZ_PORT")
		os.Unsetenv("APP_LZ_TLS_ENABLED")
	}()

	cfg := Load()
	if cfg.Port != "9095" {
		t.Errorf("expected port 9095, got %s", cfg.Port)
	}
	if !cfg.TLSEnabled {
		t.Errorf("expected tls enabled")
	}
}
