// Package config provides centralized configuration for the datasource-mgr module.
// Package config 为数据源管理模块提供集中化配置管理。
package config

import (
	"strconv"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
)

// Config holds all runtime configuration for the datasource-mgr server.
// Config 保存数据源管理服务器运行时的所有配置项。
type Config struct {
	Host          string // HTTP listen host / HTTP 监听地址
	Port          int    // HTTP listen port / HTTP 监听端口
	AgentRESTHost string // Upstream agent REST host / 上游 agent REST 地址
	AgentRESTPort int    // Upstream agent REST port / 上游 agent REST 端口
	AgentAPIKey   string // Optional auth key for upstream agent / 上游 agent 认证密钥

	// gRPC server configuration / gRPC 服务器配置
	GRPCHost string // gRPC listen host / gRPC 监听地址
	GRPCPort int    // gRPC listen port / gRPC 监听端口

	// mTLS configuration / mTLS 双向认证配置
	TLSEnabled    bool   // Enable TLS/mTLS on gRPC server / 在 gRPC 服务端启用 TLS/mTLS
	TLSCertFile   string // Server certificate PEM / 服务端证书文件路径
	TLSKeyFile    string // Server private key PEM / 服务端私钥文件路径
	TLSCAFile     string // CA cert for client verification / 用于校验客户端证书的 CA 证书路径
	TLSClientAuth string // Client auth mode: "require" | "verify" | "" / 客户端认证模式

	// Public key pinning / 公钥固定（额外安全层）
	TLSPinnedPubKeyFile string // Pinned client public key PEM / 固定的客户端公钥文件路径

	// Production hardening / 生产加固
	APIKey      string   // Inbound API key for this module / 本模块入站 API Key
	CORSOrigins []string // Allowed CORS origins / 允许的 CORS 来源
	DBPath      string   // SQLite database path (empty = in-memory) / SQLite 数据库路径
	LogFormat   string   // "json" or "text" / 日志格式
	LogLevel    string   // "debug", "info", "warn", "error" / 日志级别
}

// Load reads configuration from environment variables.
// Load 从环境变量读取所有配置项。
func Load() *Config {
	return &Config{
		Host:          pkgconfig.EnvString("DATASOURCE_MGR_HOST", "127.0.0.1"),
		Port:          pkgconfig.EnvInt("DATASOURCE_MGR_PORT", 8083),
		AgentRESTHost: pkgconfig.EnvString("PRIVACY_AGENT_REST_HOST", "127.0.0.1"),
		AgentRESTPort: pkgconfig.EnvInt("PRIVACY_REST_PORT", 8079),
		AgentAPIKey:   pkgconfig.EnvString("PRIVACY_AGENT_API_KEY", ""),

		// gRPC / gRPC 配置
		GRPCHost: pkgconfig.EnvString("DATASOURCE_MGR_GRPC_HOST", "127.0.0.1"),
		GRPCPort: pkgconfig.EnvInt("DATASOURCE_MGR_GRPC_PORT", 50053),

		// mTLS / 双向认证配置
		TLSEnabled:    pkgconfig.EnvBool("DATASOURCE_MGR_TLS_ENABLED", false),
		TLSCertFile:   pkgconfig.EnvString("DATASOURCE_MGR_TLS_CERT_FILE", ""),
		TLSKeyFile:    pkgconfig.EnvString("DATASOURCE_MGR_TLS_KEY_FILE", ""),
		TLSCAFile:     pkgconfig.EnvString("DATASOURCE_MGR_TLS_CA_FILE", ""),
		TLSClientAuth: pkgconfig.EnvString("DATASOURCE_MGR_TLS_CLIENT_AUTH", ""),

		// Public key pinning / 公钥固定
		TLSPinnedPubKeyFile: pkgconfig.EnvString("DATASOURCE_MGR_TLS_PINNED_PUBKEY_FILE", ""),

		// Production hardening / 生产加固
		APIKey:      pkgconfig.EnvString("DATASOURCE_MGR_API_KEY", ""),
		CORSOrigins: pkgconfig.EnvStringSlice("DATASOURCE_MGR_CORS_ORIGINS"),
		DBPath:      pkgconfig.EnvString("DATASOURCE_MGR_DB_PATH", ""),
		LogFormat:   pkgconfig.EnvString("DATASOURCE_MGR_LOG_FORMAT", "json"),
		LogLevel:    pkgconfig.EnvString("DATASOURCE_MGR_LOG_LEVEL", "info"),
	}
}

// Address returns the full HTTP listen address.
// Address 返回完整的 HTTP 监听地址。
func (c *Config) Address() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

// AgentBaseURL returns the upstream agent REST base URL.
// AgentBaseURL 返回上游 agent REST 基础地址。
func (c *Config) AgentBaseURL() string {
	return "http://" + c.AgentRESTHost + ":" + strconv.Itoa(c.AgentRESTPort)
}

// AgentBaseURLs returns all configured upstream agent REST base URLs.
func (c *Config) AgentBaseURLs() []string {
	envURLs := pkgconfig.EnvStringSlice("PRIVACY_AGENT_URLS")
	if len(envURLs) > 0 {
		return envURLs
	}
	return []string{c.AgentBaseURL()}
}

// GRPCAddress returns the full gRPC listen address.
// GRPCAddress 返回完整的 gRPC 监听地址。
func (c *Config) GRPCAddress() string {
	return c.GRPCHost + ":" + strconv.Itoa(c.GRPCPort)
}
