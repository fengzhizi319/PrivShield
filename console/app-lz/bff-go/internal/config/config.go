package config

import (
	"os"
	"strconv"
)

// Config represents the configuration for the App-LZ Go BFF service.
type Config struct {
	Host          string
	Port          string
	HubURL        string
	DatasourceURL string
	AuditURL      string
	AgentURL      string
	StaticDir     string
	LogLevel      string
	TLSEnabled    bool
	CertFile      string
	KeyFile       string
	ClientCAFile  string
	APIKey        string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	host := getEnv("APP_LZ_HOST", "0.0.0.0")
	port := getEnv("APP_LZ_PORT", "8085")
	hubURL := getEnv("APP_LZ_HUB_URL", "http://127.0.0.1:8082")
	datasourceURL := getEnv("APP_LZ_DATASOURCE_URL", "http://127.0.0.1:8083")
	auditURL := getEnv("APP_LZ_AUDIT_URL", "http://127.0.0.1:8084")
	agentURL := getEnv("APP_LZ_AGENT_URL", "http://127.0.0.1:8079")
	staticDir := getEnv("APP_LZ_STATIC_DIR", "./web/dist")
	logLevel := getEnv("APP_LZ_LOG_LEVEL", "info")

	tlsEnabled, _ := strconv.ParseBool(getEnv("APP_LZ_TLS_ENABLED", "false"))
	certFile := getEnv("APP_LZ_CERT_FILE", "")
	keyFile := getEnv("APP_LZ_KEY_FILE", "")
	clientCAFile := getEnv("APP_LZ_CLIENT_CA_FILE", "")
	apiKey := getEnv("APP_LZ_API_KEY", "")

	return &Config{
		Host:          host,
		Port:          port,
		HubURL:        hubURL,
		DatasourceURL: datasourceURL,
		AuditURL:      auditURL,
		AgentURL:      agentURL,
		StaticDir:     staticDir,
		LogLevel:      logLevel,
		TLSEnabled:    tlsEnabled,
		CertFile:      certFile,
		KeyFile:       keyFile,
		ClientCAFile:  clientCAFile,
		APIKey:        apiKey,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
