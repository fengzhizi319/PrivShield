// Package config provides centralized configuration for the mock datasource-mgr module.
// Package config 为模拟数据源模块提供集中化配置管理。
package config

import (
	"strconv"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
)

// Config holds runtime configuration for the mock datasource-mgr server.
type Config struct {
	Host string // HTTP listen host
	Port int    // HTTP listen port

	// gRPC server configuration
	GRPCHost string // gRPC listen host
	GRPCPort int    // gRPC listen port

	// mTLS configuration / mTLS 双向认证配置
	TLSEnabled          bool   // Enable TLS/mTLS on gRPC server
	TLSCertFile         string // Server certificate PEM
	TLSKeyFile          string // Server private key PEM
	TLSCAFile           string // CA cert for client verification
	TLSClientAuth       string // Client auth mode: "require" | "verify" | ""
	TLSPinnedPubKeyFile string // Pinned client public key PEM

	// Security & Observability
	APIKey      string   // Inbound API key
	CORSOrigins []string // Allowed CORS origins
	LogFormat   string   // "json" or "text"
	LogLevel    string   // "debug", "info", "warn", "error"
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Host: pkgconfig.EnvString("DATASOURCE_MGR_HOST", "127.0.0.1"),
		Port: pkgconfig.EnvInt("DATASOURCE_MGR_PORT", 8083),

		GRPCHost: pkgconfig.EnvString("DATASOURCE_MGR_GRPC_HOST", "127.0.0.1"),
		GRPCPort: pkgconfig.EnvInt("DATASOURCE_MGR_GRPC_PORT", 50053),

		TLSEnabled:          pkgconfig.EnvBool("DATASOURCE_MGR_TLS_ENABLED", false),
		TLSCertFile:         pkgconfig.EnvString("DATASOURCE_MGR_TLS_CERT_FILE", ""),
		TLSKeyFile:          pkgconfig.EnvString("DATASOURCE_MGR_TLS_KEY_FILE", ""),
		TLSCAFile:           pkgconfig.EnvString("DATASOURCE_MGR_TLS_CA_FILE", ""),
		TLSClientAuth:       pkgconfig.EnvString("DATASOURCE_MGR_TLS_CLIENT_AUTH", ""),
		TLSPinnedPubKeyFile: pkgconfig.EnvString("DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE", ""),

		APIKey:      pkgconfig.EnvString("DATASOURCE_MGR_API_KEY", ""),
		CORSOrigins: pkgconfig.EnvStringSlice("DATASOURCE_MGR_CORS_ORIGINS"),
		LogFormat:   pkgconfig.EnvString("DATASOURCE_MGR_LOG_FORMAT", "json"),
		LogLevel:    pkgconfig.EnvString("DATASOURCE_MGR_LOG_LEVEL", "info"),
	}
}

// Address returns the full HTTP listen address.
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// GRPCAddress returns the full gRPC listen address.
func (c *Config) GRPCAddress() string {
	return c.GRPCHost + ":" + strconv.Itoa(c.GRPCPort)
}
