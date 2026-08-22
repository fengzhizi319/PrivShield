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

func TestGetEnvIntInvalid(t *testing.T) {
	os.Setenv("TEST_INVALID_INT", "not-a-number")
	defer os.Unsetenv("TEST_INVALID_INT")

	got := getEnvInt("TEST_INVALID_INT", 42)
	if got != 42 {
		t.Errorf("expected default 42 for invalid int, got %d", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.value)
		got := getEnvBool("TEST_BOOL", false)
		if got != tt.expected {
			t.Errorf("getEnvBool(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
	os.Unsetenv("TEST_BOOL")
}
