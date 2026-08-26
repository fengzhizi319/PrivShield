package config

import (
	"os"
	"strconv"
)

// Config represents the configuration for the App-LZ Go BFF service.
type Config struct {
	Host           string
	Port           string
	HubURL         string
	HubGRPC        string
	DatasourceURL  string
	DatasourceGRPC string
	AuditURL       string
	AuditGRPC      string
	AgentURL       string
	AgentGRPC      string
	StaticDir      string
	LogLevel       string
	TLSEnabled     bool
	CertFile       string
	KeyFile        string
	ClientCAFile   string
	APIKey         string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	host := getEnv("APP_LZ_HOST", "0.0.0.0")
	port := getEnv("APP_LZ_PORT", "8085")
	hubURL := getEnv("APP_LZ_HUB_URL", getEnv("HUB_URL", "http://127.0.0.1:8082"))
	hubGRPC := getEnv("APP_LZ_HUB_GRPC", getEnv("HUB_GRPC", "127.0.0.1:50052"))
	datasourceURL := getEnv("APP_LZ_DATASOURCE_URL", getEnv("DATASOURCE_URL", "http://127.0.0.1:8083"))
	datasourceGRPC := getEnv("APP_LZ_DATASOURCE_GRPC", getEnv("DATASOURCE_GRPC", "127.0.0.1:50053"))
	auditURL := getEnv("APP_LZ_AUDIT_URL", getEnv("AUDIT_URL", "http://127.0.0.1:8084"))
	auditGRPC := getEnv("APP_LZ_AUDIT_GRPC", getEnv("AUDIT_GRPC", "127.0.0.1:50054"))
	agentURL := getEnv("APP_LZ_AGENT_URL", getEnv("AGENT_URL", "http://127.0.0.1:8079"))
	agentGRPC := getEnv("APP_LZ_AGENT_GRPC", getEnv("AGENT_GRPC", "127.0.0.1:50051"))
	staticDir := getEnv("APP_LZ_STATIC_DIR", "./web/dist")
	logLevel := getEnv("APP_LZ_LOG_LEVEL", "info")

	tlsEnabled, _ := strconv.ParseBool(getEnv("APP_LZ_TLS_ENABLED", "false"))
	certFile := getEnv("APP_LZ_CERT_FILE", "")
	keyFile := getEnv("APP_LZ_KEY_FILE", "")
	clientCAFile := getEnv("APP_LZ_CLIENT_CA_FILE", "")
	apiKey := getEnv("APP_LZ_API_KEY", "")

	return &Config{
		Host:           host,
		Port:           port,
		HubURL:         hubURL,
		HubGRPC:        hubGRPC,
		DatasourceURL:  datasourceURL,
		DatasourceGRPC: datasourceGRPC,
		AuditURL:       auditURL,
		AuditGRPC:      auditGRPC,
		AgentURL:       agentURL,
		AgentGRPC:      agentGRPC,
		StaticDir:      staticDir,
		LogLevel:       logLevel,
		TLSEnabled:     tlsEnabled,
		CertFile:       certFile,
		KeyFile:        keyFile,
		ClientCAFile:   clientCAFile,
		APIKey:         apiKey,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
